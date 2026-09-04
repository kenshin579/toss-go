package indicators

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestPrices(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/prices", Query: url.Values{"symbols": {"KOSPI"}}}, 200, testutil.Fixture(t, "prices.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "KOSPI")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "KOSPI" || got[0].LastPrice.String() != "6579.48" || got[0].Timestamp != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestPrices_NoSymbols(t *testing.T) {
	if _, err := New(nil).Prices(context.Background()); err == nil { // nil client: 검증이 요청 전에 실패해야 한다
		t.Error("want error")
	}
}

func TestCandles(t *testing.T) {
	// fixture = openapi 예시(dailyCandles)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/KOSPI/candles", Query: url.Values{"interval": {"1d"}, "count": {"2"}}}, 200, testutil.Fixture(t, "candles.json"))
	defer done()
	page, err := New(hc).Candles(context.Background(), "KOSPI", CandlesParams{Interval: tosstypes.Interval1d, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Candles) != 2 {
		t.Fatalf("page = %+v", page)
	}
	c0 := page.Candles[0]
	if c0.Timestamp.Year() != 2026 || c0.Timestamp.Month() != 6 || c0.Timestamp.Day() != 11 {
		t.Errorf("Timestamp = %v", c0.Timestamp)
	}
	if c0.OpenPrice.String() != "2798.32" || c0.HighPrice.String() != "2820.15" || c0.LowPrice.String() != "2790.1" || c0.ClosePrice.String() != "2812.45" || c0.Volume.String() != "542000000" {
		t.Errorf("candle = %+v", c0)
	}
	if page.NextBefore == nil || page.NextBefore.Day() != 10 {
		t.Errorf("NextBefore = %v", page.NextBefore)
	}
}

func TestCandles_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.Candles(context.Background(), "", CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.Candles(context.Background(), "삼성", CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for non-ascii symbol")
	}
	if _, err := c.Candles(context.Background(), "KOSPI", CandlesParams{}); err == nil {
		t.Error("want error for empty interval")
	}
}

func TestInvestorTrading(t *testing.T) {
	// fixture = openapi 예시(daily)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/KOSPI/investor-trading", Query: url.Values{"interval": {"1d"}, "count": {"1"}, "until": {"2026-09-03"}}}, 200, testutil.Fixture(t, "investor_trading.json"))
	defer done()
	page, err := New(hc).InvestorTrading(context.Background(), "KOSPI", InvestorTradingParams{Interval: tosstypes.IndicatorInterval1d, Count: 1, Until: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-06-11" || r.Institution.Breakdown.PensionFund.BuyAmount.String() != "500000000000" || r.Institution.Breakdown.PensionFund.SellAmount.String() != "490000000000" {
		t.Errorf("r = %+v", r)
	}
}

func TestInvestorTrading_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.InvestorTrading(context.Background(), "", InvestorTradingParams{Interval: tosstypes.IndicatorInterval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.InvestorTrading(context.Background(), "KOSPI", InvestorTradingParams{}); err == nil {
		t.Error("want error for empty interval")
	}
}
