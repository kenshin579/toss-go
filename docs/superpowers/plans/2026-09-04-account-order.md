# toss-go 계좌·주문 15 ops (v0.2.0) Implementation Plan

> 내부 개발 문서(설계/실행 계획). 라이브러리 사용법은 [README](../../../README.md) 를 보세요.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 계좌·자산 2, 주문 8, 조건주문 5 = 15 ops 를 추가하고 v0.2.0 으로 릴리스한다. SDK 의 첫 쓰기(write) 경로다.

**Architecture:** `internal/httpclient` 에 `Do(ctx, Request)` 를 추가해 POST/DELETE·계좌 헤더·204·쓰기 재시도 정책을 담고, 기존 `Get` 은 그 위의 얇은 래퍼로 남긴다. 루트 `toss.Client` 에 `Accounts(ctx)`(헤더 불필요)와 `Account(seq) *AccountScope`(헤더 고정 핸들)를 추가하고, 스코프가 `asset`/`order`/`conditionalorder` 세 sub-client 를 보유한다. 주문 생성은 openapi 의 `oneOf` 를 메서드 2개(`Place` 수량 기준 / `PlaceAmount` 금액 기준 US 시장가 전용)로 갈라 타입으로 강제한다.

**안전 원칙(전 태스크 공통):** **실주문을 내는 코드는 어디에도 두지 않는다.** integration 테스트는 조회 9개만 호출하고, 예시·문서의 주문 코드는 주석 처리한다. 쓰기 경로는 스텁 서버로만 검증한다.

**Tech Stack:** Go 1.25, `shopspring/decimal`, 표준 `net/http`·`httptest`. 새 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-09-04-account-order-design.md`
**Branch:** `feature/account-order` (스펙 커밋 완료: 51c38bc)
**Base:** v0.1.0 (main 493ceb0) — `marketdata`/`stockinfo`/`marketinfo`/`ranking`/`indicators` 패턴을 그대로 따른다.

---

## 파일 구조

| 경로 | 책임 |
|---|---|
| `internal/httpclient/client.go` | (수정) `Request`/`Do` — 메서드·바디·계좌 헤더·204·`IdempotencyKey` 재시도 정책 |
| `internal/fetch/fetch.go` | (수정) `One`/`List` 에 accountSeq 추가, `PostOne`/`Send` 신규 |
| `tosstypes/types.go` | (수정) `AccountType` 4종 추가 |
| `accounts.go` | (신규) 루트 `Account` 타입 + `Client.Accounts` |
| `account.go` | (신규) `AccountScope` + `Client.Account(seq)` |
| `clientorderid.go` (+`_test.go`) | (신규) `NewClientOrderID`, `ValidateClientOrderID` |
| `codes.go` | (신규) 자주 쓰는 에러 코드 상수 |
| `asset/` | `client.go`, `holdings.go`, `asset_test.go`, `testdata/` |
| `order/` | `client.go`, `types.go`(열거·공용), `place.go`, `modify.go`, `history.go`, `info.go`, `order_test.go`, `testdata/` |
| `conditionalorder/` | `client.go`, `types.go`, `place.go`, `history.go`, `conditionalorder_test.go`, `testdata/` |
| `examples/order/main.go` | (신규) 조회 전용 예시 |
| `integration_test.go` | (수정) 조회 9개 추가 |
| `README.md` | (수정) 커버리지·계좌 스코프·주문 주의사항 |

fixture 는 전부 `docs/api/openapi.json` 의 응답 예시에서 뽑는다(쓰기 계열은 2xx 예시가 없어 fixture 없이 요청 조립만 검증). 추출 헬퍼는 각 태스크에 있다. 커밋 메시지는 항상 아래 트레일러로 끝낸다:

```
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
```

---

### Task 1: `internal/httpclient` — Request/Do (POST·DELETE·헤더·204·재시도 정책)

**Files:**
- Modify: `internal/httpclient/client.go`
- Modify: `internal/httpclient/client_test.go`

- [x] **Step 1: 실패 테스트 추가** — 기존 테스트는 그대로 두고 아래를 파일 끝에 덧붙인다.

```bash
cat >> internal/httpclient/client_test.go << 'EOF'

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
	if err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/api/v1/accounts", Out: &out}); err != nil {
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
	if err == nil || !strings.Contains(err.Error(), "empty response body") {
		t.Errorf("want empty-body error, got %v", err)
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
	if err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/api/v1/orders", Body: map[string]string{"a": "b"}, IdempotencyKey: "k1", Out: &out}); err != nil {
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
	if err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: map[string]string{"k": "v"}, IdempotencyKey: "k1", Out: &out}); err != nil {
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

func TestDo_DefaultsToGetAndRetries(t *testing.T) {
	// Method 를 비우면 GET 이고, GET 은 IdempotencyKey 없이도 재시도한다
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
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
	if err := c.Do(context.Background(), Request{Path: "/x", Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !out.OK || atomic.LoadInt32(&n) != 2 {
		t.Errorf("ok=%v calls=%d", out.OK, atomic.LoadInt32(&n))
	}
}

func TestDo_HeadersSurviveRetry(t *testing.T) {
	// 재시도된 주문에도 계좌 헤더와 Content-Type 이 실려야 한다
	var accts, cts []string
	var mu sync.Mutex
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		accts = append(accts, r.Header.Get("X-Tossinvest-Account"))
		cts = append(cts, r.Header.Get("Content-Type"))
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
	if err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: map[string]string{"k": "v"}, AccountSeq: 42, IdempotencyKey: "k1", Out: &out}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(accts) != 2 || accts[0] != "42" || accts[1] != "42" {
		t.Errorf("account headers = %q", accts)
	}
	if cts[0] != "application/json" || cts[1] != "application/json" {
		t.Errorf("content-types = %q", cts)
	}
}

func TestDo_IdempotentWriteDoesNotRetryTwice(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
	})
	defer done()
	err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: map[string]string{"a": "b"}, IdempotencyKey: "k1"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "expired-token" {
		t.Fatalf("want expired-token, got %v", err)
	}
	if atomic.LoadInt32(&n) != 2 || atomic.LoadInt32(&st.invalidated) != 2 {
		t.Errorf("calls=%d invalidated=%d, want 2/2", atomic.LoadInt32(&n), atomic.LoadInt32(&st.invalidated))
	}
}

func TestDo_WriteNonTokenUnauthorizedNoRetry(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"login-user-not-found","message":""}}`))
	})
	defer done()
	_ = c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: map[string]string{"a": "b"}, IdempotencyKey: "k1"})
	if atomic.LoadInt32(&n) != 1 || atomic.LoadInt32(&st.invalidated) != 0 {
		t.Errorf("calls=%d invalidated=%d, want 1/0", atomic.LoadInt32(&n), atomic.LoadInt32(&st.invalidated))
	}
}

func TestDo_MarshalErrorNeverHitsServer(t *testing.T) {
	var n int32
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		_, _ = w.Write([]byte(`{"result":{}}`))
	})
	defer done()
	err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "/x", Body: make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "encode body") {
		t.Fatalf("want encode error, got %v", err)
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Errorf("server was hit %d times; must be 0", atomic.LoadInt32(&n))
	}
}

func TestDo_NoContentTypeWithoutBody(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "" {
			t.Errorf("Content-Type = %q, want empty", ct)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer done()
	if err := c.Do(context.Background(), Request{Method: http.MethodDelete, Path: "/x", AccountSeq: 1}); err != nil {
		t.Fatal(err)
	}
}
EOF
go test ./internal/httpclient/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: Request`, `c.Do`, `undefined: canRetry` 등). `io`, `sync` import 가 없다면 함께 추가해야 한다(다음 스텝에서 처리).

- [x] **Step 2: 구현** — `client.go` 의 `Get`/`do` 를 `Request`/`Do` 기반으로 바꾼다.

`internal/httpclient/client.go` 의 패키지 doc(파일 첫 3~4줄)을 재시도 범위가 드러나도록 정정한다:

```go
// Package httpclient 는 토스 Open API REST 호출의 단일 통로다.
// Bearer 토큰 주입, `{"result": ...}` 봉투 해제, `{"error": {...}}` → APIError 매핑,
// 401 토큰 오류 시 1회 재발급·재시도(GET 과 멱등성 키가 있는 쓰기에 한정)를 담당한다.
// 재시도(429/5xx)·스로틀링·캐싱은 하지 않는다.
package httpclient
```

`// Get 은 GET ...` 주석부터 `func isTokenError` 앞까지를 아래로 교체한다(Edit 도구로 정확히 치환). 쓰기의 재시도 여부를 `bool Idempotent` 플래그가 아니라 `IdempotencyKey string` 값으로 표현한다 — "키 없이 재시도를 켠다" 는 잘못된 상태 자체를 만들 수 없게 한다. `isTokenError` 바로 뒤에 `canRetry` 헬퍼를 추가한다:

```go
// Request 는 한 번의 HTTP 호출을 기술한다.
type Request struct {
	Method     string     // GET/POST/DELETE. 빈 값이면 GET
	Path       string     // 예: /api/v1/orders
	Query      url.Values // nil 가능
	Body       any        // nil 이 아니면 JSON 으로 직렬화해 전송. struct/map 만 — []byte 는 base64 문자열, string 은 따옴표 문자열이 된다
	AccountSeq int64      // 0 이 아니면 X-Tossinvest-Account 헤더
	// IdempotencyKey 는 이 요청이 서버 측 멱등성 키(토스 clientOrderId)를 바디에 담고 있음을 뜻한다.
	// 비어 있지 않을 때만 쓰기 요청을 401 토큰 오류에 1회 재시도한다 — 키 없는 쓰기를 재시도하면
	// 서버가 이미 접수한 주문이 중복될 수 있다. GET 은 이 값과 무관하게 항상 재시도한다.
	IdempotencyKey string
	Out            any // nil 이면 응답 본문을 읽지 않는다(204 포함)
}

// Do 는 Request 를 실행하고 `result` 를 Out 으로 디코딩한다.
// Out 이 nil 이면 본문을 버린다(204 정상). Out 이 nil 이 아닌데 본문이 없거나 result 가 null 이면 에러.
// 4xx/5xx 는 *APIError. 401 이고 code 가 expired-token / invalid-token 이면 토큰을 무효화하고,
// canRetry(GET 또는 IdempotencyKey 있는 쓰기)인 요청만 정확히 1회 재시도한다(멱등성 키 없는 쓰기의 중복 실행 방지).
func (c *Client) Do(ctx context.Context, r Request) error {
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	var payload []byte
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return fmt.Errorf("toss: encode body %s: %w", r.Path, err)
		}
		payload = b
	}
	body, err := c.do(ctx, r, payload, false)
	if err != nil {
		return err
	}
	if r.Out == nil {
		return nil
	}
	if len(body) == 0 {
		return fmt.Errorf("toss: %s: empty response body", r.Path)
	}
	var env resultEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("toss: decode envelope %s: %w", r.Path, err)
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return fmt.Errorf("toss: %s: response has no result", r.Path)
	}
	if err := json.Unmarshal(env.Result, r.Out); err != nil {
		return fmt.Errorf("toss: decode result %s: %w", r.Path, err)
	}
	return nil
}

// Get 은 GET 조회 단축형이다. GET 은 항상 멱등이라 401 토큰 오류에 1회 재시도한다(자세한 규칙은 Do 참고).
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.Do(ctx, Request{Method: http.MethodGet, Path: path, Query: query, Out: out})
}

func (c *Client) do(ctx context.Context, r Request, payload []byte, retried bool) ([]byte, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := c.baseURL + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload) // 재시도 때 다시 읽을 수 있도록 매번 새 Reader
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.AccountSeq != 0 {
		req.Header.Set("X-Tossinvest-Account", strconv.FormatInt(r.AccountSeq, 10))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toss: %s %s: %w", r.Method, r.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("toss: read %s: %w", r.Path, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	apiErr := parseError(resp, body)
	if resp.StatusCode == http.StatusUnauthorized && isTokenError(apiErr.Code) {
		// 서버가 거부한 토큰은 재시도 여부와 무관하게 캐시에서 제거한다(재시도 후 401 이어도 하루 동안 남지 않도록).
		c.tokens.Invalidate(tok)
		if !retried && canRetry(r) {
			return c.do(ctx, r, payload, true)
		}
	}
	return nil, apiErr
}

func isTokenError(code string) bool {
	return code == "expired-token" || code == "invalid-token"
}

// canRetry 는 401 토큰 오류에 요청을 다시 보내도 안전한지 판단한다.
// GET 은 HTTP 정의상 멱등이고, 쓰기는 서버 멱등성 키가 있을 때만 안전하다.
func canRetry(r Request) bool {
	return r.Method == http.MethodGet || r.IdempotencyKey != ""
}
```

import 에 `"bytes"` 를 추가한다. 테스트 파일에는 `"io"` 를 추가한다(`sync`/`sync/atomic` 은 기존 테스트에 이미 있다).

```bash
gofmt -w internal/httpclient && go vet ./internal/httpclient/ && go test ./internal/httpclient/ -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: `29` (기존 15 + 신규 14: Step 1 의 8개 + 코드리뷰 반영 후 추가된 6개 — `TestDo_DefaultsToGetAndRetries`, `TestDo_HeadersSurviveRetry`, `TestDo_IdempotentWriteDoesNotRetryTwice`, `TestDo_WriteNonTokenUnauthorizedNoRetry`, `TestDo_MarshalErrorNeverHitsServer`, `TestDo_NoContentTypeWithoutBody`).

**코드리뷰 반영 메모(Task 2 착수 전 적용, 커밋 `03f9927` 이후):**
- `Idempotent bool` → `IdempotencyKey string`. 콜러가 빈 문자열을 실수로 `true` 로 켤 수 없다.
- `Get` 은 이제 `IdempotencyKey` 를 넘기지 않는다 — GET 은 `canRetry` 에서 메서드로 판별해 항상 재시도.
- 204(빈 바디)인데 `Out != nil` 인 에러 메시지를 `"empty response body"` 로, `result:null` 인 에러 메시지는 기존 `"response has no result"` 로 구분.
- `Body` 필드 주석에 직렬화 함정(`[]byte`→base64, `string`→따옴표 문자열)을 명시.
- 헤더가 재시도에도 유지되는지, 마샬 실패 시 서버를 아예 안 부르는지, 논-토큰 401 은 재시도 안 하는지 커버하는 테스트 6개 추가.

- [x] **Step 3: 커밋**

```bash
git add internal/httpclient && git commit -m "feat(httpclient): Request/Do — POST·DELETE, 계좌 헤더, 204, 멱등성 기반 쓰기 재시도

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

(이후 코드리뷰 반영은 별도 커밋 `fix(httpclient): Idempotent bool → IdempotencyKey, GET 은 항상 재시도 (리뷰 반영)` 로 처리했다.)

---

### Task 2: `internal/fetch` 확장 + `tosstypes.AccountType`

**Files:**
- Modify: `internal/fetch/fetch.go`, `internal/fetch/fetch_test.go`
- Modify: `tosstypes/types.go`, `tosstypes/types_test.go`
- Modify: 5개 그룹 패키지의 `fetch.One`/`fetch.List` 호출부(시그니처 변경 반영)

- [ ] **Step 1: fetch 확장**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && cat > internal/fetch/fetch.go << 'EOF'
// Package fetch 는 그룹 패키지가 공유하는 제네릭 요청 헬퍼다. 검증·쿼리 조립은 호출 측이 하고,
// 여기서는 httpclient 호출과 결과 포인터/슬라이스 반환만 담당한다.
package fetch

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

// One 은 GET 으로 result 객체 하나를 *T 로 디코딩한다. accountSeq 가 0 이 아니면 계좌 헤더를 붙인다.
// GET 은 항상 재시도되므로(httpclient.canRetry) IdempotencyKey 를 넘길 필요가 없다.
func One[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values, accountSeq int64) (*T, error) {
	var out T
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: path, Query: q, AccountSeq: accountSeq, Out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// List 는 GET 으로 result 배열을 []T 로 디코딩한다. 빈 배열은 nil 이 아닌 빈 슬라이스, 실패 시 nil 과 에러.
func List[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values, accountSeq int64) ([]T, error) {
	out := []T{}
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: path, Query: q, AccountSeq: accountSeq, Out: &out}); err != nil {
		return nil, err
	}
	return out, nil
}

