package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// stubTokens 는 고정 토큰 TokenProvider. invalidated 는 Invalidate 호출 횟수.
type stubTokens struct {
	tokens      []string // 호출 순서대로 반환, 마지막 값 반복
	calls       int32
	invalidated int32

	mu        sync.Mutex
	lastStale string
}

func (s *stubTokens) Token(context.Context) (string, error) {
	i := int(atomic.AddInt32(&s.calls, 1)) - 1
	if i >= len(s.tokens) {
		i = len(s.tokens) - 1
	}
	return s.tokens[i], nil
}
func (s *stubTokens) Invalidate(stale string) {
	atomic.AddInt32(&s.invalidated, 1)
	s.mu.Lock()
	s.lastStale = stale
	s.mu.Unlock()
}

func newClient(t *testing.T, h http.HandlerFunc, tokens ...string) (*Client, *stubTokens, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	st := &stubTokens{tokens: tokens}
	if len(tokens) == 0 {
		st.tokens = []string{"tok"}
	}
	c := New(Config{BaseURL: srv.URL, HTTPClient: srv.Client(), Tokens: st})
	return c, st, srv.Close
}

func TestGet_UnwrapsResultAndSendsBearer(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/prices" || r.URL.Query().Get("symbols") != "005930,AAPL" {
			t.Errorf("unexpected %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"symbol":"005930"},{"symbol":"AAPL"}]}`))
	})
	defer done()
	var out []struct{ Symbol string }
	if err := c.Get(context.Background(), "/api/v1/prices", url.Values{"symbols": {"005930,AAPL"}}, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[1].Symbol != "AAPL" {
		t.Errorf("out = %+v", out)
	}
}

func TestGet_EmptyResultArray(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":[]}`))
	})
	defer done()
	var out []struct{ Symbol string }
	if err := c.Get(context.Background(), "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", out)
	}
}

func TestGet_MapsErrorEnvelope(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "hdr-id")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":{"requestId":"01HXYZ","code":"stock-not-found","message":"요청한 종목을 찾을 수 없습니다.","data":{"field":"symbol"}}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/api/v1/stocks/ZZZ/warnings", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.StatusCode != 404 || ae.RequestID != "01HXYZ" || ae.Code != "stock-not-found" || ae.Data["field"] != "symbol" {
		t.Errorf("got %+v", ae)
	}
	want := "toss: 404 stock-not-found: 요청한 종목을 찾을 수 없습니다. (requestId=01HXYZ)"
	if ae.Error() != want {
		t.Errorf("Error() = %q", ae.Error())
	}
}

func TestGet_NonEnvelopeErrorBody(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "hdr-id")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(strings.Repeat("x", 300)))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if ae.StatusCode != 403 || ae.Code != "" || len(ae.Message) != 200 || ae.RequestID != "hdr-id" {
		t.Errorf("got status=%d code=%q msglen=%d reqid=%q", ae.StatusCode, ae.Code, len(ae.Message), ae.RequestID)
	}
}

func TestGet_RetryAfterOn429(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"rate-limit-exceeded","message":""}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 429 || ae.RetryAfter != 3*time.Second {
		t.Errorf("got %+v (err=%v)", ae, err)
	}
}

func TestGet_RetriesOnceOnExpiredToken(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":"expired"}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok2" {
			t.Errorf("retry Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"result":{"ok":true}}`))
	}, "tok1", "tok2")
	defer done()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Get(context.Background(), "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || atomic.LoadInt32(&n) != 2 || atomic.LoadInt32(&st.invalidated) != 1 {
		t.Errorf("ok=%v calls=%d invalidated=%d", out.OK, n, st.invalidated)
	}
	st.mu.Lock()
	if st.lastStale != "tok1" {
		t.Errorf("Invalidate called with %q, want tok1", st.lastStale)
	}
	st.mu.Unlock()
}

func TestGet_DoesNotRetryTwice(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"invalid-token","message":""}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "invalid-token" {
		t.Fatalf("want invalid-token APIError, got %v", err)
	}
	if atomic.LoadInt32(&n) != 2 || atomic.LoadInt32(&st.invalidated) != 2 {
		t.Errorf("calls=%d invalidated=%d, want 2/2", n, st.invalidated)
	}
}

func TestGet_401OtherCodeNoRetry(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"login-user-not-found","message":""}}`))
	})
	defer done()
	_ = c.Get(context.Background(), "/x", nil, nil)
	if atomic.LoadInt32(&n) != 1 || atomic.LoadInt32(&st.invalidated) != 0 {
		t.Errorf("calls=%d invalidated=%d, want 1/0", n, st.invalidated)
	}
}

func TestGet_ContextCanceled(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"result":null}`))
	})
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Get(ctx, "/x", nil, nil); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

