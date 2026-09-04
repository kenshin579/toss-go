//go:build integration

package toss_test

import (
	"context"
	"os"
	"testing"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/conditionalorder"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/stockinfo"
	"github.com/kenshin579/toss-go/tosstypes"
)

// 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go test -tags integration -run TestIntegration ./
// 허용 IP 가 등록된 머신에서만 성공한다. 그룹별 rate limit 을 넘지 않도록 호출 사이에 짧게 쉰다.
func newIntegrationClient(t *testing.T) *toss.Client {
	t.Helper()
	if os.Getenv("TOSS_CLIENT_ID") == "" || os.Getenv("TOSS_CLIENT_SECRET") == "" {
		t.Skip("TOSS_CLIENT_ID / TOSS_CLIENT_SECRET not set")
	}
	c, err := toss.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestIntegration_MarketData(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	ps, err := c.MarketData.Prices(ctx, "005930", "AAPL")
	if err != nil || len(ps) != 2 {
		t.Fatalf("Prices: %+v %v", ps, err)
	}
	for _, p := range ps {
		if p.LastPrice.IsZero() {
			t.Errorf("zero price: %+v", p)
		}
	}
	time.Sleep(200 * time.Millisecond)

	page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 3})
	if err != nil || len(page.Candles) != 3 {
		t.Fatalf("Candles: %+v %v", page, err)
	}
	if page.NextBefore == nil {
		t.Error("NextBefore nil")
	}
	time.Sleep(200 * time.Millisecond)

	if _, err := c.MarketData.Orderbook(ctx, "005930"); err != nil {
		t.Errorf("Orderbook: %v", err)
	}
}

func TestIntegration_StockInfo(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	ss, err := c.StockInfo.Stocks(ctx, "005930")
	if err != nil || len(ss) != 1 || ss[0].Name != "삼성전자" {
		t.Fatalf("Stocks: %+v %v", ss, err)
	}
	time.Sleep(300 * time.Millisecond)

	page, err := c.StockInfo.InvestorTrading(ctx, "005930", stockinfo.TrendParams{Count: 2})
	if err != nil || len(page.Records) == 0 {
		t.Fatalf("InvestorTrading: %+v %v", page, err)
	}
	time.Sleep(300 * time.Millisecond)

	// 없는 종목의 유의사항: 404 stock-not-found 또는 빈 결과 — 둘 다 허용하되 다른 에러는 실패
	if _, err := c.StockInfo.Warnings(ctx, "000000"); err != nil && !toss.IsCode(err, "stock-not-found") {
		t.Errorf("Warnings(000000): %v", err)
	}
}

func TestIntegration_MarketInfo(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	fx, err := c.MarketInfo.ExchangeRate(ctx, tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, nil)
	if err != nil || fx.Rate.IsZero() {
		t.Fatalf("ExchangeRate: %+v %v", fx, err)
	}
	time.Sleep(500 * time.Millisecond) // MARKET_INFO 3/s

	cal, err := c.MarketInfo.KRMarketCalendar(ctx, "")
	if err != nil || cal.Today.Date == "" {
		t.Fatalf("KRMarketCalendar: %+v %v", cal, err)
	}
}

func TestIntegration_AccessToken(t *testing.T) {
	c := newIntegrationClient(t)
	tok, err := c.AccessToken(context.Background())
	if err != nil || len(tok) < 100 {
		t.Fatalf("AccessToken: len=%d %v", len(tok), err)
	}
}

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
