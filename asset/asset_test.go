package asset

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestHoldings(t *testing.T) {
	// fixture = openapi 예시(withHoldings)
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings"}, "5", 200, testutil.Fixture(t, "holdings.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if h.TotalPurchaseAmount.KRW.String() != "6500000" || h.TotalPurchaseAmount.USD == nil || h.TotalPurchaseAmount.USD.String() != "1553" {
		t.Errorf("TotalPurchaseAmount = %+v", h.TotalPurchaseAmount)
	}
	if h.MarketValue.Amount.KRW.String() != "7200000" || h.ProfitLoss.Rate.String() != "0.1179" || h.DailyProfitLoss.Rate.String() != "0.0141" {
		t.Errorf("overview = %+v", h)
	}
	if len(h.Items) != 2 {
		t.Fatalf("items = %d", len(h.Items))
	}
	it := h.Items[0]
	if it.Symbol != "005930" || it.Name != "삼성전자" || it.MarketCountry != tosstypes.MarketCountryKR || it.Currency != tosstypes.CurrencyKRW {
		t.Errorf("item = %+v", it)
	}
	if it.Quantity.String() != "100" || it.LastPrice.String() != "72000" || it.AveragePurchasePrice.String() != "65000" {
		t.Errorf("item numbers = %+v", it)
	}
	if it.MarketValue.PurchaseAmount.String() != "6500000" || it.ProfitLoss.RateAfterCost.String() != "0.0846" || it.Cost.Commission.String() != "14400" {
		t.Errorf("item nested = %+v", it)
	}
	if it.Cost.Tax == nil || it.Cost.Tax.String() != "135600" {
		t.Errorf("tax = %v", it.Cost.Tax)
	}
	us := h.Items[1]
	if us.Symbol != "AAPL" || us.MarketCountry != tosstypes.MarketCountryUS || us.Currency != tosstypes.CurrencyUSD {
		t.Errorf("us item = %+v", us)
	}
	if us.LastPrice.String() != "178.5" || us.MarketValue.AmountAfterCost.String() != "1771.43" {
		t.Errorf("us decimals = %+v", us)
	}
	if us.Cost.Tax == nil {
		t.Errorf("us tax must be present in this fixture: %+v", us.Cost)
	}
}

func TestHoldings_Empty(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings"}, "5", 200, testutil.Fixture(t, "holdings_empty.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Items) != 0 {
		t.Errorf("items = %+v", h.Items)
	}
}

func TestHoldings_SymbolFilter(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings", Query: url.Values{"symbol": {"005930"}}}, "5", 200, testutil.Fixture(t, "holdings.json"))
	defer done()
	if _, err := New(hc, 5).Holdings(context.Background(), &HoldingsParams{Symbol: "005930"}); err != nil {
		t.Fatal(err)
	}
}

func TestHoldings_InvalidSymbol(t *testing.T) {
	if _, err := New(nil, 5).Holdings(context.Background(), &HoldingsParams{Symbol: "삼성"}); err == nil {
		t.Error("want validation error")
	}
}

func TestHoldings_NullableTax(t *testing.T) {
	// fixture = openapi 예시(filteredByUsSymbol): 미국 종목은 매도 세금이 없어 cost.tax 가 null
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/holdings", Query: url.Values{"symbol": {"AAPL"}}}, "5", 200, testutil.Fixture(t, "holdings_us.json"))
	defer done()
	h, err := New(hc, 5).Holdings(context.Background(), &HoldingsParams{Symbol: "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Items) != 1 {
		t.Fatalf("items = %d", len(h.Items))
	}
	if h.Items[0].Cost.Tax != nil {
		t.Errorf("tax must be nil, got %v", h.Items[0].Cost.Tax)
	}
}
