package toss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

func TestNewClient_RequiresCredentials(t *testing.T) {
	if _, err := NewClient("", "s"); err == nil {
		t.Error("want error for empty clientID")
	}
	if _, err := NewClient("i", ""); err == nil {
		t.Error("want error for empty clientSecret")
	}
}

func TestNewClient_WiresGroups(t *testing.T) {
	c, err := NewClient("i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if c.MarketData == nil || c.StockInfo == nil || c.MarketInfo == nil || c.Ranking == nil || c.MarketIndicators == nil {
		t.Errorf("group clients not wired: %+v", c)
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("TOSS_CLIENT_ID", "")
	t.Setenv("TOSS_CLIENT_SECRET", "")
	if _, err := NewClientFromEnv(); err == nil {
		t.Error("want error when env missing")
	}
	t.Setenv("TOSS_CLIENT_ID", "id")
	t.Setenv("TOSS_CLIENT_SECRET", "sec")
	if _, err := NewClientFromEnv(); err != nil {
		t.Errorf("NewClientFromEnv: %v", err)
	}
}

// 토큰 발급 + 첫 API 호출까지 루트에서 end-to-end 로 확인한다.
func TestClient_EndToEnd(t *testing.T) {
	var tokenCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			atomic.AddInt32(&tokenCalls, 1)
			_, _ = w.Write([]byte(`{"access_token":"T","token_type":"Bearer","expires_in":3600}`))
		case "/api/v1/prices":
			if r.Header.Get("Authorization") != "Bearer T" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"result":[{"symbol":"005930","timestamp":null,"lastPrice":"1","currency":"KRW"}]}`))
		case "/api/v1/stocks/X/warnings":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"stock-not-found","message":"no"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := NewClient("i", "s", WithBaseURL(srv.URL), WithTimeout(5*time.Second), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tok, err := c.AccessToken(ctx)
	if err != nil || tok != "T" {
		t.Fatalf("AccessToken = %q, %v", tok, err)
	}
	ps, err := c.MarketData.Prices(ctx, "005930")
	if err != nil || len(ps) != 1 {
		t.Fatalf("Prices = %+v, %v", ps, err)
	}
	if tokenCalls != 1 {
		t.Errorf("token issued %d times, want 1 (cached)", tokenCalls)
	}

	_, err = c.StockInfo.Warnings(ctx, "X")
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 404 {
		t.Fatalf("want *APIError 404, got %v", err)
	}
	if !IsCode(err, "stock-not-found") || IsCode(err, "other") || IsCode(nil, "stock-not-found") {
		t.Error("IsCode mismatch")
	}
}

func TestAPIErrorAlias(t *testing.T) {
	var e error = &httpclient.APIError{StatusCode: 429}
	var ae *APIError
	if !errors.As(e, &ae) {
		t.Error("APIError must alias httpclient.APIError")
	}
}