// PostOne 은 POST 로 body 를 보내고 result 를 *T 로 디코딩한다.
// clientOrderID 는 body 에 실린 멱등성 키를 그대로 넘긴다(없으면 빈 문자열) — 401 재시도 허용 여부를 결정한다.
func PostOne[T any](ctx context.Context, hc *httpclient.Client, path string, body any, accountSeq int64, clientOrderID string) (*T, error) {
	var out T
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodPost, Path: path, Body: body, AccountSeq: accountSeq, IdempotencyKey: clientOrderID, Out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Send 는 응답 본문을 쓰지 않는 요청(예: DELETE 204)을 보낸다. q 는 필요 없으면 nil.
func Send(ctx context.Context, hc *httpclient.Client, method, path string, q url.Values, body any, accountSeq int64) error {
	return hc.Do(ctx, httpclient.Request{Method: method, Path: path, Query: q, Body: body, AccountSeq: accountSeq})
}
EOF
```

- [ ] **Step 2: fetch 테스트 갱신 + 신규**

기존 `fetch_test.go` 의 `One[item](ctx, hc, "/one", url.Values{...})` 호출에 `, 0` 을 추가하고(4곳), 아래를 파일 끝에 덧붙인다:

```bash
cat >> internal/fetch/fetch_test.go << 'EOF'

func TestOne_SendsAccountHeader(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/h"}, "9", 200, []byte(`{"result":{"symbol":"X"}}`))
	defer done()
	if _, err := One[item](context.Background(), hc, "/h", nil, 9); err != nil {
		t.Fatal(err)
	}
}

func TestPostOne(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/p"}, "3", 200, []byte(`{"result":{"symbol":"P"}}`))
	defer done()
	got, err := PostOne[item](context.Background(), hc, "/p", map[string]string{"a": "b"}, 3, "k1")
	if err != nil || got == nil || got.Symbol != "P" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSend_NoContent(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/d"}, "2", 204, nil)
	defer done()
	if err := Send(context.Background(), hc, http.MethodDelete, "/d", nil, nil, 2); err != nil {
		t.Fatal(err)
	}
}
EOF
```
`net/http` import 를 테스트에 추가한다.

- [ ] **Step 3: testutil 에 헤더 검증 서버 추가**

`internal/testutil/server.go` 에 아래를 덧붙인다:

```go
// NewServerWithHeader 는 NewServer 와 같지만 X-Tossinvest-Account 헤더까지 검증한다.
// wantAccount 가 빈 문자열이면 헤더가 없어야 한다.
func NewServerWithHeader(t *testing.T, want Expect, wantAccount string, status int, body []byte) (*httpclient.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkRequest(t, r, want)
		if got := r.Header.Get("X-Tossinvest-Account"); got != wantAccount {
			t.Errorf("X-Tossinvest-Account = %q, want %q", got, wantAccount)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_, _ = w.Write(body)
		}
	}))
	c := httpclient.New(httpclient.Config{BaseURL: srv.URL, HTTPClient: srv.Client(), Tokens: staticTokens{}})
	return c, srv.Close
}
```
그리고 기존 `NewServer` 의 핸들러 본문 중 경로·쿼리·Authorization 검증 부분을 `checkRequest(t, r, want)` 헬퍼로 추출해 둘이 공유하게 한다:

```go
func checkRequest(t *testing.T, r *http.Request, want Expect) {
	t.Helper()
	if r.URL.Path != want.Path {
		t.Errorf("path = %q, want %q", r.URL.Path, want.Path)
	}
	got := r.URL.Query()
	for k := range want.Query {
		if got.Get(k) != want.Query.Get(k) {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), want.Query.Get(k))
		}
	}
	for k := range got {
		if _, ok := want.Query[k]; !ok {
			t.Errorf("unexpected query %s=%q", k, got.Get(k))
		}
	}
	if r.Header.Get("Authorization") != "Bearer test-token" {
		t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
	}
}
```

- [ ] **Step 4: 5개 그룹 패키지 호출부 갱신** — `fetch.One[...](ctx, c.http, path, q)` → `fetch.One[...](ctx, c.http, path, q, 0)`, `fetch.List` 동일. 기계적 치환:

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && perl -pi -e 's/(fetch\.(?:One|List)\[[^\]]+\]\(ctx, (?:c\.http|hc), [^)]*?)\)/$1, 0)/g' marketdata/*.go stockinfo/*.go marketinfo/*.go ranking/*.go indicators/*.go && gofmt -w . && go build ./... && echo BUILD_OK
```
Expected: `BUILD_OK`. 실패하면 컴파일 에러 메시지의 파일을 열어 수동으로 `, 0` 을 추가한다(정규식이 다중행 호출을 놓칠 수 있다).

- [ ] **Step 5: tosstypes.AccountType 추가**

`tosstypes/types.go` 의 `Currency` 정의 바로 위에 추가:
```go
// AccountType 은 계좌 종류.
type AccountType string

const (
	AccountTypeBrokerage            AccountType = "BROKERAGE"             // 위탁(주식)
	AccountTypeOverseasDerivatives  AccountType = "OVERSEAS_DERIVATIVES"  // 해외파생
	AccountTypePensionSavings       AccountType = "PENSION_SAVINGS"       // 연금저축
	AccountTypeReshoringInvestment  AccountType = "RESHORING_INVESTMENT"  // 국내복귀기업 투자
)
```

- [ ] **Step 6: 검증 + 커밋**

```bash
gofmt -l . ; go vet ./... && go test ./... -race -count=1 2>&1 | tail -14
git add internal tosstypes marketdata stockinfo marketinfo ranking indicators && git commit -m "feat(fetch): 계좌 헤더·PostOne·Send 추가, AccountType 열거

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
Expected: 전 패키지 `ok`.

---

### Task 3: 루트 — `Account`/`Accounts`/`AccountScope`/`NewClientOrderID`/에러 코드 상수

**Files:**
- Create: `accounts.go`, `account.go`, `clientorderid.go`, `clientorderid_test.go`, `codes.go`, `account_test.go`
- Create: `asset/client.go`, `asset/holdings.go`, `asset/asset_test.go`, `asset/testdata/holdings.json`
- Modify: `client.go` (import 3개 추가는 Task 4·5 이후 — 이 태스크에서는 `asset` 만 연결)

> 이 태스크는 스코프 뼈대 + 가장 단순한 그룹(`asset` 1 op)까지 만들어 구조를 먼저 검증한다.
> `order`/`conditionalorder` 는 Task 4·5 에서 추가하며, 그때 `AccountScope` 에 필드를 덧붙인다.

- [ ] **Step 1: fixture 추출**

```bash
mkdir -p asset/testdata && jq '.paths."/api/v1/holdings".get.responses."200".content."application/json".examples.withHoldings.value' docs/api/openapi.json > asset/testdata/holdings.json && jq '.paths."/api/v1/holdings".get.responses."200".content."application/json".examples.emptyHoldings.value' docs/api/openapi.json > asset/testdata/holdings_empty.json && jq '.paths."/api/v1/holdings".get.responses."200".content."application/json".examples.filteredByUsSymbol.value' docs/api/openapi.json > asset/testdata/holdings_us.json && jq -c '.result | {n:(.items|length), krw:.totalPurchaseAmount.krw}' asset/testdata/holdings.json && jq -c '.result | {n:(.items|length)}' asset/testdata/holdings_empty.json
```
Expected: `{"n":2,"krw":"6500000"}` 와 `{"n":0}`. (fixture 는 `withHoldings` 예시에 KR+US 2건이 들어 있다; `docs/api/openapi.json` 의 예시 데이터가 계획 작성 시점 이후 갱신되었다.)

- [ ] **Step 2: 실패 테스트 작성**

```bash
cat > clientorderid_test.go << 'EOF'
package toss

import (
	"strings"
	"testing"
)

func TestNewClientOrderID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewClientOrderID()
		if err := ValidateClientOrderID(id); err != nil {
			t.Fatalf("generated id %q invalid: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestValidateClientOrderID(t *testing.T) {
	for _, ok := range []string{"a", "A-b_C9", strings.Repeat("x", 36)} {
		if err := ValidateClientOrderID(ok); err != nil {
			t.Errorf("ValidateClientOrderID(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 37), "has space", "한글", "a.b", "a/b"} {
		if err := ValidateClientOrderID(bad); err == nil {
			t.Errorf("ValidateClientOrderID(%q) must fail", bad)
		}
	}
}
EOF
cat > account_test.go << 'EOF'
package toss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kenshin579/toss-go/asset"
)

// 계좌 목록은 헤더 없이, 스코프 호출은 헤더와 함께 나가는지 end-to-end 로 확인한다.
func TestAccountsAndScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"T","token_type":"Bearer","expires_in":3600}`))
		case "/api/v1/accounts":
			if got := r.Header.Get("X-Tossinvest-Account"); got != "" {
				t.Errorf("accounts must not send account header, got %q", got)
			}
			_, _ = w.Write([]byte(`{"result":[{"accountNo":"12345678901","accountSeq":7,"accountType":"BROKERAGE"}]}`))
		case "/api/v1/holdings":
			if got := r.Header.Get("X-Tossinvest-Account"); got != "7" {
				t.Errorf("holdings account header = %q, want 7", got)
			}
			_, _ = w.Write([]byte(`{"result":{"totalPurchaseAmount":{"krw":"1"},"marketValue":{"amount":{"krw":"1"},"amountAfterCost":{"krw":"1"}},"profitLoss":{"amount":{"krw":"0"},"amountAfterCost":{"krw":"0"},"rate":"0","rateAfterCost":"0"},"dailyProfitLoss":{"amount":{"krw":"0"},"rate":"0"},"items":[]}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := NewClient("i", "s", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	accts, err := c.Accounts(ctx)
	if err != nil || len(accts) != 1 {
		t.Fatalf("Accounts = %+v, %v", accts, err)
	}
	a := accts[0]
	if a.AccountNo != "12345678901" || a.AccountSeq != 7 || a.AccountType != "BROKERAGE" {
		t.Errorf("account = %+v", a)
	}
	scope := c.Account(a.AccountSeq)
	if scope.Asset == nil {
		t.Fatal("scope.Asset is nil")
	}
	if _, err := scope.Asset.Holdings(ctx, asset.HoldingsParams{}); err != nil {
		t.Fatalf("Holdings: %v", err)
	}
}

func TestAccount_ZeroSeqIsRejected(t *testing.T) {
	// accountSeq 0 은 httpclient 가 헤더를 생략해 서버가 account-header-required 를 돌려준다.
	// 요청 전에 실패시키는 편이 원인을 명확히 한다.
	c, err := NewClient("i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Account(0).Asset.Holdings(context.Background(), asset.HoldingsParams{}); err == nil {
		t.Error("Account(0) 사용 시 에러를 기대")
	}
	if _, err := c.Account(-1).Asset.Holdings(context.Background(), asset.HoldingsParams{}); err == nil {
		t.Error("음수 accountSeq 사용 시 에러를 기대")
	}
}

func TestAccountScope_ConcurrentUse(t *testing.T) {
	// AccountScope 는 여러 goroutine 에서 동시 사용해도 안전해야 한다(문서화된 약속).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"T","token_type":"Bearer","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"totalPurchaseAmount":{"krw":"1"},"marketValue":{"amount":{"krw":"1"},"amountAfterCost":{"krw":"1"}},"profitLoss":{"amount":{"krw":"0"},"amountAfterCost":{"krw":"0"},"rate":"0","rateAfterCost":"0"},"dailyProfitLoss":{"amount":{"krw":"0"},"rate":"0"},"items":[]}}`))
	}))
	defer srv.Close()
	c, err := NewClient("i", "s", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	scope := c.Account(7)
	if scope.AccountSeq() != 7 {
		t.Errorf("AccountSeq() = %d", scope.AccountSeq())
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := scope.Asset.Holdings(context.Background(), asset.HoldingsParams{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
EOF
cat > asset/asset_test.go << 'EOF'
package asset

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestHoldings(t *testing.T) {
	// fixture = openapi 예시(withHoldings)
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings"}, "5", 200, testutil.Fixture(t, "holdings.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), HoldingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if h.TotalPurchaseAmount.KRW.String() != "6500000" || h.TotalPurchaseAmount.USD == nil || h.TotalPurchaseAmount.USD.String() != "1553" {
		t.Errorf("TotalPurchaseAmount = %+v", h.TotalPurchaseAmount)
	}
	if h.MarketValue.Amount.KRW.String() != "7200000" || h.ProfitLoss.Rate.String() != "0.1179" || h.DailyProfitLoss.Rate.String() != "0.0141" {
		t.Errorf("overview = %+v", h)
	}
	if len(h.Items) != 2 {
		t.Fatalf("items = %d", len(h.Items))
	}
	it := h.Items[0]
	if it.Symbol != "005930" || it.Name != "삼성전자" || it.MarketCountry != tosstypes.MarketCountryKR || it.Currency != tosstypes.CurrencyKRW {
		t.Errorf("item = %+v", it)
	}
	if it.Quantity.String() != "100" || it.LastPrice.String() != "72000" || it.AveragePurchasePrice.String() != "65000" {
		t.Errorf("item numbers = %+v", it)
	}
	if it.MarketValue.PurchaseAmount.String() != "6500000" || it.ProfitLoss.RateAfterCost.String() != "0.0846" || it.Cost.Commission.String() != "14400" {
		t.Errorf("item nested = %+v", it)
	}
	if it.Cost.Tax == nil || it.Cost.Tax.String() != "135600" {
		t.Errorf("tax = %v", it.Cost.Tax)
	}
	us := h.Items[1]
	if us.Symbol != "AAPL" || us.MarketCountry != tosstypes.MarketCountryUS || us.Currency != tosstypes.CurrencyUSD {
		t.Errorf("us item = %+v", us)
	}
	if us.LastPrice.String() != "178.5" || us.MarketValue.AmountAfterCost.String() != "1771.43" {
		t.Errorf("us decimals = %+v", us)
	}
	if us.Cost.Tax == nil {
		t.Errorf("us tax must be present in this fixture: %+v", us.Cost)
	}
}

func TestHoldings_Empty(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings"}, "5", 200, testutil.Fixture(t, "holdings_empty.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), HoldingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Items) != 0 {
		t.Errorf("items = %+v", h.Items)
	}
}

func TestHoldings_SymbolFilter(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings", Query: url.Values{"symbol": {"005930"}}}, "5", 200, testutil.Fixture(t, "holdings.json"))
	defer done()
	if _, err := New(hc, 5).Holdings(context.Background(), HoldingsParams{Symbol: "005930"}); err != nil {
		t.Fatal(err)
	}
}

func TestHoldings_InvalidSymbol(t *testing.T) {
	if _, err := New(nil, 5).Holdings(context.Background(), HoldingsParams{Symbol: "삼성"}); err == nil {
		t.Error("want validation error")
	}
}

func TestHoldings_NullableTax(t *testing.T) {
	// fixture = openapi 예시(filteredByUsSymbol): 미국 종목은 매도 세금이 없어 cost.tax 가 null
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings", Query: url.Values{"symbol": {"AAPL"}}}, "5", 200, testutil.Fixture(t, "holdings_us.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), HoldingsParams{Symbol: "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Items) != 1 {
		t.Fatalf("items = %d", len(h.Items))
	}
	if h.Items[0].Cost.Tax != nil {
		t.Errorf("tax must be nil, got %v", h.Items[0].Cost.Tax)
	}
}
EOF
go test ./asset/ . 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: New`, `Accounts` 등). 테스트 파일에 `net/url` import 를 추가해야 한다(다음 스텝에서 함께).

- [ ] **Step 3: 구현**

```bash
cat > clientorderid.go << 'EOF'
package toss

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/kenshin579/toss-go/internal/params"
)

// MaxClientOrderIDLen 은 clientOrderId 최대 길이(토스 규칙).
const MaxClientOrderIDLen = params.MaxClientOrderIDLen

