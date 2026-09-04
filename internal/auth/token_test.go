package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// newServer 는 토큰 엔드포인트 스텁. issued 는 발급 요청 횟수.
func newServer(t *testing.T, status int, body string, issued *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "id" || r.Form.Get("client_secret") != "sec" {
			t.Errorf("form = %v", r.Form)
		}
		atomic.AddInt32(issued, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const okBody = `{"access_token":"tok-1","token_type":"Bearer","expires_in":86399}`

func TestToken_IssuesAndCaches(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())

	for i := 0; i < 3; i++ {
		tok, err := s.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if tok != "tok-1" {
			t.Errorf("tok = %q", tok)
		}
	}
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Errorf("issued %d times, want 1", got)
	}
}

func TestToken_RefreshesNearExpiry(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	now := time.Now()
	s.now = func() time.Time { return now }

	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 만료 61초 전: 아직 유효
	s.now = func() time.Time { return now.Add(86399*time.Second - 61*time.Second) }
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Fatalf("issued %d, want 1 (still valid)", got)
	}
	// 만료 59초 전: 여유(60s) 안 → 재발급
	s.now = func() time.Time { return now.Add(86399*time.Second - 59*time.Second) }
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2 (refreshed)", got)
	}
}

func TestToken_ConcurrentIssuesOnce(t *testing.T) {
	var issued int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&issued, 1)
		<-release // 모든 goroutine 이 Token() 에 진입할 때까지 첫 발급을 붙잡아 둔다
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())

	const n = 100
	var started, wg sync.WaitGroup
	started.Add(n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			started.Done()
			if _, err := s.Token(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	started.Wait()
	time.Sleep(20 * time.Millisecond) // goroutine 들이 mutex 대기열에 쌓이도록
	close(release)
	wg.Wait()
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Errorf("issued %d, want 1", got)
	}
}

func TestToken_InvalidateForcesReissue(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Invalidate("tok-1")
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2", got)
	}

	// 캐시된 토큰과 다른 stale 값은 무시해야 한다(다른 goroutine 이 이미 받아 둔 새 토큰을 지우지 않기 위함).
	s.Invalidate("stale-other")
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2 (mismatch must not clear)", got)
	}
}

func TestToken_OAuth2Error(t *testing.T) {
	var issued int32
	srv := newServer(t, 403, `{"error":"access_denied","error_description":"IP address not allowed"}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.StatusCode != 403 || ae.Code != "access_denied" || ae.Description != "IP address not allowed" {
		t.Errorf("got %+v", ae)
	}
	if got := ae.Error(); got != "toss: token request failed (status 403): access_denied: IP address not allowed" {
		t.Errorf("Error() = %q", got)
	}
}

func TestToken_NonJSONError(t *testing.T) {
	var issued int32
	srv := newServer(t, 502, `<html>bad gateway</html>`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %v", err)
	}
	if ae.StatusCode != 502 || ae.Code != "" || ae.Description != "<html>bad gateway</html>" {
		t.Errorf("got %+v", ae)
	}
}

func TestToken_EmptyAccessToken(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, `{"access_token":"","token_type":"Bearer","expires_in":10}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err == nil {
		t.Error("want error for empty access_token")
	}
}

func TestToken_InvalidExpiresIn(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, `{"access_token":"tok","token_type":"Bearer","expires_in":0}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "expires_in") {
		t.Errorf("want expires_in error, got %v", err)
	}
}

func TestToken_ErrorBodyTruncatedOnRuneBoundary(t *testing.T) {
	var issued int32
	body := strings.Repeat("가", 100) + "\n\n" + strings.Repeat("나", 100) // 3바이트 문자 200개 = 600바이트
	srv := newServer(t, 500, body, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %v", err)
	}
	if !utf8.ValidString(ae.Description) || len(ae.Description) > 200 || strings.Contains(ae.Description, "\n") {
		t.Errorf("Description = %q (len %d)", ae.Description, len(ae.Description))
	}
}

func TestNew_NilHTTPClientDefaults(t *testing.T) {
	s := New("id", "sec", "https://example.invalid", nil)
	if s.hc == nil {
		t.Error("nil hc must default to http.DefaultClient")
	}
}
