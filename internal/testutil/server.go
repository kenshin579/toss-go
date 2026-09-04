// Package testutil 은 그룹 패키지 단위 테스트용 스텁 서버 헬퍼다.
package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

type staticTokens struct{}

func (staticTokens) Token(context.Context) (string, error) { return "test-token", nil }
func (staticTokens) Invalidate(string)                     {}

// Expect 는 스텁 서버가 검증할 요청 조건.
type Expect struct {
	Path  string     // 정확히 일치해야 하는 경로
	Query url.Values // 나열된 키는 값이 정확히 일치해야 하고, 나열되지 않은 키는 있으면 안 된다
}

// NewServer 는 요청을 검증하고 status/body 를 돌려주는 서버와 그에 연결된 httpclient 를 만든다.
// Bearer 헤더가 "test-token" 인지, 그리고 계좌 헤더가 실리지 않았는지 검증한다(계좌가 필요 없는 API 용).
func NewServer(t *testing.T, want Expect, status int, body []byte) (*httpclient.Client, func()) {
	t.Helper()
	return NewServerWithHeader(t, want, "", status, body)
}

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

// Fixture 는 testdata/<name> 을 읽는다.
func Fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}