func TestGet_DecodeError(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"n":"notanumber"}}`))
	})
	defer done()
	var out struct {
		N int `json:"n"`
	}
	if err := c.Get(context.Background(), "/x", nil, &out); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("want decode error, got %v", err)
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New(Config{Tokens: &stubTokens{tokens: []string{"t"}}})
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", c.http.Timeout)
	}
}

type failingTokens struct{ err error }

func (f failingTokens) Token(context.Context) (string, error) { return "", f.err }
func (failingTokens) Invalidate(string)                       {}

func TestGet_TokenErrorSurfaces(t *testing.T) {
	want := errors.New("boom")
	c := New(Config{BaseURL: "http://127.0.0.1:0", Tokens: failingTokens{err: want}})
	if err := c.Get(context.Background(), "/x", nil, nil); !errors.Is(err, want) {
		t.Errorf("want token error passthrough, got %v", err)
	}
}

func TestGet_NullResultIsError(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":null}`))
	})
	defer done()
	var out struct{ A int }
	if err := c.Get(context.Background(), "/x", nil, &out); err == nil || !strings.Contains(err.Error(), "no result") {
		t.Errorf("want no-result error, got %v", err)
	}
	// out == nil 이면 바디를 보지 않으므로 에러 없음
	if err := c.Get(context.Background(), "/x", nil, nil); err != nil {
		t.Errorf("nil out must not error: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter(" 3 "); d != 3*time.Second {
		t.Errorf("seconds = %v", d)
	}
	if d := parseRetryAfter("-1"); d != 0 {
		t.Errorf("negative = %v", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("garbage = %v", d)
	}
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d < 80*time.Second || d > 91*time.Second {
		t.Errorf("http-date = %v", d)
	}
	past := time.Now().Add(-90 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(past); d != 0 {
		t.Errorf("past date = %v", d)
	}
}

func TestGet_NonEnvelopeErrorBodyRuneSafe(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte(strings.Repeat("가", 100)))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || !utf8.ValidString(ae.Message) || len(ae.Message) > 200 {
		t.Errorf("got %+v (%v)", ae, err)
	}
}

func TestDo_PostSendsBodyAndHeaders(t *testing.T) {
	type reqBody struct {
		Symbol string `json:"symbol"`
	}
	var got struct {
		method, ct, acct, auth string
		body                   string
	}
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.ct, got.acct, got.auth, got.body = r.Method, r.Header.Get("Content-Type"), r.Header.Get("X-Tossinvest-Account"), r.Header.Get("Authorization"), string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"orderId":"o-1"}}`))
	})
	defer done()
	var out struct {
		OrderID string `json:"orderId"`
	}
	err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/api/v1/orders", Body: reqBody{Symbol: "005930"}, AccountSeq: 7, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.ct != "application/json" || got.acct != "7" || got.auth != "Bearer tok" {
		t.Errorf("headers = %+v", got)
	}
	if got.body != `{"symbol":"005930"}` {
		t.Errorf("body = %s", got.body)
	}
	if out.OrderID != "o-1" {
		t.Errorf("out = %+v", out)
	}
}

func TestDo_NoAccountHeaderWhenZero(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["X-Tossinvest-Account"]; ok {
			t.Errorf("account header must be absent, got %q", r.Header.Get("X-Tossinvest-Account"))
		}
		_, _ = w.Write([]byte(`{"result":[]}`))
	})
	defer done()
	var out []struct{}
	if err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/api/v1/accounts", Out: &out, Idempotent: true}); err != nil {
		t.Fatal(err)
	}
}

func TestDo_NoContent(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer done()
	// Out 이 nil 이면 204 는 성공
	if err := c.Do(context.Background(), Request{Method: http.MethodDelete, Path: "/api/v1/conditional-orders/c-1", AccountSeq: 1}); err != nil {
		t.Fatalf("204 with nil Out: %v", err)
	}
}

func TestDo_NoContentWithOutIsError(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer done()
	var out struct{ A int }
	err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Out: &out})
	if err == nil || !strings.Contains(err.Error(), "no result") {
		t.Errorf("want no-result error, got %v", err)
	}
}

func TestDo_WriteWithoutIdempotencyDoesNotRetry(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
	})
	defer done()
	err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/api/v1/orders", Body: map[string]string{"a": "b"}})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "expired-token" {
		t.Fatalf("want expired-token, got %v", err)
	}
	// 멱등성 키가 없는 쓰기는 재시도하지 않는다(중복 주문 방지). 토큰은 무효화한다.
	if atomic.LoadInt32(&n) != 1 || atomic.LoadInt32(&st.invalidated) != 1 {
		t.Errorf("calls=%d invalidated=%d, want 1/1", atomic.LoadInt32(&n), atomic.LoadInt32(&st.invalidated))
	}
}

func TestDo_IdempotentWriteRetriesOnce(t *testing.T) {
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"orderId":"o-2"}}`))
	}, "tok1", "tok2")
	defer done()
	var out struct {
		OrderID string `json:"orderId"`
	}
	if err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/api/v1/orders", Body: map[string]string{"a": "b"}, Idempotent: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if out.OrderID != "o-2" || atomic.LoadInt32(&n) != 2 {
		t.Errorf("out=%+v calls=%d", out, atomic.LoadInt32(&n))
	}
}

func TestDo_BodyIsResentOnRetry(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"ok":true}}`))
	}, "tok1", "tok2")
	defer done()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: map[string]string{"k": "v"}, Idempotent: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || bodies[0] != bodies[1] || bodies[0] != `{"k":"v"}` {
		t.Errorf("bodies = %q", bodies)
	}
}

func TestGet_StillIdempotent(t *testing.T) {
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"ok":true}}`))
	}, "tok1", "tok2")
	defer done()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Get(context.Background(), "/x", nil, &out); err != nil || !out.OK {
		t.Fatalf("GET must still retry: %+v %v", out, err)
	}
}
