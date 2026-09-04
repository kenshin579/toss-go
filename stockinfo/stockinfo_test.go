package stockinfo

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestStocks(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks", Query: url.Values{"symbols": {"005930,AAPL"}}}, 200, testutil.Fixture(t, "stocks.json"))
	defer done()
	got, err := New(hc).Stocks(context.Background(), "005930", "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	var samsung, apple *Stock
	for i := range got {
		switch got[i].Symbol {
		case "005930":
			samsung = &got[i]
		case "AAPL":
			apple = &got[i]
		}
	}
	if samsung == nil || apple == nil {
		t.Fatalf("symbols = %+v", got)
	}
	if samsung.Name != "삼성전자" || samsung.Market != tosstypes.MarketKOSPI || samsung.SecurityType != tosstypes.SecurityTypeStock || samsung.Status != tosstypes.StockStatusActive || !samsung.IsCommonShare {
		t.Errorf("samsung = %+v", samsung)
	}
	if samsung.ListDate == nil || *samsung.ListDate != "1975-06-11" || samsung.DelistDate != nil || samsung.LeverageFactor != nil {
		t.Errorf("samsung dates/leverage = %v %v %v", samsung.ListDate, samsung.DelistDate, samsung.LeverageFactor)
	}
	if samsung.SharesOutstanding.String() != "5846278608" {
		t.Errorf("SharesOutstanding = %s", samsung.SharesOutstanding)
	}
	if samsung.KoreanMarketDetail == nil || !samsung.KoreanMarketDetail.NXTSupported || samsung.KoreanMarketDetail.NXTTradingSuspended == nil || *samsung.KoreanMarketDetail.NXTTradingSuspended {
		t.Errorf("KoreanMarketDetail = %+v", samsung.KoreanMarketDetail)
	}
	if apple.KoreanMarketDetail != nil || apple.Market != tosstypes.MarketNASDAQ || apple.Currency != tosstypes.CurrencyUSD {
		t.Errorf("apple = %+v", apple)
	}
}

// fixture = openapi 예시(kospi): 요청 파라미터와 무관하게 STOCK+ETF 2건
func TestListStocks(t *testing.T) {
	cs := true
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/all", Query: url.Values{
		"market": {"KOSPI"}, "securityType": {"REIT"}, "status": {"ACTIVE"}, "commonShare": {"true"},
	}}, 200, testutil.Fixture(t, "stocks_all.json"))
	defer done()
	got, err := New(hc).ListStocks(context.Background(), ListStocksParams{Market: tosstypes.MarketKOSPI, SecurityType: tosstypes.SecurityTypeREIT, Status: tosstypes.StockStatusActive, CommonShare: &cs})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Symbol != "005930" || got[0].Name != "삼성전자" || got[0].SecurityType != tosstypes.SecurityTypeStock || !got[0].IsCommonShare || got[0].ISINCode != "KR7005930003" {
		t.Errorf("got[0] = %+v", got)
	}
	if got[1].Symbol != "069500" || got[1].SecurityType != tosstypes.SecurityTypeETF || got[1].ISINCode != "KR7069500007" {
		t.Errorf("got[1] = %+v", got)
	}
}

func TestListStocks_RequiresMarket(t *testing.T) {
	if _, err := New(nil).ListStocks(context.Background(), ListStocksParams{}); err == nil {
		t.Error("want error")
	}
}

func TestWarnings_Empty(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/warnings"}, 200, testutil.Fixture(t, "warnings_empty.json"))
	defer done()
	got, err := New(hc).Warnings(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v", got)
	}
}