// NewClientOrderID 는 멱등성 키로 쓸 새 clientOrderId 를 만든다(32자, URL-safe).
//
// 주문 생성/조건주문 생성에 이 값을 넣으면 (1) 같은 값으로 재요청할 때 토스가 이전 주문 결과를
// 그대로 돌려주고(10분 유효), (2) SDK 가 401 토큰 오류에 요청을 1회 재시도한다.
// 키가 없으면 SDK 는 쓰기 요청을 재시도하지 않는다 — 중복 주문을 만들지 않기 위해서다.
//
// crypto/rand 가 실패하면 panic 한다(Go 1.24+ 에서는 발생하지 않는다). 약한 난수로 대체하면
// 키가 충돌해 서로 다른 주문이 하나로 합쳐질 수 있어 실패를 감추지 않는다.
func NewClientOrderID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("toss: crypto/rand failed: " + err.Error()) // 이 실패는 프로세스가 정상 동작할 수 없는 상태다
	}
	return base64.RawURLEncoding.EncodeToString(b[:]) // 24바이트 → 32자
}

// ValidateClientOrderID 는 clientOrderId 형식을 검증한다(1~36자, 영숫자와 -, _).
func ValidateClientOrderID(id string) error {
	if id == "" {
		return fmt.Errorf("toss: clientOrderId must not be empty")
	}
	return params.ClientOrderIDFormat(id)
}
EOF
cat > accounts.go << 'EOF'
package toss

import (
	"context"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Account 는 계좌 하나의 식별 정보.
type Account struct {
	AccountNo   string                `json:"accountNo"`   // 계좌번호
	AccountSeq  int64                 `json:"accountSeq"`  // 계좌 일련번호. Client.Account 와 X-Tossinvest-Account 헤더에 쓴다
	AccountType tosstypes.AccountType `json:"accountType"` // 계좌 종류
}

// Accounts 는 계좌 목록을 조회한다(GET /api/v1/accounts).
// 계좌 헤더가 필요 없는 유일한 계좌 API 이며, 여기서 얻은 AccountSeq 로 Client.Account 를 만든다.
// Rate limit 그룹 ACCOUNT(초당 1회)이므로 반복 호출하지 말고 결과를 재사용한다.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	return fetchList[Account](ctx, c, "/api/v1/accounts")
}
EOF
cat > account.go << 'EOF'
package toss

import (
	"context"

	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/conditionalorder"
	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/order"
)

// AccountScope 는 특정 계좌(accountSeq)에 고정된 sub-client 묶음이다.
// 이 아래의 모든 요청에는 X-Tossinvest-Account 헤더가 자동으로 실린다.
// 여러 goroutine 에서 동시에 사용해도 안전하다.
type AccountScope struct {
	accountSeq int64

	Asset            *asset.Client            // 자산: 보유 주식
	Order            *order.Client            // 주문: 생성·정정·취소·조회·주문 정보
	ConditionalOrder *conditionalorder.Client // 조건주문: 생성·수정·취소·조회
}

// AccountSeq 는 이 스코프가 고정된 계좌 일련번호를 돌려준다. 생성 후 바뀌지 않는다.
func (a *AccountScope) AccountSeq() int64 { return a.accountSeq }

// Account 는 accountSeq 에 고정된 스코프를 만든다. 네트워크 호출은 없다.
// accountSeq 는 Accounts 로 조회한다. 0 이하를 넘기면 스코프의 모든 호출이 요청 전에 에러를 낸다
// (계좌 헤더 없이 요청하면 서버가 account-header-required 를 돌려주므로 미리 막는다).
//
//	accts, _ := c.Accounts(ctx)
//	a := c.Account(accts[0].AccountSeq)
//	h, _ := a.Asset.Holdings(ctx, asset.HoldingsParams{})
func (c *Client) Account(accountSeq int64) *AccountScope {
	return &AccountScope{
		accountSeq:       accountSeq,
		Asset:            asset.New(c.http, accountSeq),
		Order:            order.New(c.http, accountSeq),
		ConditionalOrder: conditionalorder.New(c.http, accountSeq),
	}
}

// fetchList 는 루트에서 계좌 헤더 없이 목록을 조회한다.
func fetchList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	return fetch.List[T](ctx, c.http, path, nil, 0)
}
EOF
```
> `account.go` 는 `c.http` 를 쓴다. v0.1.0 에서 미사용이라 지웠던 필드를 되살려야 한다 —
> `client.go` 의 `Client` 구조체에 `http *httpclient.Client` 를 추가하고 `NewClient` 의 반환 리터럴에
> `http: h,` 를 넣는다. `account.go` 의 import 에서 `httpclient` 는 쓰지 않으므로 넣지 않는다.

```bash
cat > codes.go << 'EOF'
package toss

// 자주 마주치는 토스 에러 코드. IsCode 와 함께 쓴다.
//
//	if toss.IsCode(err, toss.CodeInsufficientBuyingPower) { ... }
//
// 토스는 코드를 예고 없이 추가할 수 있으므로 이 목록은 편의일 뿐 전수가 아니다.
// 알 수 없는 코드도 그대로 *APIError.Code 에 담긴다.
const (
	CodeAccountHeaderRequired    = "account-header-required"
	CodeAccountNotFound          = "account-not-found"
	CodeAlreadyCanceled          = "already-canceled"
	CodeAlreadyFilled            = "already-filled"
	CodeConfirmHighValueRequired = "confirm-high-value-required"
	CodeInsufficientBuyingPower  = "insufficient-buying-power"
	CodeOrderNotFound            = "order-not-found"
	CodeOrderHoursClosed         = "order-hours-closed"
	CodePriceOutOfRange          = "price-out-of-range"
	CodeStockRestricted          = "stock-restricted"
	CodeRateLimitExceeded        = "rate-limit-exceeded"
	CodeIdempotencyKeyConflict   = "idempotency-key-conflict"
	CodeStockNotFound            = "stock-not-found"
	CodeRequestInProgress        = "request-in-progress"
	CodeAlreadyModified          = "already-modified"
	CodeAlreadyProcessing        = "already-processing"
	CodeModifyRestricted         = "modify-restricted"
	CodeCancelRestricted         = "cancel-restricted"
	CodeMaxOrderAmountExceeded   = "max-order-amount-exceeded"
	CodeConditionalOrderNotFound = "conditional-order-not-found"
)
EOF
mkdir -p asset && cat > asset/client.go << 'EOF'
// Package asset 은 토스 Open API 자산(Asset) 그룹 — 보유 주식.
// toss.Client.Account(seq).Asset 으로 접근하며, 모든 요청에 계좌 헤더가 실린다.
package asset

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 자산 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
EOF
cat > asset/holdings.go << 'EOF'
package asset

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Amount 는 원화·달러 동시 표기 금액. USD 는 국내 전용 계좌 등에서 nil 일 수 있다.
type Amount struct {
	KRW decimal.Decimal  `json:"krw"`
	USD *decimal.Decimal `json:"usd"`
}

// OverviewMarketValue 는 계좌 전체 평가금액.
type OverviewMarketValue struct {
	Amount          Amount `json:"amount"`          // 평가금액
	AmountAfterCost Amount `json:"amountAfterCost"` // 비용(수수료·세금) 차감 후 평가금액
}

// OverviewProfitLoss 는 계좌 전체 손익.
type OverviewProfitLoss struct {
	Amount          Amount          `json:"amount"`
	AmountAfterCost Amount          `json:"amountAfterCost"`
	Rate            decimal.Decimal `json:"rate"`          // 수익률(소수 비율, 0.1179 = 11.79%)
	RateAfterCost   decimal.Decimal `json:"rateAfterCost"` // 비용 차감 후 수익률
}

// OverviewDailyProfitLoss 는 계좌 전체 일간 손익.
type OverviewDailyProfitLoss struct {
	Amount Amount          `json:"amount"`
	Rate   decimal.Decimal `json:"rate"`
}

// MarketValue 는 종목별 평가금액.
type MarketValue struct {
	PurchaseAmount  decimal.Decimal `json:"purchaseAmount"`  // 매입금액
	Amount          decimal.Decimal `json:"amount"`          // 평가금액
	AmountAfterCost decimal.Decimal `json:"amountAfterCost"` // 비용 차감 후 평가금액
}

// ProfitLoss 는 종목별 손익.
type ProfitLoss struct {
	Amount          decimal.Decimal `json:"amount"`
	AmountAfterCost decimal.Decimal `json:"amountAfterCost"`
	Rate            decimal.Decimal `json:"rate"`
	RateAfterCost   decimal.Decimal `json:"rateAfterCost"`
}

// DailyProfitLoss 는 종목별 일간 손익.
type DailyProfitLoss struct {
	Amount decimal.Decimal `json:"amount"`
	Rate   decimal.Decimal `json:"rate"`
}

// Cost 는 매도 시 예상 비용. Tax 는 미국 주식 등 해당 없으면 nil.
type Cost struct {
	Commission decimal.Decimal  `json:"commission"`
	Tax        *decimal.Decimal `json:"tax"`
}

// Item 은 보유 종목 1건.
type Item struct {
	Symbol               string                  `json:"symbol"`
	Name                 string                  `json:"name"`
	MarketCountry        tosstypes.MarketCountry `json:"marketCountry"`
	Currency             tosstypes.Currency      `json:"currency"`
	Quantity             decimal.Decimal         `json:"quantity"`
	LastPrice            decimal.Decimal         `json:"lastPrice"`
	AveragePurchasePrice decimal.Decimal         `json:"averagePurchasePrice"`
	MarketValue          MarketValue             `json:"marketValue"`
	ProfitLoss           ProfitLoss              `json:"profitLoss"`
	DailyProfitLoss      DailyProfitLoss         `json:"dailyProfitLoss"`
	Cost                 Cost                    `json:"cost"`
}

// Holdings 는 계좌 전체 요약과 보유 종목 목록.
type Holdings struct {
	TotalPurchaseAmount Amount                  `json:"totalPurchaseAmount"`
	MarketValue         OverviewMarketValue     `json:"marketValue"`
	ProfitLoss          OverviewProfitLoss      `json:"profitLoss"`
	DailyProfitLoss     OverviewDailyProfitLoss `json:"dailyProfitLoss"`
	Items               []Item                  `json:"items"`
}

// HoldingsParams 는 Holdings 의 선택 인자.
type HoldingsParams struct {
	Symbol string // 특정 종목만 조회. 비우면 전체. 지정하면 요약 필드도 그 종목 기준으로 재계산된다
}

// Holdings 는 보유 주식을 조회한다(GET /api/v1/holdings). 인자의 zero value 면 전체를 조회한다.
// Symbol 을 지정하면 Items 뿐 아니라 TotalPurchaseAmount·MarketValue 등 요약 필드도 그 종목 기준으로 재계산된다.
// 보유하지 않은 종목을 지정하면 Items 가 빈 슬라이스로 돌아온다.
func (c *Client) Holdings(ctx context.Context, p HoldingsParams) (*Holdings, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	q := url.Values{}
	if p.Symbol != "" {
		if err := params.Symbol(p.Symbol); err != nil {
			return nil, err
		}
		q.Set("symbol", p.Symbol)
	}
	return fetch.One[Holdings](ctx, c.http, "/api/v1/holdings", q, c.accountSeq)
}
EOF
gofmt -w . && go vet ./... && go test ./asset/ . -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: `13` (루트 8 기존 + `TestAccountsAndScope` + `TestNewClientOrderID` + `TestValidateClientOrderID` = 11, asset 4 → 합 15). **정확한 수는 실행 결과로 확인하고, 모두 PASS 인지만 본다.**

- [ ] **Step 4: 커밋**

