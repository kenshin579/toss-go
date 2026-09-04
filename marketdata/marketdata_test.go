package marketdata

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestPrices(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/prices", Query: url.Values{"symbols": {"005930,AAPL"}}}, 200, testutil.Fixture(t, "prices.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "005930", "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Symbol != "005930" || got[0].LastPrice.String() != "248000" || got[0].Currency != tosstypes.CurrencyKRW {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].Timestamp == nil || got[0].Timestamp.Hour() != 19 {
		t.Errorf("Timestamp = %v", got[0].Timestamp)
	}
	if got[1].LastPrice.String() != "330.02" || got[1].Currency != tosstypes.CurrencyUSD {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestPrices_EmptyResult(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/prices", Query: url.Values{"symbols": {"ZZZZZZ"}}}, 200, testutil.Fixture(t, "prices_empty.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "ZZZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

func TestPrices_NoSymbols(t *testing.T) {
	if _, err := New(nil).Prices(context.Background()); err == nil { // nil client: 검증이 요청 전에 실패해야 한다
		t.Error("want error")
	}
}

func TestOrderbook(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/orderbook", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "orderbook.json"))
	defer done()
	ob, err := New(hc).Orderbook(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if len(ob.Asks) != 10 || len(ob.Bids) != 10 {
		t.Errorf("asks=%d bids=%d", len(ob.Asks), len(ob.Bids))
	}
	if ob.Asks[0].Price.String() != "248000" || ob.Asks[0].Volume.String() != "33855" {
		t.Errorf("asks[0] = %+v", ob.Asks[0])
	}
	if ob.Timestamp == nil || ob.Currency != tosstypes.CurrencyKRW {
		t.Errorf("ts=%v cur=%s", ob.Timestamp, ob.Currency)
	}
}

func TestTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/trades", Query: url.Values{"symbol": {"005930"}, "count": {"2"}}}, 200, testutil.Fixture(t, "trades.json"))
	defer done()
	got, err := New(hc).Trades(context.Background(), "005930", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Price.String() != "247500" || got[1].Volume.String() != "200" || got[1].Timestamp.IsZero() {
		t.Errorf("got = %+v", got)
	}
}

func TestTrades_DefaultCountOmitted(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/trades", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "trades.json"))
	defer done()
	if _, err := New(hc).Trades(context.Background(), "005930", 0); err != nil {
		t.Fatal(err)
	}
}

func TestPriceLimits(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/price-limits", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "price_limits.json"))
	defer done()
	pl, err := New(hc).PriceLimits(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if pl.UpperLimitPrice == nil || pl.UpperLimitPrice.String() != "325000" || pl.LowerLimitPrice == nil || pl.LowerLimitPrice.String() != "175000" {
		t.Errorf("got %+v", pl)
	}
}

func TestCandles(t *testing.T) {
	before := time.Date(2026, 9, 1, 0, 0, 0, 0, tosstypes.KST)
	adj := false
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/candles", Query: url.Values{
		"symbol": {"005930"}, "interval": {"1d"}, "count": {"2"}, "before": {"2026-09-01T00:00:00+09:00"}, "adjusted": {"false"},
	}}, 200, testutil.Fixture(t, "candles.json"))
	defer done()
	page, err := New(hc).Candles(context.Background(), CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 2, Before: &before, Adjusted: &adj})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Candles) != 2 {
		t.Fatalf("candles = %d", len(page.Candles))
	}
	c0 := page.Candles[0]
	if c0.OpenPrice.String() != "252000" || c0.HighPrice.String() != "255500" || c0.LowPrice.String() != "243000" || c0.ClosePrice.String() != "248000" || c0.Volume.String() != "21475989" {
		t.Errorf("c0 = %+v", c0)
	}
	if c0.Timestamp.Year() != 2026 || c0.Timestamp.Day() != 3 {
		t.Errorf("Timestamp = %v", c0.Timestamp)
	}
	if page.NextBefore == nil || page.NextBefore.Day() != 1 {
		t.Errorf("NextBefore = %v", page.NextBefore)
	}
}

func TestCandles_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.Candles(context.Background(), CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.Candles(context.Background(), CandlesParams{Symbol: "005930"}); err == nil {
		t.Error("want error for empty interval")
	}
}

func TestEmptySymbolRejected(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	if _, err := c.Orderbook(ctx, ""); err == nil {
		t.Error("Orderbook")
	}
	if _, err := c.Trades(ctx, " 005930", 1); err == nil {
		t.Error("Trades with whitespace")
	}
	if _, err := c.PriceLimits(ctx, "삼성"); err == nil {
		t.Error("PriceLimits non-ascii")
	}
	if _, err := c.Prices(ctx, "005930", ""); err == nil {
		t.Error("Prices empty element")
	}
}