func TestWarnings_Decode(t *testing.T) {
	body := []byte(`{"result":[{"warningType":"OVERHEATED","exchange":"KRX","startDate":"2026-09-01","endDate":null}]}`)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/000001/warnings"}, 200, body)
	defer done()
	got, err := New(hc).Warnings(context.Background(), "000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].WarningType != tosstypes.WarningTypeOverheated || got[0].Exchange == nil || *got[0].Exchange != "KRX" || got[0].StartDate == nil || got[0].EndDate != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestInvestorTrading(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/investor-trading", Query: url.Values{"count": {"1"}, "until": {"2026-09-03"}}}, 200, testutil.Fixture(t, "investor_trading.json"))
	defer done()
	page, err := New(hc).InvestorTrading(context.Background(), "005930", TrendParams{Count: 1, Until: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || *page.NextUntil != "2026-09-02" || len(page.Records) != 1 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-09-03" || r.UpdatedAt.IsZero() {
		t.Errorf("date/updatedAt = %s %v", r.Date, r.UpdatedAt)
	}
	if r.Individual == nil || r.Individual.NetBuyVolume.String() != "-1248264" {
		t.Errorf("Individual = %+v", r.Individual)
	}
	if r.Foreigner.BuyVolume.String() != "3898908" {
		t.Errorf("Foreigner = %+v", r.Foreigner)
	}
	if r.Institution.NetBuyVolume.String() != "-860563" || r.Institution.Breakdown == nil || r.Institution.Breakdown.FinancialInvestment.BuyVolume.String() != "1036782" {
		t.Errorf("Institution = %+v", r.Institution)
	}
	if r.OtherCorporation == nil || r.ForeignerHolding == nil || r.ForeignerHolding.HoldingRate.String() != "0.4671" || r.CFD != nil {
		t.Errorf("other=%v fh=%v cfd=%v", r.OtherCorporation, r.ForeignerHolding, r.CFD)
	}
}

func TestProgramTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/program-trades", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "program_trades.json"))
	defer done()
	page, err := New(hc).ProgramTrades(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].Arbitrage.NetBuyVolume.String() != "3406" || page.Records[0].NonArbitrage.SellVolume.String() != "3201913" {
		t.Errorf("page = %+v", page)
	}
}

func TestShortSelling(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/short-selling", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "short_selling.json"))
	defer done()
	page, err := New(hc).ShortSelling(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := page.Records[0]
	if r.ShortSellingVolume.String() != "1226909" || r.ShortSellingAmount.String() != "306620169500" || r.ShortSellingVolumeRate == nil || r.ShortSellingVolumeRate.String() != "0.08919" {
		t.Errorf("r = %+v", r)
	}
}

// fixture = openapi 예시(daily): 2건, marginLoan/stockLoan 모두 존재
func TestCreditTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/credit-trades", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "credit_trades.json"))
	defer done()
	page, err := New(hc).CreditTrades(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || *page.NextUntil != "2026-07-14" || len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-07-16" || r.UpdatedAt.IsZero() || r.MarginLoan == nil || r.StockLoan == nil {
		t.Fatalf("r = %+v", r)
	}
	if r.MarginLoan.NewQuantity.String() != "125300" || r.MarginLoan.BalanceQuantity.String() != "2513400" || r.MarginLoan.BalanceRate.String() != "0.0042" || r.MarginLoan.TradingRate.String() != "0.09" {
		t.Errorf("MarginLoan = %+v", r.MarginLoan)
	}
	if r.StockLoan.BalanceQuantity.String() != "45200" || r.StockLoan.TradingRate.String() != "0.0004" {
		t.Errorf("StockLoan = %+v", r.StockLoan)
	}
}

// fixture = openapi 예시(daily)
func TestSecuritiesLending(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/securities-lending", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "securities_lending.json"))
	defer done()
	page, err := New(hc).SecuritiesLending(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-07-17" || r.ExecutionQuantity.String() != "210500" || r.RepaymentQuantity.String() != "185300" || r.BalanceQuantity.String() != "15234000" || r.BalanceAmount.String() != "1218720000000" {
		t.Errorf("r = %+v", r)
	}
}

// params.Symbol 이 [A-Za-z0-9.-] 만 허용하므로 PathEscape 는 방어용 — '.' 이 경로에 그대로 남는지만 확인
func TestTrend_DottedSymbolPath(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/BRK.B/short-selling"}, 200, []byte(`{"result":{"nextUntil":null,"records":[]}}`))
	defer done()
	page, err := New(hc).ShortSelling(context.Background(), "BRK.B", TrendParams{})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil != nil || len(page.Records) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestTrend_RequiresSymbol(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.InvestorTrading(context.Background(), "", TrendParams{}); err == nil {
		t.Error("InvestorTrading empty symbol")
	}
	if _, err := c.InvestorTrading(context.Background(), " 005930", TrendParams{}); err == nil {
		t.Error("InvestorTrading symbol with whitespace")
	}
	if _, err := c.Warnings(context.Background(), ""); err == nil {
		t.Error("Warnings empty symbol")
	}
}