```bash
git add accounts.go account.go clientorderid.go clientorderid_test.go codes.go account_test.go client.go asset && git commit -m "feat(account): 계좌 목록·스코프 클라이언트·멱등성 키 + 보유 주식(asset)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: `order` 패키지 (8 ops)

**Files:**
- Create: `order/client.go`, `order/types.go`, `order/place.go`, `order/modify.go`, `order/history.go`, `order/info.go`, `order/order_test.go`, `order/testdata/*.json`
- Modify: `account.go` (`AccountScope.Order` 추가), `internal/testutil/server.go` (`NewServerFunc` 추가)

- [x] **Step 1: fixture 추출**

```bash
mkdir -p order/testdata && J=docs/api/openapi.json
ex() { jq --arg p "$1" --arg k "$2" '.paths[$p].get.responses."200".content."application/json".examples[$k].value' "$J" > "order/testdata/$3.json"; }
ex "/api/v1/orders" pendingMixed orders
ex "/api/v1/orders" empty orders_empty
ex "/api/v1/orders" completedWithNextPage orders_page
ex "/api/v1/orders/{orderId}" krLimitFilled order_filled
ex "/api/v1/orders/{orderId}" usMarketPartialFilled order_partial
ex "/api/v1/orders/{orderId}" rejected order_rejected
ex "/api/v1/buying-power" krw buying_power
ex "/api/v1/sellable-quantity" kr sellable_quantity
ex "/api/v1/commissions" standard commissions
for f in order/testdata/*.json; do printf "%-40s " "$f"; jq -c 'if .result|type=="array" then {n:(.result|length)} else (.result|keys|join(",")) end' "$f"; done
```
Expected: 9개 파일. `orders.json` 은 `orders,nextCursor,hasNext`, `order_filled.json` 은 `orderId,symbol,...`, `commissions.json` 은 `{"n":2}`.
`order_partial.json`(usMarketPartialFilled)·`order_rejected.json`(rejected) 은 `TestGet_USPartialFilled`/`TestGet_Rejected` 가 소비한다(둘 다 초안에서 미사용이었다가 리뷰로 추가).

- [x] **Step 2: 실패 테스트 작성**

```bash
cat > order/order_test.go << 'EOF'
package order

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// --- 조회 ---

func TestList(t *testing.T) {
	// fixture = openapi 예시(pendingMixed): 2건, hasNext=false
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{"status": {"OPEN"}}}, "3", 200, testutil.Fixture(t, "orders.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{Status: StatusFilterOpen})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 2 || page.HasNext || page.NextCursor != nil {
		t.Fatalf("page = %+v", page)
	}
	o := page.Orders[0]
	if o.OrderID == "" || o.Symbol != "005930" || o.Side != SideBuy || o.OrderType != TypeLimit || o.TimeInForce != TimeInForceDay || o.Status != StatusPending {
		t.Errorf("order = %+v", o)
	}
	if o.Price == nil || o.Price.String() != "70000" || o.Quantity.String() != "10" || o.OrderAmount != nil || o.Currency != tosstypes.CurrencyKRW {
		t.Errorf("order numbers = %+v", o)
	}
	if o.OrderedAt.IsZero() || o.CanceledAt != nil {
		t.Errorf("times = %v %v", o.OrderedAt, o.CanceledAt)
	}
	if o.Execution.FilledQuantity.String() != "0" || o.Execution.AverageFilledPrice != nil || o.Execution.FilledAt != nil || o.Execution.SettlementDate != nil {
		t.Errorf("execution = %+v", o.Execution)
	}
}

func TestList_AllParams(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{
		"status": {"CLOSED"}, "symbol": {"005930"}, "from": {"2026-09-01"}, "to": {"2026-09-04"}, "cursor": {"c1"}, "limit": {"50"},
	}}, "3", 200, testutil.Fixture(t, "orders_empty.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{
		Status: StatusFilterClosed, Symbol: "005930", From: "2026-09-01", To: "2026-09-04", Cursor: "c1", Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 0 {
		t.Errorf("orders = %+v", page.Orders)
	}
}

func TestList_NextCursor(t *testing.T) {
	// fixture = openapi 예시(completedWithNextPage): hasNext=true, nextCursor 존재
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{"status": {"CLOSED"}}}, "3", 200, testutil.Fixture(t, "orders_page.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{Status: StatusFilterClosed})
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
		t.Errorf("page = %+v", page)
	}
}

func TestList_RequiresStatus(t *testing.T) {
	if _, err := New(nil, 3).List(context.Background(), ListParams{}); err == nil {
		t.Error("want error for empty status")
	}
}

func TestGet(t *testing.T) {
	// fixture = openapi 예시(krLimitFilled)
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders/o-1"}, "3", 200, testutil.Fixture(t, "order_filled.json"))
	defer done()
	o, err := New(hc, 3).Get(context.Background(), "o-1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusFilled || o.Execution.FilledQuantity.String() != "10" {
		t.Errorf("order = %+v", o)
	}
	if o.Execution.AverageFilledPrice == nil || o.Execution.AverageFilledPrice.String() != "70000" {
		t.Errorf("avg = %v", o.Execution.AverageFilledPrice)
	}
	if o.Execution.Commission == nil || o.Execution.Commission.String() != "1400" || o.Execution.Tax == nil || o.Execution.Tax.String() != "0" {
		t.Errorf("cost = %+v", o.Execution)
	}
	if o.Execution.FilledAt == nil || o.Execution.SettlementDate == nil || *o.Execution.SettlementDate != "2026-03-30" {
		t.Errorf("settlement = %v %v", o.Execution.FilledAt, o.Execution.SettlementDate)
	}
}

func TestGet_RequiresID(t *testing.T) {
	if _, err := New(nil, 3).Get(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestGet_USPartialFilled(t *testing.T) {
	// fixture = openapi 예시(usMarketPartialFilled): 미국 시장가 매수, 소수점 없는 수량 5주 중 3주 체결
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders/o-2"}, "3", 200, testutil.Fixture(t, "order_partial.json"))
	defer done()
	o, err := New(hc, 3).Get(context.Background(), "o-2")
	if err != nil {
		t.Fatal(err)
	}
	if o.Symbol != "AAPL" || o.Side != SideBuy || o.OrderType != TypeMarket || o.Currency != tosstypes.CurrencyUSD {
		t.Errorf("order = %+v", o)
	}
	if o.Status != StatusPartialFilled || o.Price != nil || o.Quantity.String() != "5" || o.OrderAmount != nil {
		t.Errorf("order fields = %+v", o)
	}
	if o.Execution.FilledQuantity.String() != "3" || o.Execution.AverageFilledPrice == nil || o.Execution.AverageFilledPrice.String() != "185.25" {
		t.Errorf("execution = %+v", o.Execution)
	}
	if o.Execution.Commission == nil || o.Execution.Commission.String() != "0.99" || o.Execution.Tax == nil || o.Execution.Tax.String() != "0" {
		t.Errorf("cost = %+v", o.Execution)
	}
	if o.Execution.FilledAt == nil || o.Execution.SettlementDate != nil {
		t.Errorf("filledAt/settlementDate = %v %v", o.Execution.FilledAt, o.Execution.SettlementDate)
	}
}

func TestGet_Rejected(t *testing.T) {
	// fixture = openapi 예시(rejected): 미국 시장가, 소수점 수량 0.5주, 체결 없이 거부
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders/o-3"}, "3", 200, testutil.Fixture(t, "order_rejected.json"))
	defer done()
	o, err := New(hc, 3).Get(context.Background(), "o-3")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusRejected || o.Quantity.String() != "0.5" || o.Price != nil || o.Currency != tosstypes.CurrencyUSD {
		t.Errorf("order = %+v", o)
	}
	if o.Execution.FilledQuantity.String() != "0" || o.Execution.AverageFilledPrice != nil || o.Execution.FilledAmount != nil {
		t.Errorf("execution = %+v", o.Execution)
	}
	if o.Execution.Commission != nil || o.Execution.Tax != nil || o.Execution.FilledAt != nil || o.Execution.SettlementDate != nil {
		t.Errorf("execution nulls = %+v", o.Execution)
	}
}

func TestBuyingPower(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/buying-power", Query: url.Values{"currency": {"KRW"}}}, "3", 200, testutil.Fixture(t, "buying_power.json"))
	defer done()
	bp, err := New(hc, 3).BuyingPower(context.Background(), tosstypes.CurrencyKRW)
	if err != nil {
		t.Fatal(err)
	}
	if bp.Currency != tosstypes.CurrencyKRW || bp.CashBuyingPower.String() != "5000000" {
		t.Errorf("bp = %+v", bp)
	}
}

func TestBuyingPower_RequiresCurrency(t *testing.T) {
	if _, err := New(nil, 3).BuyingPower(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestSellableQuantity(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/sellable-quantity", Query: url.Values{"symbol": {"005930"}}}, "3", 200, testutil.Fixture(t, "sellable_quantity.json"))
	defer done()
	s, err := New(hc, 3).SellableQuantity(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if s.SellableQuantity.String() != "100" {
		t.Errorf("sellable = %+v", s)
	}
}

func TestCommissions(t *testing.T) {
	// fixture = openapi 예시(standard): KR/US 2건
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/commissions"}, "3", 200, testutil.Fixture(t, "commissions.json"))
	defer done()
	cs, err := New(hc, 3).Commissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("commissions = %+v", cs)
	}
	if cs[0].MarketCountry != tosstypes.MarketCountryKR || cs[0].CommissionRate.String() != "0.00015" {
		t.Errorf("kr = %+v", cs[0])
	}
	if cs[0].StartDate == nil || *cs[0].StartDate != "2026-01-01" || cs[1].StartDate != nil {
		t.Errorf("dates = %v %v", cs[0].StartDate, cs[1].StartDate)
	}
}

// --- 계좌 검증 (accountSeq <= 0 은 요청 전에 실패해야 한다) ---

func TestZeroAccountSeq(t *testing.T) {
	// http 는 nil — 검증을 통과해 실제로 요청을 보내려 하면 nil pointer dereference 로 즉시 드러난다.
	c := New(nil, 0)
	ctx := context.Background()
	if _, err := c.List(ctx, ListParams{Status: StatusFilterOpen}); err == nil {
		t.Error("List: want error for accountSeq=0")
	}
	if _, err := c.Get(ctx, "o-1"); err == nil {
		t.Error("Get: want error for accountSeq=0")
	}
	if _, err := c.BuyingPower(ctx, tosstypes.CurrencyKRW); err == nil {
		t.Error("BuyingPower: want error for accountSeq=0")
	}
	if _, err := c.SellableQuantity(ctx, "005930"); err == nil {
		t.Error("SellableQuantity: want error for accountSeq=0")
	}
	if _, err := c.Commissions(ctx); err == nil {
		t.Error("Commissions: want error for accountSeq=0")
	}
	if _, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("10"), Price: d("70000")}); err == nil {
		t.Error("Place: want error for accountSeq=0")
	}
	if _, err := c.PlaceAmount(ctx, AmountRequest{Symbol: "AAPL", Side: SideBuy, OrderAmount: d("100")}); err == nil {
		t.Error("PlaceAmount: want error for accountSeq=0")
	}
	price, qty := d("71000"), d("5")
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Price: &price, Quantity: &qty}); err == nil {
		t.Error("Modify: want error for accountSeq=0")
	}
	if _, err := c.Cancel(ctx, "o-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=0")
	}

	cNeg := New(nil, -1)
	if _, err := cNeg.Cancel(ctx, "o-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=-1")
	}
}

// --- 쓰기(요청 조립만 검증. 실주문은 절대 내지 않는다) ---

// captureBody 는 요청 바디를 그대로 돌려주는 스텁이다.
func captureBody(t *testing.T, path, wantAccount string, respond []byte) (*Client, *string, func()) {
	t.Helper()
	var got string
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if a := r.Header.Get("X-Tossinvest-Account"); a != wantAccount {
			t.Errorf("account = %q, want %q", a, wantAccount)
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(respond)
	})
	return New(hc, 3), &got, done
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got is not JSON: %s", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %s", want)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("body =\n  %s\nwant\n  %s", gb, wb)
	}
}

func TestPlace_LimitBuy(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-9","clientOrderId":"k1"}}`))
	defer done()
	res, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "005930", Side: SideBuy, OrderType: TypeLimit,
		Quantity: d("10"), Price: d("70000"), ClientOrderID: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","side":"BUY","orderType":"LIMIT","quantity":"10","price":"70000","clientOrderId":"k1"}`)
	if res.OrderID != "o-9" || res.ClientOrderID == nil || *res.ClientOrderID != "k1" {
		t.Errorf("res = %+v", res)
	}
}

func TestPlace_MarketSellOmitsPriceAndTIF(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-10"}}`))
	defer done()
	if _, err := c.Place(context.Background(), PlaceRequest{Symbol: "AAPL", Side: SideSell, OrderType: TypeMarket, Quantity: d("1.5")}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"AAPL","side":"SELL","orderType":"MARKET","quantity":"1.5"}`)
}

func TestPlace_TimeInForceAndConfirm(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-11"}}`))
	defer done()
	if _, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "AAPL", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("200"),
		TimeInForce: TimeInForceClose, ConfirmHighValue: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":"1","price":"200","timeInForce":"CLS","confirmHighValueOrder":true}`)
}

func TestPlaceAmount(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-12"}}`))
	defer done()
	if _, err := c.PlaceAmount(context.Background(), AmountRequest{Symbol: "AAPL", Side: SideBuy, OrderAmount: d("100"), ClientOrderID: "k2"}); err != nil {
		t.Fatal(err)
	}
	// orderType 은 SDK 가 MARKET 으로 채운다(스키마상 유일값)
	assertJSON(t, *body, `{"symbol":"AAPL","side":"BUY","orderType":"MARKET","orderAmount":"100","clientOrderId":"k2"}`)
}

func TestPlace_Validation(t *testing.T) {
	c := New(nil, 3) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	cases := map[string]PlaceRequest{
		"empty symbol":      {Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"bad symbol":        {Symbol: "삼성", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"empty side":        {Symbol: "005930", OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"empty type":        {Symbol: "005930", Side: SideBuy, Quantity: d("1"), Price: d("1")},
		"zero quantity":     {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Price: d("1")},
		"neg quantity":      {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("-1"), Price: d("1")},
		"limit no price":    {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1")},
		"bad key":           {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1"), ClientOrderID: "has space"},
		"market with price": {Symbol: "005930", Side: SideBuy, OrderType: TypeMarket, Quantity: d("1"), Price: d("70000")},
	}
	for name, r := range cases {
		if _, err := c.Place(ctx, r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := c.PlaceAmount(ctx, AmountRequest{Symbol: "AAPL", Side: SideBuy}); err == nil {
		t.Error("zero orderAmount: want error")
	}
}

func TestModify(t *testing.T) {
	// 정정 응답의 orderId 는 새로 발급된 id 다(원주문 id 와 다르다)
	c, body, done := captureBody(t, "/api/v1/orders/o-1/modify", "3", []byte(`{"result":{"orderId":"o-1b"}}`))
	defer done()
	price, qty := d("71000"), d("5")
	res, err := c.Modify(context.Background(), "o-1", ModifyRequest{OrderType: TypeLimit, Price: &price, Quantity: &qty})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"orderType":"LIMIT","price":"71000","quantity":"5"}`)
	if res.OrderID != "o-1b" || res.OrderID == "o-1" {
		t.Errorf("정정 결과는 새 주문 id 여야 한다: %+v", res)
	}
}

func TestModify_Validation(t *testing.T) {
	c := New(nil, 3)
	ctx := context.Background()
	if _, err := c.Modify(ctx, "", ModifyRequest{OrderType: TypeLimit}); err == nil {
		t.Error("empty orderId: want error")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{}); err == nil {
		t.Error("empty orderType: want error")
	}

	price, qtyZero, qtyFrac, qtyNeg := d("71000"), d("0"), d("1.5"), d("-1")
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit}); err == nil {
		t.Error("LIMIT with nil price: want error")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeMarket, Price: &price}); err == nil {
		t.Error("MARKET with price set: want error")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Price: &price, Quantity: &qtyZero}); err == nil {
		t.Error("quantity 0: want error")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Price: &price, Quantity: &qtyFrac}); err == nil {
		t.Error("fractional quantity: want error")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Price: &price, Quantity: &qtyNeg}); err == nil {
		t.Error("negative quantity: want error")
	}
}

func TestCancel(t *testing.T) {
	// 취소 응답의 orderId 도 새로 발급된 id 다
	c, body, done := captureBody(t, "/api/v1/orders/o-1/cancel", "3", []byte(`{"result":{"orderId":"o-1c"}}`))
	defer done()
	res, err := c.Cancel(context.Background(), "o-1")
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{}`) // 취소는 빈 바디
	if res.OrderID != "o-1c" || res.OrderID == "o-1" {
		t.Errorf("취소 결과는 새 주문 id 여야 한다: %+v", res)
	}
}

func TestCancel_RequiresID(t *testing.T) {
	if _, err := New(nil, 3).Cancel(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

// --- 중복 주문 방지 불변식 ---

// countingUnauthorized 는 항상 401 invalid-token 을 돌려주며 요청 수를 센다.
func countingUnauthorized(t *testing.T) (*Client, *int32, func()) {
	t.Helper()
	var n int32
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"invalid-token","message":""}}`))
	})
	return New(hc, 3), &n, done
}

func TestWrites_WithoutKeyAreNeverRetried(t *testing.T) {
	// 멱등성 키 없는 쓰기는 401 이어도 절대 재전송되지 않는다 — 중복 주문 방지의 핵심 불변식
	ctx := context.Background()
	for name, call := range map[string]func(c *Client) error{
		"Place": func(c *Client) error {
			_, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Side: SideBuy, OrderType: TypeMarket, Quantity: d("1")})
			return err
		},
		"PlaceAmount": func(c *Client) error {
			_, err := c.PlaceAmount(ctx, AmountRequest{Symbol: "AAPL", Side: SideBuy, OrderAmount: d("10")})
			return err
		},
		"Modify": func(c *Client) error {
			q := d("1")
			p := d("100")
			_, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Quantity: &q, Price: &p})
			return err
		},
		"Cancel": func(c *Client) error { _, err := c.Cancel(ctx, "o-1"); return err },
	} {
		c, n, done := countingUnauthorized(t)
		if err := call(c); err == nil {
			t.Errorf("%s: want error", name)
		}
		if got := atomic.LoadInt32(n); got != 1 {
			t.Errorf("%s: %d requests, want exactly 1 (재시도 금지)", name, got)
		}
		done()
	}
}

func TestPlace_WithKeyRetriesAndResendsSameKey(t *testing.T) {
	// 키가 있으면 1회 재시도하되, 두 요청의 clientOrderId 가 같아야 서버가 중복을 제거할 수 있다
	var n int32
	var keys []string
	var mu sync.Mutex
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body struct {
			ClientOrderID string `json:"clientOrderId"`
		}
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		keys = append(keys, body.ClientOrderID)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"orderId":"o-1","clientOrderId":"k9"}}`))
	})
	defer done()
	if _, err := New(hc, 3).Place(context.Background(), PlaceRequest{Symbol: "005930", Side: SideBuy, OrderType: TypeMarket, Quantity: d("1"), ClientOrderID: "k9"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != "k9" || keys[1] != "k9" {
		t.Errorf("clientOrderId per attempt = %q, want [k9 k9]", keys)
	}
}
EOF
go test ./order/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: New` 등). `testutil.NewServerFunc` 도 아직 없다.

- [x] **Step 3: testutil 에 임의 핸들러 서버 추가**

`internal/testutil/server.go` 에 덧붙인다:
```go
// NewServerFunc 는 검증을 호출자가 직접 하는 스텁 서버다(POST 바디 캡처 등).
func NewServerFunc(t *testing.T, h http.HandlerFunc) (*httpclient.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	c := httpclient.New(httpclient.Config{BaseURL: srv.URL, HTTPClient: srv.Client(), Tokens: staticTokens{}})
	return c, srv.Close
}
```

- [x] **Step 4: 구현**

**계좌 검증**: `order` 의 모든 메서드는 다른 검증보다 먼저 `params.AccountSeq(c.accountSeq)` 를 확인한다
(asset 패키지 컨벤션과 동일 — accountSeq <= 0 이면 계좌 헤더 없이 요청이 나가 서버가
`account-header-required` 를 돌려주므로 요청 전에 막는다). 파라미터 구조체는 값으로 받는다(포인터 아님) —
단, `ModifyRequest.Quantity`/`Price` 는 예외로 포인터다(아래 참고).

**에러 코드 보정**: 계획 초안의 `대표 에러:` 주석 중 `docs/api/openapi.json` 에 실제로 존재하지 않는
문자열을 실제 코드로 교정했다 — `invalid-tick-size`/`amount-us-market-only` → `invalid-request`
(둘 다 스펙상 400 `invalid-request` 로 뭉뚱그려진다), `unsupported-currency` → `invalid-request`
(스펙 예시 키는 `unsupportedCurrency` 지만 `code` 필드는 `invalid-request`). `idempotency-key-conflict`
는 실재가 확인되어 `Place`/`PlaceAmount` 대표 에러 목록에 추가했다.

**코드 리뷰 반영(Critical/Important)**:
- `Place`: `MARKET` 주문에 `Price` 를 지정하면 조용히 버리지 않고 요청 전에 거부한다
  (openapi: `MARKET` 에 `price` 전달 시 서버가 `400 invalid-request`로 거부 — 가격을 버리고
  시장가로 체결시키는 쪽이 서버보다 위험하다).
- `ModifyRequest.Quantity`/`Price` 를 `*decimal.Decimal` 로 바꿨다 — 값 `0` 과 "미전송" 을 구분하기
  위해서다. `Quantity` 는 양의 정수(`IsInteger`)인지도 사전 검증하고, `OrderType` 에 따라 `LIMIT` 이면
  `Price` 필수·`MARKET` 이면 `Price` 는 반드시 `nil` 이어야 한다.
- `order.Request` → `order.PlaceRequest` 로 이름을 바꿨다(conditionalorder 가 두 번째 `Request` 를
  만들기 전에 지금이 가장 싸다).
- `order_test.go` 에 중복 주문 방지 불변식 테스트를 추가했다: 멱등성 키 없는 쓰기 4종(Place/
  PlaceAmount/Modify/Cancel)은 401 이어도 정확히 1회만 요청되고, 키 있는 `Place` 는 401 에 1회
  재시도하되 두 시도가 같은 `clientOrderId` 를 보낸다.
- `captureBody` 의 바디 읽기를 `r.Body.Read(make([]byte, r.ContentLength))` 에서 `io.ReadAll(r.Body)`
  로 바꿔 안정화했다(짧은 읽기로 바디가 잘릴 수 있는 문제).

```bash
mkdir -p order && cat > order/client.go << 'EOF'
// Package order 는 토스 Open API 주문 그룹 — 주문 생성·정정·취소, 주문 조회, 주문 정보.
// toss.Client.Account(seq).Order 로 접근하며, 모든 요청에 계좌 헤더가 실린다.
//
// 주문은 실제 체결로 이어진다. SDK 는 요청 조립 오류(필수 누락·형식)만 사전 검증하고,
// 호가단위·잔고·거래시간 같은 상태 의존 규칙은 서버가 판단한다 — 에러는 *toss.APIError 로 온다.
//
// 멱등성: PlaceRequest.ClientOrderID 를 채우면 (1) 같은 값으로 재요청 시 토스가 이전 주문 결과를
// 그대로 돌려주고(10분), (2) SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다.
// 같은 키로 내용이 다른 요청을 보내면 400 idempotency-key-conflict 를 받는다.
package order

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 주문 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
EOF
cat > order/types.go << 'EOF'
package order

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Side 는 주문 방향.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// Type 은 호가 유형.
type Type string

const (
	TypeLimit  Type = "LIMIT"  // 지정가
	TypeMarket Type = "MARKET" // 시장가
)

// TimeInForce 는 주문 유효 조건. OrderType 과 결합해 주문 방식이 정해진다(LIMIT+CLS = LOC).
type TimeInForce string

const (
	TimeInForceDay   TimeInForce = "DAY" // 당일 유효. 미지정 시 서버 기본값
	TimeInForceClose TimeInForce = "CLS" // 장 마감(At the Close). 미국 주식 + LIMIT 만 지원
	TimeInForceOpen  TimeInForce = "OPG" // 장 개시(국내 시가단일가). 국내 전용
)

// Status 는 개별 주문의 상태.
type Status string

const (
	StatusPending         Status = "PENDING"          // 접수, 미체결
	StatusPendingCancel   Status = "PENDING_CANCEL"   // 취소 처리 중
	StatusPendingReplace  Status = "PENDING_REPLACE"  // 정정 처리 중
	StatusPartialFilled   Status = "PARTIAL_FILLED"   // 부분 체결
	StatusFilled          Status = "FILLED"           // 전량 체결
	StatusCanceled        Status = "CANCELED"         // 취소됨
	StatusRejected        Status = "REJECTED"         // 거부됨
	StatusCancelRejected  Status = "CANCEL_REJECTED"  // 취소 거부
	StatusReplaceRejected Status = "REPLACE_REJECTED" // 정정 거부
	StatusReplaced        Status = "REPLACED"         // 정정되어 대체됨
)

// StatusFilter 는 주문 목록 조회의 라이프사이클 그룹 필터다. 개별 주문의 Status 와 값 체계가 다르다.
type StatusFilter string

const (
	// StatusFilterOpen 은 진행 중 그룹 — PENDING, PARTIAL_FILLED, PENDING_CANCEL, PENDING_REPLACE.
	StatusFilterOpen StatusFilter = "OPEN"
	// StatusFilterClosed 는 종료 그룹 — FILLED, CANCELED, REJECTED, REPLACED, CANCEL_REJECTED,
	// REPLACE_REJECTED, PARTIAL_FILLED.
	StatusFilterClosed StatusFilter = "CLOSED"
)

// Execution 은 체결 정보. 미체결 주문은 FilledQuantity 가 0 이고 나머지는 nil 이다.
type Execution struct {
	FilledQuantity     decimal.Decimal  `json:"filledQuantity"`
	AverageFilledPrice *decimal.Decimal `json:"averageFilledPrice"` // 미체결이면 nil
	FilledAmount       *decimal.Decimal `json:"filledAmount"`
	Commission         *decimal.Decimal `json:"commission"`
	Tax                *decimal.Decimal `json:"tax"`
	FilledAt           *time.Time       `json:"filledAt"`
	SettlementDate     *tosstypes.Date  `json:"settlementDate"` // 결제일. 체결 전이면 nil
}

// Order 는 주문 1건.
type Order struct {
	OrderID     string             `json:"orderId"`
	Symbol      string             `json:"symbol"`
	Side        Side               `json:"side"`
	OrderType   Type               `json:"orderType"`
	TimeInForce TimeInForce        `json:"timeInForce"`
	Status      Status             `json:"status"`
	Price       *decimal.Decimal   `json:"price"` // 시장가 주문이면 nil
	Quantity    decimal.Decimal    `json:"quantity"`
	OrderAmount *decimal.Decimal   `json:"orderAmount"` // 금액 주문이면 설정, 수량 주문이면 nil
	Currency    tosstypes.Currency `json:"currency"`
	OrderedAt   time.Time          `json:"orderedAt"`
	CanceledAt  *time.Time         `json:"canceledAt"` // 취소되지 않았으면 nil
	Execution   Execution          `json:"execution"`
}

// Page 는 주문 목록 한 페이지. NextCursor 를 다음 요청의 Cursor 로 넘기면 이어서 조회한다.
type Page struct {
	Orders     []Order `json:"orders"`
	NextCursor *string `json:"nextCursor"` // 더 없으면 nil
	HasNext    bool    `json:"hasNext"`
}

// PlaceResult 는 주문 생성 결과.
type PlaceResult struct {
	OrderID       string  `json:"orderId"`
	ClientOrderID *string `json:"clientOrderId"` // 요청에 넣었을 때만 설정
}

// OperationResult 는 정정·취소 결과.
//
// OrderID 는 **정정/취소로 새로 발급된 주문 식별자**이며 원주문의 OrderID 와 다르다.
// 이후 조회·정정·취소에는 반드시 이 값을 써야 한다 — 원주문 ID 로 다시 취소를 시도하면
// 이미 처리된 주문을 다시 건드리게 된다.
type OperationResult struct {
	OrderID string `json:"orderId"` // 새로 발급된 주문 id(원주문 id 와 다름)
}
EOF
cat > order/place.go << 'EOF'
package order

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// PlaceRequest 는 수량 기준 주문 생성 요청.
type PlaceRequest struct {
	Symbol           string          // 필수
	Side             Side            // 필수
	OrderType        Type            // 필수
	Quantity         decimal.Decimal // 필수. 소수점 수량은 미국 주식 시장가 매도 전용이며 정규장 종료 1시간 전까지만 접수된다
	Price            decimal.Decimal // LIMIT 이면 필수. MARKET 이면 반드시 zero(전달하면 400 invalid-request)
	TimeInForce      TimeInForce     // 비우면 서버 기본값 DAY
	ClientOrderID    string          // 멱등성 키(10분). 설정하면 401 토큰 오류에 1회 재시도한다
	ConfirmHighValue bool            // 1억원 이상 주문에 true 가 아니면 400 confirm-high-value-required
}

// AmountRequest 는 금액 기준 주문 생성 요청 — 미국 주식 시장가 전용.
// 체결 수량은 시장가에 따라 정해지며, 정규장 시작~정규장 종료 1시간 전에만 접수된다.
type AmountRequest struct {
	Symbol           string
	Side             Side
	OrderAmount      decimal.Decimal // 필수(USD)
	ClientOrderID    string
	ConfirmHighValue bool
}

type placeBody struct {
	Symbol                string `json:"symbol"`
	Side                  Side   `json:"side"`
	OrderType             Type   `json:"orderType"`
	Quantity              string `json:"quantity,omitempty"`
	OrderAmount           string `json:"orderAmount,omitempty"`
	Price                 string `json:"price,omitempty"`
	TimeInForce           string `json:"timeInForce,omitempty"`
	ClientOrderID         string `json:"clientOrderId,omitempty"`
	ConfirmHighValueOrder bool   `json:"confirmHighValueOrder,omitempty"`
}

// Place 는 수량 기준 주문을 생성한다(POST /api/v1/orders).
//
// 대표 에러: insufficient-buying-power, order-hours-closed, invalid-request, price-out-of-range,
// stock-restricted, confirm-high-value-required, request-in-progress, idempotency-key-conflict.
func (c *Client) Place(ctx context.Context, r PlaceRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if r.Side == "" {
		return nil, errors.New("toss: side is required")
	}
	if r.OrderType == "" {
		return nil, errors.New("toss: orderType is required")
	}
	if !r.Quantity.IsPositive() {
		return nil, fmt.Errorf("toss: quantity must be positive (got %s)", r.Quantity)
	}
	if r.OrderType == TypeLimit && !r.Price.IsPositive() {
		return nil, fmt.Errorf("toss: price is required for LIMIT orders (got %s)", r.Price)
	}
	if r.OrderType == TypeMarket && !r.Price.IsZero() {
		return nil, fmt.Errorf("toss: price must not be set for MARKET orders (got %s)", r.Price)
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Side: r.Side, OrderType: r.OrderType,
		Quantity: r.Quantity.String(), TimeInForce: string(r.TimeInForce),
		ClientOrderID: r.ClientOrderID, ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if !r.Price.IsZero() {
		body.Price = r.Price.String()
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/orders", body, c.accountSeq, r.ClientOrderID)
}

// PlaceAmount 는 금액 기준 주문을 생성한다(POST /api/v1/orders). 미국 주식 시장가 전용이며
// orderType 은 SDK 가 MARKET 으로 채운다.
//
// 대표 에러: invalid-request(미국 시장가 외 요청 등), amount-order-outside-regular-hours,
// insufficient-buying-power, max-order-amount-exceeded, confirm-high-value-required,
// idempotency-key-conflict.
func (c *Client) PlaceAmount(ctx context.Context, r AmountRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if r.Side == "" {
		return nil, errors.New("toss: side is required")
	}
	if !r.OrderAmount.IsPositive() {
		return nil, fmt.Errorf("toss: orderAmount must be positive (got %s)", r.OrderAmount)
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Side: r.Side, OrderType: TypeMarket,
		OrderAmount:           r.OrderAmount.String(),
		ClientOrderID:         r.ClientOrderID,
		ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/orders", body, c.accountSeq, r.ClientOrderID)
}
EOF
cat > order/modify.go << 'EOF'
package order

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// validateKey 는 internal/params.ClientOrderIDFormat 에 위임한다(빈 값만 여기서 "멱등성 미적용"으로 통과시킨다).
func validateKey(id string) error {
	if id == "" {
		return nil // 멱등성 미적용
	}
	return params.ClientOrderIDFormat(id)
}

// ModifyRequest 는 주문 정정 요청.
// Quantity/Price 는 nil 이면 전송하지 않는다 — 값 0 과 "미전송" 을 구분하기 위해 포인터다.
// 국내 주식은 Quantity 가 필수(양의 정수)이고, 미국 주식은 Quantity 를 보낼 수 없다
// (보내면 400 us-modify-quantity-not-supported) — 가격만 정정할 수 있다. SDK 는 시장을 알 수 없으므로
// 이 규칙은 서버가 판단한다.
type ModifyRequest struct {
	OrderType        Type             // 필수
	Quantity         *decimal.Decimal // 국내 필수(양의 정수). 미국은 nil
	Price            *decimal.Decimal // LIMIT 필수, MARKET 은 nil 이어야 한다
	ConfirmHighValue bool
}

type modifyBody struct {
	OrderType             Type   `json:"orderType"`
	Quantity              string `json:"quantity,omitempty"`
	Price                 string `json:"price,omitempty"`
	ConfirmHighValueOrder bool   `json:"confirmHighValueOrder,omitempty"`
}

// Modify 는 주문의 가격 또는 수량을 정정한다(POST /api/v1/orders/{orderId}/modify).
// 성공 시 **새 주문 id** 를 돌려준다(원주문 id 는 더 이상 쓰지 않는다).
// 정정은 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: already-filled, already-canceled, already-modified, already-processing,
// order-not-found, modify-restricted, order-hours-closed, us-modify-quantity-not-supported.
func (c *Client) Modify(ctx context.Context, orderID string, r ModifyRequest) (*OperationResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	if r.OrderType == "" {
		return nil, errors.New("toss: orderType is required")
	}
	if r.Quantity != nil {
		if !r.Quantity.IsPositive() {
			return nil, fmt.Errorf("toss: quantity must be positive (got %s)", r.Quantity)
		}
		if !r.Quantity.IsInteger() {
			return nil, fmt.Errorf("toss: quantity must be an integer (got %s)", r.Quantity)
		}
	}
	switch r.OrderType {
	case TypeLimit:
		if r.Price == nil || !r.Price.IsPositive() {
			return nil, errors.New("toss: price is required for LIMIT orders")
		}
	case TypeMarket:
		if r.Price != nil {
			return nil, fmt.Errorf("toss: price must not be set for MARKET orders (got %s)", r.Price)
		}
	}
	body := modifyBody{OrderType: r.OrderType, ConfirmHighValueOrder: r.ConfirmHighValue}
	if r.Quantity != nil {
		body.Quantity = r.Quantity.String()
	}
	if r.Price != nil {
		body.Price = r.Price.String()
	}
	return fetch.PostOne[OperationResult](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID)+"/modify", body, c.accountSeq, "")
}

// Cancel 은 주문을 취소한다(POST /api/v1/orders/{orderId}/cancel). 이미 체결된 주문은 취소할 수 없다.
// 성공 시 **새 주문 id** 를 돌려준다(원주문 id 와 다르다).
// 취소는 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: already-filled, already-canceled, already-processing, order-not-found,
// cancel-restricted, order-hours-closed.
func (c *Client) Cancel(ctx context.Context, orderID string) (*OperationResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	return fetch.PostOne[OperationResult](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID)+"/cancel", struct{}{}, c.accountSeq, "")
}
EOF
cat > order/history.go << 'EOF'
package order

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// ListParams 는 주문 목록 조회 인자. Status 는 필수다.
type ListParams struct {
	Status StatusFilter   // 필수. OPEN(진행 중) 또는 CLOSED(종료)
	Symbol string         // 특정 종목만. 비우면 전체
	From   tosstypes.Date // 주문 생성일(orderedAt, KST) 기준 시작일(inclusive)
	To     tosstypes.Date // 주문 생성일 기준 종료일(inclusive)
	Cursor string         // 이전 응답의 NextCursor
	Limit  int            // 최대 100, 0 이면 서버 기본값(20). Status 가 OPEN 이면 무시되고 전량 반환된다
}

// List 는 주문 목록을 조회한다(GET /api/v1/orders).
func (c *Client) List(ctx context.Context, p ListParams) (*Page, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("status", string(p.Status)); err != nil {
		return nil, err
	}
	q := url.Values{"status": {string(p.Status)}}
	if p.Symbol != "" {
		if err := params.Symbol(p.Symbol); err != nil {
			return nil, err
		}
		q.Set("symbol", p.Symbol)
	}
	params.Date(q, "from", p.From)
	params.Date(q, "to", p.To)
	params.Str(q, "cursor", p.Cursor)
	params.Int(q, "limit", p.Limit)
	return fetch.One[Page](ctx, c.http, "/api/v1/orders", q, c.accountSeq)
}

// Get 은 주문 상세를 조회한다(GET /api/v1/orders/{orderId}). 없으면 404 order-not-found.
func (c *Client) Get(ctx context.Context, orderID string) (*Order, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	return fetch.One[Order](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID), nil, c.accountSeq)
}
EOF
cat > order/info.go << 'EOF'
package order

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// BuyingPower 는 매수 가능 금액.
type BuyingPower struct {
	Currency        tosstypes.Currency `json:"currency"`
	CashBuyingPower decimal.Decimal    `json:"cashBuyingPower"` // 현금 매수 가능 금액
}

// SellableQuantity 는 판매 가능 수량.
type SellableQuantity struct {
	SellableQuantity decimal.Decimal `json:"sellableQuantity"`
}

// Commission 은 시장별 매매 수수료율. StartDate/EndDate 는 기간 한정 요율일 때만 설정된다.
type Commission struct {
	MarketCountry  tosstypes.MarketCountry `json:"marketCountry"`
	CommissionRate decimal.Decimal         `json:"commissionRate"` // 소수 비율(0.00015 = 0.015%)
	StartDate      *tosstypes.Date         `json:"startDate"`
	EndDate        *tosstypes.Date         `json:"endDate"`
}

// BuyingPower 는 매수 가능 금액을 조회한다(GET /api/v1/buying-power).
// 대표 에러: invalid-request(지원하지 않는 통화), account-not-found.
func (c *Client) BuyingPower(ctx context.Context, currency tosstypes.Currency) (*BuyingPower, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("currency", string(currency)); err != nil {
		return nil, err
	}
	return fetch.One[BuyingPower](ctx, c.http, "/api/v1/buying-power", url.Values{"currency": {string(currency)}}, c.accountSeq)
}

// SellableQuantity 는 판매 가능 수량을 조회한다(GET /api/v1/sellable-quantity).
func (c *Client) SellableQuantity(ctx context.Context, symbol string) (*SellableQuantity, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.One[SellableQuantity](ctx, c.http, "/api/v1/sellable-quantity", url.Values{"symbol": {symbol}}, c.accountSeq)
}

// Commissions 는 계좌의 매매 수수료율을 조회한다(GET /api/v1/commissions).
func (c *Client) Commissions(ctx context.Context) ([]Commission, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	return fetch.List[Commission](ctx, c.http, "/api/v1/commissions", nil, c.accountSeq)
}
EOF
```

`account.go` 의 `AccountScope` 에 `Order *order.Client // 주문: 생성·정정·취소·조회·주문 정보` 필드를 추가하고 `Account` 에서 `Order: order.New(c.http, accountSeq),` 로 연결한다. import 도 추가한다.

```bash
gofmt -w . && go vet ./... && go test ./order/ . -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: order 24 + 루트 13 = `37` (실행 결과로 확인하고 모두 PASS 인지만 본다).

- [x] **Step 5: 커밋**

```bash
git add order account.go internal/testutil && git commit -m "feat(order): 주문 생성·정정·취소·조회·주문정보 8 ops

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

(리뷰 반영 커밋은 별도로 `fix(order): ...` 로 이어졌다 — 위 Critical/Important 항목 참고.)

---

### Task 5: `conditionalorder` 패키지 (5 ops)

**Files:**
- Create: `conditionalorder/client.go`, `conditionalorder/types.go`, `conditionalorder/place.go`, `conditionalorder/history.go`, `conditionalorder/conditionalorder_test.go`
- Modify: `account.go` (`AccountScope.ConditionalOrder` 추가)

> 조건주문은 2xx 응답 예시가 없다. 조회 fixture 도 openapi 에 없으므로 **스키마 기준으로 손으로 만든
> fixture** 를 쓴다(테스트 파일 안에 인라인 JSON). 필드명·타입은 `ConditionalOrderDetailResponse`,
> `ConditionalOrderCondition`, `PaginatedConditionalOrderResponse` 스키마를 그대로 따른다.

- [ ] **Step 1: 실패 테스트 작성**

```bash
mkdir -p conditionalorder && cat > conditionalorder/conditionalorder_test.go << 'EOF'
package conditionalorder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/testutil"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// detailJSON 은 ConditionalOrderDetailResponse 스키마를 그대로 옮긴 응답 예시다(openapi 에 2xx 예시가 없다).
// 그룹(OCO/OTO)의 first/second 는 항상 같은 ConditionType 이므로 둘 다 STOP 이다.
// second.status=HOLDING 은 "선행 조건(OTO first) 체결 전 대기(leg 전용)" 이므로 OTO 여야 한다(OCO 에는
// 나올 수 없는 조합) — type 을 OTO 로, 아직 발동 전이므로 second.triggeredOrderId 는 null 이다.
const detailJSON = `{"result":{"conditionalOrderId":"c-1","type":"OTO","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"LIMIT","expireDate":"2026-12-31","first":{"type":"STOP","status":"WATCHING","triggerPrice":"65000","targetProfitRate":null,"orderPrice":"64900","triggeredOrderId":null},"second":{"type":"STOP","status":"HOLDING","triggerPrice":"63000","targetProfitRate":null,"orderPrice":"62900","triggeredOrderId":null},"createdAt":"2026-09-01T10:00:00+09:00"}}`

// detailProfitRateJSON 은 PROFIT_RATE 조건(퍼센트 단위 targetProfitRate, triggerPrice 는 null) 응답 예시다.
const detailProfitRateJSON = `{"result":{"conditionalOrderId":"c-2","type":"SINGLE","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"MARKET","expireDate":"2026-12-31","first":{"type":"PROFIT_RATE","status":"WATCHING","triggerPrice":null,"targetProfitRate":"10.5","orderPrice":null,"triggeredOrderId":null},"second":null,"createdAt":"2026-09-01T10:00:00+09:00"}}`

const listJSON = `{"result":{"conditionalOrders":[{"conditionalOrderId":"c-1","type":"SINGLE","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"MARKET","expireDate":"2026-12-31","first":{"type":"STOP","status":"WATCHING","triggerPrice":"65000","targetProfitRate":null,"orderPrice":null,"triggeredOrderId":null},"second":null,"createdAt":"2026-09-01T10:00:00+09:00"}],"nextCursor":"cur-2","hasNext":true}}`

func TestGet(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders/c-1"}, "4", 200, []byte(detailJSON))
	defer done()
	got, err := New(hc, 4).Get(context.Background(), "c-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConditionalOrderID != "c-1" || got.Type != TypeOTO || got.Status != StatusWatching || got.Market != "KR" {
		t.Errorf("detail = %+v", got)
	}
	if got.Quantity.String() != "10" || got.OrderType != OrderTypeLimit || got.ExpireDate == nil || *got.ExpireDate != "2026-12-31" || got.CreatedAt.IsZero() {
		t.Errorf("detail fields = %+v", got)
	}
	if got.First.Type != ConditionStop || got.First.Status != ConditionWatching || got.First.TriggerPrice == nil || got.First.TriggerPrice.String() != "65000" {
		t.Errorf("first = %+v", got.First)
	}
	if got.First.OrderPrice == nil || got.First.OrderPrice.String() != "64900" || got.First.TargetProfitRate != nil || got.First.TriggeredOrderID != nil {
		t.Errorf("first optional = %+v", got.First)
	}
	if got.Second == nil || got.Second.Type != ConditionStop || got.Second.Status != ConditionHolding || got.Second.TriggerPrice == nil || got.Second.TriggerPrice.String() != "63000" || got.Second.TargetProfitRate != nil {
		t.Errorf("second = %+v", got.Second)
	}
	if got.Second.TriggeredOrderID != nil {
		// HOLDING(OTO 의 second, first 체결 전 대기)은 아직 발동 전이라 triggeredOrderId 가 없다.
		t.Errorf("triggeredOrderId = %v, want nil (not yet triggered)", *got.Second.TriggeredOrderID)
	}
}

func TestGet_ProfitRateCondition(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders/c-2"}, "4", 200, []byte(detailProfitRateJSON))
	defer done()
	got, err := New(hc, 4).Get(context.Background(), "c-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.First.Type != ConditionProfitRate || got.First.TargetProfitRate == nil || got.First.TargetProfitRate.String() != "10.5" {
		t.Errorf("first = %+v", got.First)
	}
	if got.First.TriggerPrice != nil {
		t.Errorf("triggerPrice must be nil for PROFIT_RATE: %v", got.First.TriggerPrice)
	}
	if got.Second != nil {
		t.Errorf("second must be nil for SINGLE: %+v", got.Second)
	}
}

func TestList(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders", Query: url.Values{"status": {"OPEN"}, "symbol": {"005930"}, "cursor": {"c0"}, "limit": {"10"}}}, "4", 200, []byte(listJSON))
	defer done()
	page, err := New(hc, 4).List(context.Background(), ListParams{Status: StatusFilterOpen, Symbol: "005930", Cursor: "c0", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.ConditionalOrders) != 1 || !page.HasNext || page.NextCursor == nil || *page.NextCursor != "cur-2" {
		t.Fatalf("page = %+v", page)
	}
	if page.ConditionalOrders[0].Second != nil {
		t.Errorf("second must be nil for SINGLE: %+v", page.ConditionalOrders[0].Second)
	}
}

func TestList_RequiresStatus(t *testing.T) {
	if _, err := New(nil, 4).List(context.Background(), ListParams{}); err == nil {
		t.Error("want error")
	}
}

func TestGet_RequiresID(t *testing.T) {
	if _, err := New(nil, 4).Get(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestZeroAccountSeq(t *testing.T) {
	// http 는 nil — 검증을 통과해 실제로 요청을 보내려 하면 nil pointer dereference 로 즉시 드러난다.
	c := New(nil, 0)
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	if _, err := c.List(ctx, ListParams{Status: StatusFilterOpen}); err == nil {
		t.Error("List: want error for accountSeq=0")
	}
	if _, err := c.Get(ctx, "c-1"); err == nil {
		t.Error("Get: want error for accountSeq=0")
	}
	if _, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok}); err == nil {
		t.Error("Place: want error for accountSeq=0")
	}
	if _, err := c.Modify(ctx, "c-1", ModifyRequest{Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok}); err == nil {
		t.Error("Modify: want error for accountSeq=0")
	}
	if err := c.Cancel(ctx, "c-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=0")
	}

	cNeg := New(nil, -1)
	if err := cNeg.Cancel(ctx, "c-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=-1")
	}
}

// --- 쓰기(요청 조립만 검증) ---

func captureBody(t *testing.T, path, method, wantAccount string, status int, respond []byte) (*Client, *string, func()) {
	t.Helper()
	var got string
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || r.Method != method {
			t.Errorf("%s %s, want %s %s", r.Method, r.URL.Path, method, path)
		}
		if a := r.Header.Get("X-Tossinvest-Account"); a != wantAccount {
			t.Errorf("account = %q, want %q", a, wantAccount)
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respond != nil {
			_, _ = w.Write(respond)
		}
	})
	return New(hc, 4), &got, done
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got is not JSON: %s", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %s", want)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("body =\n  %s\nwant\n  %s", gb, wb)
	}
}

func TestPlace_Single(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/conditional-orders", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-9","clientOrderId":"k1"}}`))
	defer done()
	res, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeSingle, Quantity: d("10"), OrderType: OrderTypeLimit,
		ExpireDate: "2026-12-31", ClientOrderID: "k1",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("65000"), OrderPrice: d("64900")},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","type":"SINGLE","quantity":"10","orderType":"LIMIT","expireDate":"2026-12-31","clientOrderId":"k1","first":{"orderSide":"SELL","triggerPrice":"65000","orderPrice":"64900"}}`)
	if res.ConditionalOrderID != "c-9" {
		t.Errorf("res = %+v", res)
	}
}

func TestPlace_OCOWithSecond(t *testing.T) {
	// OCO/OTO 는 지정가(LIMIT)만 지원한다 — MARKET 조합은 서버가 거부하므로 fixture 로 쓰지 않는다.
	c, body, done := captureBody(t, "/api/v1/conditional-orders", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-10"}}`))
	defer done()
	second := Condition{OrderSide: SideSell, TriggerPrice: d("70000"), OrderPrice: d("70000")}
	if _, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeOCO, Quantity: d("5"), OrderType: OrderTypeLimit, ExpireDate: "2026-10-01",
		First:  Condition{OrderSide: SideSell, TriggerPrice: d("80000"), OrderPrice: d("80000")},
		Second: &second,
	}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","type":"OCO","quantity":"5","orderType":"LIMIT","expireDate":"2026-10-01","first":{"orderSide":"SELL","triggerPrice":"80000","orderPrice":"80000"},"second":{"orderSide":"SELL","triggerPrice":"70000","orderPrice":"70000"}}`)
}

func TestPlace_Validation(t *testing.T) {
	c := New(nil, 4) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}                          // MARKET 과 짝
	okLimit := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")} // LIMIT 과 짝
	second := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	sell1 := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}
	sell2 := Condition{OrderSide: SideSell, TriggerPrice: d("2"), OrderPrice: d("2")}
	buy2 := Condition{OrderSide: SideBuy, TriggerPrice: d("2"), OrderPrice: d("2")}
	cases := map[string]PlaceRequest{
		"empty symbol":                 {Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"empty type":                   {Symbol: "005930", Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"zero quantity":                {Symbol: "005930", Type: TypeSingle, OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"empty orderType":              {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), ExpireDate: "2026-12-31", First: ok},
		"empty expire":                 {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, First: ok},
		"no first side":                {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{TriggerPrice: d("1")}},
		"no trigger":                   {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{OrderSide: SideSell}},
		"market with orderPrice":       {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}},
		"limit without orderPrice":     {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: ok},
		"single with second":           {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok, Second: &second},
		"oco without second":           {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: okLimit},
		"oco market":                   {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok, Second: &second},
		"oco first buy (must be sell)": {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: buy2, Second: &sell1},
		"oco price order (first must exceed second)": {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2},
		"oto first sell (must be buy)":               {Symbol: "005930", Type: TypeOTO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2},
	}
	for name, r := range cases {
		if _, err := c.Place(ctx, r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestModify(t *testing.T) {
	// 응답의 conditionalOrderId 는 요청 id 와 다르다 — 수정은 취소 후 재생성이라 새 ID 가 발급되고
	// 기존 "c-1" 은 무효화된다(ModifyRequest 문서화한 불변식을 실행 가능하게 고정).
	c, body, done := captureBody(t, "/api/v1/conditional-orders/c-1/modify", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-1b"}}`))
	defer done()
	res, err := c.Modify(context.Background(), "c-1", ModifyRequest{
		Type: TypeSingle, Quantity: d("7"), OrderType: OrderTypeLimit, ExpireDate: "2026-11-30",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("66000"), OrderPrice: d("65900")},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"type":"SINGLE","quantity":"7","orderType":"LIMIT","expireDate":"2026-11-30","first":{"orderSide":"SELL","triggerPrice":"66000","orderPrice":"65900"}}`)
	if res.ConditionalOrderID != "c-1b" || res.ConditionalOrderID == "c-1" {
		t.Errorf("res.ConditionalOrderID = %q, want new id %q (not the modified-from id)", res.ConditionalOrderID, "c-1b")
	}
}

func TestPlace_WithKeyRetriesAndResendsSameKey(t *testing.T) {
	// 키가 있으면 1회 재시도하되, 두 요청의 clientOrderId 가 같아야 서버가 중복을 제거할 수 있다
	var n int32
	var keys []string
	var mu sync.Mutex
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body struct {
			ClientOrderID string `json:"clientOrderId"`
		}
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		keys = append(keys, body.ClientOrderID)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"conditionalOrderId":"c-9","clientOrderId":"k9"}}`))
	})
	defer done()
	if _, err := New(hc, 4).Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("1")}, ClientOrderID: "k9",
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != "k9" || keys[1] != "k9" {
		t.Errorf("clientOrderId per attempt = %q, want [k9 k9]", keys)
	}
}

func TestModify_Validation(t *testing.T) {
	// Modify 도 Place 와 같은 validateCommon 을 타는지 고정한다(id 자체는 유효해 통과시키고, body 규칙만 어긴다).
	c := New(nil, 4) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	sell1 := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}
	sell2 := Condition{OrderSide: SideSell, TriggerPrice: d("2"), OrderPrice: d("2")}
	buy2 := Condition{OrderSide: SideBuy, TriggerPrice: d("2"), OrderPrice: d("2")}
	cases := map[string]ModifyRequest{
		"single with second": {
			Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31",
			First: Condition{OrderSide: SideSell, TriggerPrice: d("1")}, Second: &sell1,
		},
		"oco without second": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell2,
		},
		"market with orderPrice": {
			Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: sell1,
		},
		"oco first buy": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: buy2, Second: &sell1,
		},
		"oco price order (first must exceed second)": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2,
		},
		"oto first sell (must be buy)": {
			Type: TypeOTO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2,
		},
	}
	for name, r := range cases {
		if _, err := c.Modify(ctx, "c-1", r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCancel_NoContent(t *testing.T) {
	c, _, done := captureBody(t, "/api/v1/conditional-orders/c-1", http.MethodDelete, "4", 204, nil)
	defer done()
	if err := c.Cancel(context.Background(), "c-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestCancel_RequiresID(t *testing.T) {
	if err := New(nil, 4).Cancel(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

// countingUnauthorized 는 항상 401 invalid-token 을 돌려주며 요청 수를 센다.
func countingUnauthorized(t *testing.T) (*Client, *int32, func()) {
	t.Helper()
	var n int32
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"invalid-token","message":""}}`))
	})
	return New(hc, 4), &n, done
}

func TestWrites_WithoutKeyAreNeverRetried(t *testing.T) {
	// 멱등성 키 없는 쓰기는 401 이어도 절대 재전송되지 않는다 — 중복 조건주문 방지의 핵심 불변식
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	for name, call := range map[string]func(c *Client) error{
		"Place": func(c *Client) error {
			_, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok})
			return err
		},
		"Modify": func(c *Client) error {
			_, err := c.Modify(ctx, "c-1", ModifyRequest{Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok})
			return err
		},
		"Cancel": func(c *Client) error { return c.Cancel(ctx, "c-1") },
	} {
		c, n, done := countingUnauthorized(t)
		if err := call(c); err == nil {
			t.Errorf("%s: want error", name)
		}
		if got := atomic.LoadInt32(n); got != 1 {
			t.Errorf("%s: %d requests, want exactly 1 (재시도 금지)", name, got)
		}
		done()
	}
}
EOF
go test ./conditionalorder/ 2>&1 | head -5
```
Expected: 컴파일 에러.

- [ ] **Step 2: 구현**

```bash
cat > conditionalorder/client.go << 'EOF'
// Package conditionalorder 는 토스 Open API 조건주문 그룹 — 생성·수정·취소·목록·상세.
// toss.Client.Account(seq).ConditionalOrder 로 접근하며, 모든 요청에 계좌 헤더가 실린다.
//
// 조건주문은 트리거 조건(가격 도달)이 충족되면 실제 주문을 낸다. 이 API 로 만들 수 있는 조건은
// 가격 도달(STOP)뿐이다. 목표 수익률(PROFIT_RATE) 조건은 토스 앱 등에서 만든 것을 조회만 할 수 있다.
// SDK 는 요청 조립·구조 오류를 사전 검증하고, 호가단위·잔고 등은 서버가 판단한다.
//
// 발동 세션: 국내는 KRX 정규장에서만, 해외는 거래 가능한 모든 시간대에 발동된다.
//
// 멱등성: PlaceRequest.ClientOrderID 를 채우면 같은 값으로 재요청할 때 서버가 중복 생성을 막고
// (10분 유효), SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다. 같은 키로 다른
// 내용을 보내면 400 idempotency-key-conflict.
package conditionalorder

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 조건주문 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
EOF
cat > conditionalorder/types.go << 'EOF'
package conditionalorder

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Type 은 조건주문 구성.
type Type string

const (
	TypeSingle Type = "SINGLE" // 조건 1개
	TypeOCO    Type = "OCO"    // 둘 중 하나가 충족되면 나머지 자동 취소. 양쪽 모두 매도이며 first 감시가 > 현재가 > second 감시가, 지정가 전용
	TypeOTO    Type = "OTO"    // first 가 체결되면 그때부터 second 감시 시작. first 는 매수·second 는 매도, 지정가 전용
)

// OrderType 은 조건 충족 시 낼 주문의 호가 유형.
type OrderType string

const (
	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"
)

// Side 는 조건 충족 시 낼 주문의 방향.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// Status 는 조건주문 전체 상태 — 살아있는 조건(leg)의 상태를 대표로 따른다.
// leg 전용 상태인 HOLDING/CANCELED 는 최상위 status 로는 내려오지 않는다.
type Status string

const (
	StatusWatching  Status = "WATCHING"  // 조건 감시 중
	StatusPaused    Status = "PAUSED"    // 일시 중지
	StatusOrdering  Status = "ORDERING"  // 조건 충족 — 주문 생성 진행 중
	StatusOrdered   Status = "ORDERED"   // 주문 생성됨
	StatusCompleted Status = "COMPLETED" // 완료
	StatusExpired   Status = "EXPIRED"   // 만료
)

// StatusFilter 는 목록 조회의 라이프사이클 그룹 필터. 개별 Status 와 값 체계가 다르다.
type StatusFilter string

const (
	// StatusFilterOpen 은 진행 중 — WATCHING, PAUSED, ORDERING, ORDERED.
	StatusFilterOpen StatusFilter = "OPEN"
	// StatusFilterClosed 는 종료 — COMPLETED, EXPIRED.
	StatusFilterClosed StatusFilter = "CLOSED"
)

// ConditionType 은 개별 조건(leg)의 종류. 그룹(OCO/OTO)의 first/second 는 항상 같은 타입이다.
type ConditionType string

const (
	ConditionStop       ConditionType = "STOP"        // 가격 트리거
	ConditionProfitRate ConditionType = "PROFIT_RATE" // 목표 수익률(%) 트리거. 조회 전용 — 이 API 로는 생성할 수 없다
)

// ConditionStatus 는 개별 조건(leg)의 상태.
type ConditionStatus string

const (
	ConditionWatching  ConditionStatus = "WATCHING"
	ConditionHolding   ConditionStatus = "HOLDING" // 선행 조건(OTO first) 체결 전 대기(leg 전용)
	ConditionPaused    ConditionStatus = "PAUSED"
	ConditionOrdering  ConditionStatus = "ORDERING"
	ConditionOrdered   ConditionStatus = "ORDERED"
	ConditionCompleted ConditionStatus = "COMPLETED"
	ConditionExpired   ConditionStatus = "EXPIRED"
	ConditionCanceled  ConditionStatus = "CANCELED" // 완료된 OCO 에서 자동취소된 반대편 조건(leg 전용)
)

// ConditionDetail 은 조회 응답의 개별 조건(leg).
type ConditionDetail struct {
	Type             ConditionType    `json:"type"`
	Status           ConditionStatus  `json:"status"`
	TriggerPrice     *decimal.Decimal `json:"triggerPrice"`     // STOP 조건에만
	TargetProfitRate *decimal.Decimal `json:"targetProfitRate"` // PROFIT_RATE 조건에만. 퍼센트 단위(10.5 = +10.5%)
	OrderPrice       *decimal.Decimal `json:"orderPrice"`       // 그룹 orderType 이 LIMIT 일 때 발동 주문 가격. MARKET 이면 nil
	TriggeredOrderID *string          `json:"triggeredOrderId"` // 발동해서 생성된 주문 id. 미발동이면 nil. Account(seq).Order.Get(ctx, *id) 로 조회한다
}

// Detail 은 조건주문 1건. 목록·상세가 같은 스키마를 쓴다.
type Detail struct {
	ConditionalOrderID string                  `json:"conditionalOrderId"`
	Type               Type                    `json:"type"`
	Status             Status                  `json:"status"`
	Symbol             string                  `json:"symbol"`
	Market             tosstypes.MarketCountry `json:"market"` // KR / US
	Quantity           decimal.Decimal         `json:"quantity"`
	OrderType          OrderType               `json:"orderType"`
	ExpireDate         *tosstypes.Date         `json:"expireDate"`
	First              ConditionDetail         `json:"first"`
	Second             *ConditionDetail        `json:"second"` // SINGLE 이면 nil
	CreatedAt          time.Time               `json:"createdAt"`
}

// Page 는 조건주문 목록 한 페이지.
type Page struct {
	ConditionalOrders []Detail `json:"conditionalOrders"`
	NextCursor        *string  `json:"nextCursor"` // 더 없으면 nil
	HasNext           bool     `json:"hasNext"`
}

// PlaceResult 는 조건주문 생성 결과.
type PlaceResult struct {
	ConditionalOrderID string  `json:"conditionalOrderId"`
	ClientOrderID      *string `json:"clientOrderId"` // 요청에 넣었을 때만 설정
}

// Result 는 조건주문 수정 결과.
type Result struct {
	ConditionalOrderID string `json:"conditionalOrderId"`
}
EOF
cat > conditionalorder/place.go << 'EOF'
package conditionalorder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// validateKey 는 internal/params.ClientOrderIDFormat 에 위임한다(빈 값만 여기서 "멱등성 미적용"으로 통과시킨다).
func validateKey(id string) error {
	if id == "" {
		return nil // 멱등성 미적용
	}
	return params.ClientOrderIDFormat(id)
}

// Condition 은 생성·수정 요청의 조건 1개.
type Condition struct {
	OrderSide    Side            // 필수
	TriggerPrice decimal.Decimal // 필수. 이 가격에 도달하면 발동
	OrderPrice   decimal.Decimal // 그룹 orderType 이 LIMIT 이면 필수, MARKET 이면 반드시 zero(설정하면 사전 검증에서 거부)
}

type conditionBody struct {
	OrderSide    Side   `json:"orderSide"`
	TriggerPrice string `json:"triggerPrice"`
	OrderPrice   string `json:"orderPrice,omitempty"`
}

// validate 는 조건 1개의 형식을 검증한다. ot 는 그룹 공통 호가유형(orderType) — LIMIT 이면 OrderPrice 가
// 필수이고, MARKET 이면 OrderPrice 를 보내면 안 된다(openapi ConditionRequest.orderPrice).
func (c Condition) validate(field string, ot OrderType) error {
	if c.OrderSide == "" {
		return fmt.Errorf("toss: %s.orderSide is required", field)
	}
	if !c.TriggerPrice.IsPositive() {
		return fmt.Errorf("toss: %s.triggerPrice must be positive (got %s)", field, c.TriggerPrice)
	}
	switch ot {
	case OrderTypeLimit:
		if !c.OrderPrice.IsPositive() {
			return fmt.Errorf("toss: %s.orderPrice is required for LIMIT conditional orders", field)
		}
	case OrderTypeMarket:
		if !c.OrderPrice.IsZero() {
			return fmt.Errorf("toss: %s.orderPrice must not be set for MARKET conditional orders (got %s)", field, c.OrderPrice)
		}
	}
	return nil
}

func (c Condition) body() conditionBody {
	b := conditionBody{OrderSide: c.OrderSide, TriggerPrice: c.TriggerPrice.String()}
	if c.OrderPrice.IsPositive() {
		b.OrderPrice = c.OrderPrice.String()
	}
	return b
}

// PlaceRequest 는 조건주문 생성 요청.
type PlaceRequest struct {
	Symbol     string          // 필수
	Type       Type            // 필수. SINGLE/OCO/OTO
	Quantity   decimal.Decimal // 필수. 그룹 공통값 — OCO/OTO 는 동일 포지션이라 두 조건이 같은 수량을 쓴다
	OrderType  OrderType       // 필수
	ExpireDate tosstypes.Date  // 필수. 이 날짜까지 감시
	First      Condition       // 필수
	Second     *Condition      // OCO/OTO 에서 사용
	// ClientOrderID 는 멱등성 키다. 같은 값으로 재요청하면 서버가 중복 생성을 막고(10분 유효), 설정하면
	// SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다. 같은 키로 내용이 다른 요청을
	// 보내면 400 idempotency-key-conflict.
	ClientOrderID string
	// ConfirmHighValue 는 1억원 이상 주문 동의 여부. true 가 아니면 400 confirm-high-value-required.
	ConfirmHighValue bool
}

type placeBody struct {
	Symbol                string         `json:"symbol"`
	Type                  Type           `json:"type"`
	Quantity              string         `json:"quantity"`
	OrderType             OrderType      `json:"orderType"`
	ExpireDate            string         `json:"expireDate"`
	First                 conditionBody  `json:"first"`
	Second                *conditionBody `json:"second,omitempty"`
	ClientOrderID         string         `json:"clientOrderId,omitempty"`
	ConfirmHighValueOrder bool           `json:"confirmHighValueOrder,omitempty"`
}

// ModifyRequest 는 조건주문 수정 요청.
//
// **수정은 부분 변경이 아니라 전체 재설정이다.** 서버는 기존 조건주문을 취소하고 새로 생성하므로,
// 유지하려는 조건은 모두 다시 보내야 한다. 예를 들어 OCO 의 만료일만 바꾸려고 Type: TypeSingle 과
// First 하나만 보내면 나머지 한쪽 조건(예: 손절 다리)이 사라진다.
//
// 수정 후에는 **새 conditionalOrderId 가 발급되고 기존 ID 는 무효화**된다.
// 이후 조회·수정·취소에는 반환된 ConditionalOrderID 를 써야 한다.
// 생성과 달리 ExpireDate 가 필수이며, 종목은 conditionalOrderId 로 식별되므로 Symbol 은 없다.
type ModifyRequest struct {
	Type       Type
	Quantity   decimal.Decimal // 필수. 그룹 공통값 — OCO/OTO 는 동일 포지션이라 두 조건이 같은 수량을 쓴다
	OrderType  OrderType
	ExpireDate tosstypes.Date
	First      Condition
	Second     *Condition
	// ConfirmHighValue 는 1억원 이상 주문 동의 여부. true 가 아니면 400 confirm-high-value-required.
	ConfirmHighValue bool
}

type modifyBody struct {
	Type                  Type           `json:"type"`
	Quantity              string         `json:"quantity"`
	OrderType             OrderType      `json:"orderType"`
	ExpireDate            string         `json:"expireDate"`
	First                 conditionBody  `json:"first"`
	Second                *conditionBody `json:"second,omitempty"`
	ConfirmHighValueOrder bool           `json:"confirmHighValueOrder,omitempty"`
}

// validateCommon 은 Place/Modify 가 공유하는 그룹 필드 검증이다. symbol 은 Place 에서만 있으므로
// 여기서 다루지 않는다(호출부가 별도로 params.Symbol 을 검증한다).
//
// type↔second 조합(openapi ConditionalOrderCreateRequest.second/orderType 설명): SINGLE 은 second 를
// 보내면 안 되고, OCO/OTO 는 second 가 필수이며 지정가(LIMIT)만 지원한다.
// OCO 는 양쪽 다 매도이며 first 감시가 > second 감시가 여야 하고, OTO 는 first 매수·second 매도다.
func validateCommon(typ Type, qty decimal.Decimal, ot OrderType, expire tosstypes.Date, first Condition, second *Condition) error {
	if typ == "" {
		return errors.New("toss: type is required")
	}
	if !qty.IsPositive() {
		return fmt.Errorf("toss: quantity must be positive (got %s)", qty)
	}
	if ot == "" {
		return errors.New("toss: orderType is required")
	}
	if expire.IsZero() {
		return errors.New("toss: expireDate is required")
	}
	switch typ {
	case TypeSingle:
		if second != nil {
			return errors.New("toss: second must not be set for SINGLE conditional orders")
		}
	case TypeOCO:
		if second == nil {
			return errors.New("toss: second is required for OCO conditional orders")
		}
		if ot != OrderTypeLimit {
			return fmt.Errorf("toss: OCO conditional orders support LIMIT only (got %s)", ot)
		}
		// openapi: "first/second 모두 매도(SELL)이며 first 감시가 > 현재가 > second 감시가 여야 합니다."
		// 현재가 비교는 상태 의존이라 서버 몫이고, 방향·순서만 사전에 막는다.
		if first.OrderSide != SideSell || second.OrderSide != SideSell {
			return fmt.Errorf("toss: OCO requires both conditions to be SELL (got first=%s second=%s)", first.OrderSide, second.OrderSide)
		}
		if !first.TriggerPrice.GreaterThan(second.TriggerPrice) {
			return fmt.Errorf("toss: OCO requires first.triggerPrice > second.triggerPrice (got %s, %s)", first.TriggerPrice, second.TriggerPrice)
		}
	case TypeOTO:
		if second == nil {
			return errors.New("toss: second is required for OTO conditional orders")
		}
		if ot != OrderTypeLimit {
			return fmt.Errorf("toss: OTO conditional orders support LIMIT only (got %s)", ot)
		}
		// openapi: "first 는 매수(BUY), second 는 매도(SELL)입니다."
		if first.OrderSide != SideBuy || second.OrderSide != SideSell {
			return fmt.Errorf("toss: OTO requires first=BUY and second=SELL (got first=%s second=%s)", first.OrderSide, second.OrderSide)
		}
	}
	if err := first.validate("first", ot); err != nil {
		return err
	}
	if second != nil {
		if err := second.validate("second", ot); err != nil {
			return err
		}
	}
	return nil
}

// Place 는 조건주문을 생성한다(POST /api/v1/conditional-orders).
//
// 대표 에러: invalid-request(orderSide·triggerPrice·orderPrice 형식 오류, 호가단위 불일치 등),
// stock-not-found.
func (c *Client) Place(ctx context.Context, r PlaceRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if err := validateCommon(r.Type, r.Quantity, r.OrderType, r.ExpireDate, r.First, r.Second); err != nil {
		return nil, err
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Type: r.Type, Quantity: r.Quantity.String(), OrderType: r.OrderType,
		ExpireDate: r.ExpireDate.String(), First: r.First.body(),
		ClientOrderID: r.ClientOrderID, ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if r.Second != nil {
		sb := r.Second.body()
		body.Second = &sb
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/conditional-orders", body, c.accountSeq, r.ClientOrderID)
}

// Modify 는 조건주문을 수정한다(POST /api/v1/conditional-orders/{id}/modify).
// 전체 재설정이며 성공 시 새 conditionalOrderId 를 돌려준다(기존 ID 무효).
// 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: invalid-request(호가단위 불일치 등), conditional-order-not-found.
func (c *Client) Modify(ctx context.Context, id string, r ModifyRequest) (*Result, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return nil, err
	}
	if err := validateCommon(r.Type, r.Quantity, r.OrderType, r.ExpireDate, r.First, r.Second); err != nil {
		return nil, err
	}
	body := modifyBody{
		Type: r.Type, Quantity: r.Quantity.String(), OrderType: r.OrderType,
		ExpireDate: r.ExpireDate.String(), First: r.First.body(), ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if r.Second != nil {
		sb := r.Second.body()
		body.Second = &sb
	}
	return fetch.PostOne[Result](ctx, c.http, "/api/v1/conditional-orders/"+url.PathEscape(id)+"/modify", body, c.accountSeq, "")
}

// Cancel 은 조건주문을 취소한다(DELETE /api/v1/conditional-orders/{id}). 성공 시 본문이 없다(204).
// 취소는 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: conditional-order-not-found.
func (c *Client) Cancel(ctx context.Context, id string) error {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return err
	}
	return fetch.Send(ctx, c.http, http.MethodDelete, "/api/v1/conditional-orders/"+url.PathEscape(id), nil, nil, c.accountSeq)
}
EOF
cat > conditionalorder/history.go << 'EOF'
package conditionalorder

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// ListParams 는 조건주문 목록 조회 인자. Status 는 필수다.
type ListParams struct {
	Status StatusFilter // 필수. OPEN / CLOSED
	Symbol string       // 특정 종목만. 비우면 전체
	Cursor string       // 이전 응답의 NextCursor
	Limit  int          // 최대 100, 0 이면 서버 기본값(20)
}

// List 는 조건주문 목록을 조회한다(GET /api/v1/conditional-orders).
// 이 API 로 등록한 조건주문뿐 아니라 다른 채널(토스증권 앱 등)에서 등록한 것도 함께 반환된다.
//
// 대표 에러: invalid-request(잘못된 status).
func (c *Client) List(ctx context.Context, p ListParams) (*Page, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("status", string(p.Status)); err != nil {
		return nil, err
	}
	q := url.Values{"status": {string(p.Status)}}
	if p.Symbol != "" {
		if err := params.Symbol(p.Symbol); err != nil {
			return nil, err
		}
		q.Set("symbol", p.Symbol)
	}
	params.Str(q, "cursor", p.Cursor)
	params.Int(q, "limit", p.Limit)
	return fetch.One[Page](ctx, c.http, "/api/v1/conditional-orders", q, c.accountSeq)
}

// Get 은 조건주문 상세를 조회한다(GET /api/v1/conditional-orders/{id}). 진행 중 + 종료된 조건주문을
// 모두 조회할 수 있다.
//
// 대표 에러: conditional-order-not-found.
func (c *Client) Get(ctx context.Context, id string) (*Detail, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return nil, err
	}
	return fetch.One[Detail](ctx, c.http, "/api/v1/conditional-orders/"+url.PathEscape(id), nil, c.accountSeq)
}
EOF
```

`account.go` 의 `AccountScope` 에 `ConditionalOrder *conditionalorder.Client // 조건주문` 필드와 `ConditionalOrder: conditionalorder.New(c.http, accountSeq),` 를 추가한다.

```bash
gofmt -w . && go vet ./... && go test ./conditionalorder/ . -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: conditionalorder 15(`TestZeroAccountSeq`·`TestWrites_WithoutKeyAreNeverRetried`·`TestGet_ProfitRateCondition`·`TestModify_Validation`·`TestPlace_WithKeyRetriesAndResendsSameKey` 포함) + 루트 13 = `28` (실행 결과로 확인, 모두 PASS 인지만 본다).

- [ ] **Step 3: 커밋**

```bash
git add conditionalorder account.go && git commit -m "feat(conditionalorder): 조건주문 생성·수정·취소·목록·상세 5 ops

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: 예시 · integration · README · 워크스페이스 CLAUDE.md

**Files:**
- Create: `examples/order/main.go`
- Modify: `integration_test.go`, `README.md`, `/Users/user/src/workspace_moneyflow/CLAUDE.md`

- [ ] **Step 1: 조회 전용 예시**

```bash
mkdir -p examples/order && cat > examples/order/main.go << 'EOF'
// 계좌·주문 조회 예시. 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go run ./examples/order
//
// 이 예시는 조회만 한다. 실제 주문을 내는 코드는 주석으로만 두었다 — 실행하면 실제 체결로 이어진다.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

func main() {
	c, err := toss.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accts, err := c.Accounts(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if len(accts) == 0 {
		log.Fatal("no accounts")
	}
	a := c.Account(accts[0].AccountSeq)
	fmt.Printf("account %s (%s)\n", accts[0].AccountNo, accts[0].AccountType)

	h, err := a.Asset.Holdings(ctx, asset.HoldingsParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("평가금액 %s KRW, 손익률 %s\n", h.MarketValue.Amount.KRW, h.ProfitLoss.Rate)
	for _, it := range h.Items {
		fmt.Printf("  %s %s: %s주 @ %s\n", it.Symbol, it.Name, it.Quantity, it.LastPrice)
	}

	bp, err := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("매수 가능 금액:", bp.CashBuyingPower)

	page, err := a.Order.List(ctx, order.ListParams{Status: order.StatusFilterOpen, Limit: 10})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("진행 중 주문 %d건\n", len(page.Orders))
	for _, o := range page.Orders {
		fmt.Printf("  %s %s %s %s주 (%s)\n", o.OrderID, o.Symbol, o.Side, o.Quantity, o.Status)
	}

	// 실제 주문 예시 — 실행하면 진짜 주문이 나간다. 필요할 때만 주석을 풀 것.
	//
	//	res, err := a.Order.Place(ctx, order.PlaceRequest{
	//	    Symbol: "005930", Side: order.SideBuy, OrderType: order.TypeLimit,
	//	    Quantity: decimal.NewFromInt(1), Price: decimal.NewFromInt(50000),
	//	    ClientOrderID: toss.NewClientOrderID(), // 멱등성 키 권장
	//	})
	//	if err != nil { log.Fatal(err) }
	//	fmt.Println("주문 접수:", res.OrderID)
	//	if _, err := a.Order.Cancel(ctx, res.OrderID); err != nil { log.Fatal(err) }
}
EOF
go vet ./examples/... && echo VET_OK
```
Expected: `VET_OK`.

- [ ] **Step 2: integration 테스트 — 조회 9개만**

`integration_test.go` 끝에 추가한다. **쓰기 메서드는 호출하지 않는다.**

```bash
cat >> integration_test.go << 'EOF'

// TestIntegration_AccountReadOnly 는 계좌·주문 조회만 호출한다.
// 주문 생성·정정·취소·조건주문 쓰기는 실제 체결로 이어지므로 integration 테스트에서 절대 호출하지 않는다.
func TestIntegration_AccountReadOnly(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	accts, err := c.Accounts(ctx)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accts) == 0 {
		t.Skip("no accounts on this credential")
	}
	if accts[0].AccountNo == "" || accts[0].AccountSeq == 0 {
		t.Errorf("account = %+v", accts[0])
	}
	a := c.Account(accts[0].AccountSeq)
	time.Sleep(1100 * time.Millisecond) // ACCOUNT 그룹 1/s

	if _, err := a.Asset.Holdings(ctx, asset.HoldingsParams{}); err != nil {
		t.Errorf("Holdings: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if _, err := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW); err != nil {
		t.Errorf("BuyingPower: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if _, err := a.Order.Commissions(ctx); err != nil {
		t.Errorf("Commissions: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	page, err := a.Order.List(ctx, order.ListParams{Status: order.StatusFilterClosed, Limit: 5})
	if err != nil {
		t.Fatalf("Order.List: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if len(page.Orders) > 0 {
		if _, err := a.Order.Get(ctx, page.Orders[0].OrderID); err != nil {
			t.Errorf("Order.Get: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	cpage, err := a.ConditionalOrder.List(ctx, conditionalorder.ListParams{Status: conditionalorder.StatusFilterOpen, Limit: 5})
	if err != nil {
		t.Fatalf("ConditionalOrder.List: %v", err)
	}
	if len(cpage.ConditionalOrders) > 0 {
		time.Sleep(300 * time.Millisecond)
		if _, err := a.ConditionalOrder.Get(ctx, cpage.ConditionalOrders[0].ConditionalOrderID); err != nil {
			t.Errorf("ConditionalOrder.Get: %v", err)
		}
	}
}
EOF
```
import 에 `"github.com/kenshin579/toss-go/asset"`, `"github.com/kenshin579/toss-go/order"`, `"github.com/kenshin579/toss-go/conditionalorder"` 를 추가한다.

```bash
gofmt -w . && go vet -tags integration ./... && echo VET_OK
```
Expected: `VET_OK`. 실행은 허용 IP 가 등록된 환경에서만 성공한다 — 현재 환경에서는 403 이며 그대로 보고한다.

- [ ] **Step 3: README 갱신**

`## 커버리지` 표 아래에 계좌 스코프 표를 추가한다:
```markdown
계좌가 필요한 API 는 `c.Account(accountSeq)` 스코프 아래에 있다(`X-Tossinvest-Account` 헤더 자동 주입).

| 그룹 | 필드 | 메서드 |
| --- | --- | --- |
| Account | (루트) | `Accounts` |
| Asset | `Asset` | `Holdings` |
| Order | `Order` | `Place` `PlaceAmount` `Modify` `Cancel` `List` `Get` `BuyingPower` `SellableQuantity` `Commissions` |
| Conditional Order | `ConditionalOrder` | `Place` `Modify` `Cancel` `List` `Get` |

조회 21 + 계좌·주문 15 = 36 ops (v0.2.0). 실시간 웹소켓은 후속 버전.
```

`## 사용` 섹션 끝에 계좌 예시와 주의사항을 추가한다:
````markdown
### 계좌·주문

```go
accts, _ := c.Accounts(ctx)          // 계좌 헤더가 필요 없는 유일한 계좌 API
a := c.Account(accts[0].AccountSeq)  // 이후 모든 호출에 계좌 헤더 자동 주입

h, _ := a.Asset.Holdings(ctx, asset.HoldingsParams{})
bp, _ := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW)

res, err := a.Order.Place(ctx, order.PlaceRequest{
    Symbol: "005930", Side: order.SideBuy, OrderType: order.TypeLimit,
    Quantity: decimal.NewFromInt(1), Price: decimal.NewFromInt(70000),
    ClientOrderID: toss.NewClientOrderID(), // 멱등성 키(권장)
})
```

**주문 시 주의**

- `ClientOrderID` 를 넣으면 10분간 멱등성이 적용되고(같은 값으로 재요청 시 이전 결과 반환),
  SDK 가 401 토큰 오류에 요청을 1회 재시도한다. **키가 없으면 쓰기 요청은 재시도하지 않는다** — 중복 주문을 만들지 않기 위해서다.
- 1억원 이상 주문은 `ConfirmHighValue: true` 가 없으면 `400 confirm-high-value-required`.
- 금액 주문(`PlaceAmount`)과 소수점 수량 주문은 **미국 주식 전용**이며 정규장 시작~종료 1시간 전에만 접수된다.
- SDK 는 요청 조립 오류(필수 누락·형식)만 검증한다. 호가단위·잔고·거래시간 같은 상태 의존 규칙은
  서버가 판단하며 `*toss.APIError` 로 돌아온다.
````

- [ ] **Step 4: 워크스페이스 CLAUDE.md** — `/Users/user/src/workspace_moneyflow/CLAUDE.md` 의 toss-go 항목 2곳에서 "조회 21 ops, 주문·WS 예정" → "조회 21 + 계좌·주문 15 = 36 ops, WS 예정", `**Module**` 문장의 그룹 목록에 `Account/Asset/Order/ConditionalOrder` 를 추가한다(파일만 수정, 커밋 없음 — git 저장소 아님).

- [ ] **Step 5: 전체 검증 + 커밋**

```bash
gofmt -l . ; go build ./... && go vet ./... && go vet -tags integration ./... && go test ./... -race -count=1 2>&1 | tail -16 && go mod tidy && git status --short
git add -A && git commit -m "docs: 계좌·주문 예시·integration(조회 전용)·README

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
Expected: 전 패키지 `ok`, `go mod tidy` 후 변경 없음.

---

### Task 7: PR 생성

- [ ] **Step 1: 푸시 + PR (gh + HEREDOC, 리뷰어 지정 금지)**

```bash
git push -u origin feature/account-order && gh pr create --title "feat: 계좌·주문 15 ops (v0.2.0)" --body "$(cat <<'EOF'
## Summary
- 계좌 스코프 클라이언트 `c.Account(accountSeq)` — `X-Tossinvest-Account` 헤더 자동 주입, 헤더가 필요 없는 `c.Accounts(ctx)` 는 루트
- `asset` 1 op(보유 주식), `order` 8 ops(생성·정정·취소·목록·상세·매수가능금액·판매가능수량·수수료), `conditionalorder` 5 ops
- `internal/httpclient` 에 `Request`/`Do` 추가 — POST/DELETE, 계좌 헤더, 204, **멱등성 키가 있을 때만 쓰기 재시도**(중복 주문 방지)
- 주문 생성은 openapi `oneOf` 를 `Place`(수량)/`PlaceAmount`(금액, US 시장가 전용) 두 메서드로 분리
- `toss.NewClientOrderID()` 멱등성 키 헬퍼, 자주 쓰는 에러 코드 상수
- 설계 `docs/superpowers/specs/2026-09-04-account-order-design.md`, 계획 `docs/superpowers/plans/2026-09-04-account-order.md`

## 안전
- **실주문을 내는 코드는 없다.** integration 테스트는 조회 9개만 호출하고, 예시의 주문 코드는 주석 처리했다.
- 쓰기 경로는 스텁 서버로 요청 바디·헤더·재시도 정책만 검증한다.

## Test plan
- [x] `go build ./... && go vet ./... && go vet -tags integration ./... && go test ./... -race` 통과
- [x] 멱등성 키 없는 쓰기는 401 에 재시도하지 않음(httpclient 테스트)
- [x] 204(조건주문 취소) 처리, 계좌 헤더 주입/미주입
- [ ] `go test -tags integration ./...` — 허용 IP 등록 환경에서 재실행 필요(현재 IP 는 403 access_denied)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)" && gh pr view --json number,title,url,baseRefName,headRefName,reviewRequests
```
Expected: PR URL, base `main`, head `feature/account-order`, reviewRequests 비어 있음.

---

## 머지 후 (사용자 머지 뒤 실행)

```bash
git checkout main && git pull origin main && ./scripts/release.sh v0.2.0
```
이후 메모리(`toss_go_library.md`)를 갱신하고, 다음 스펙(WebSocket)을 브레인스토밍한다.
