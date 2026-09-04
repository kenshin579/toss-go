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
	if len(got) == 0 || got[0].Symbol == "" || got[0].Name == "" || got[0].ISINCode == "" || got[0].SecurityType == "" {
		t.Errorf("got = %+v", got)
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

func TestCreditTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/credit-trades", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "credit_trades.json"))
	defer done()
	page, err := New(hc).CreditTrades(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) == 0 || page.Records[0].Date == "" || page.Records[0].UpdatedAt.IsZero() {
		t.Fatalf("page = %+v", page)
	}
	if ml := page.Records[0].MarginLoan; ml != nil && ml.BalanceQuantity.IsZero() && ml.NewQuantity.IsZero() {
		t.Errorf("MarginLoan present but all zero: %+v", ml)
	}
}

func TestSecuritiesLending(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/securities-lending", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "securities_lending.json"))
	defer done()
	page, err := New(hc).SecuritiesLending(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) == 0 || page.Records[0].BalanceQuantity.IsZero() {
		t.Errorf("page = %+v", page)
	}
}

func TestTrend_SymbolEscaped(t *testing.T) {
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
