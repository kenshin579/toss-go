package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
			t.Fatal(err)
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
	if issued != 1 {
		t.Errorf("issued %d times, want 1", issued)
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
	if issued != 1 {
		t.Fatalf("issued %d, want 1 (still valid)", issued)
	}
	// 만료 59초 전: 여유(60s) 안 → 재발급
	s.now = func() time.Time { return now.Add(86399*time.Second - 59*time.Second) }
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if issued != 2 {
		t.Errorf("issued %d, want 2 (refreshed)", issued)
	}
}

func TestToken_ConcurrentIssuesOnce(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Token(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if issued != 1 {
		t.Errorf("issued %d, want 1", issued)
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
	s.Invalidate()
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if issued != 2 {
		t.Errorf("issued %d, want 2", issued)
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
