package toss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
	if _, err := scope.Asset.Holdings(ctx, nil); err != nil {
		t.Fatalf("Holdings: %v", err)
	}
}
