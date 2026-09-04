package ranking

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestRankings(t *testing.T) {
	ex := true
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/rankings", Query: url.Values{
		"type": {"TOP_GAINERS"}, "marketCountry": {"KR"}, "duration": {"1d"}, "count": {"2"}, "excludeInvestmentCaution": {"true"},
	}}, 200, testutil.Fixture(t, "rankings.json"))
	defer done()
	r, err := New(hc).Rankings(context.Background(), RankingsParams{Type: tosstypes.RankingTypeTopGainers, MarketCountry: tosstypes.MarketCountryKR, Duration: tosstypes.RankingDuration1d, Count: 2, ExcludeInvestmentCaution: &ex})
	if err != nil {
		t.Fatal(err)
	}
	if r.RankedAt == nil || len(r.Rankings) != 2 {
		t.Fatalf("r = %+v", r)
	}
	it := r.Rankings[0]
	if it.Rank != 1 || it.Symbol != "459550" || it.Currency != tosstypes.CurrencyKRW || it.Price.LastPrice.String() != "2570" || it.Price.BasePrice.String() != "1979" || it.Price.ChangeRate == nil || it.Price.ChangeRate.String() != "0.2986" {
		t.Errorf("it = %+v", it)
	}
	if it.TradingVolume.String() != "13640212" || it.TradingAmount.String() != "31835155684" {
		t.Errorf("volume/amount = %s %s", it.TradingVolume, it.TradingAmount)
	}
}

func TestRankings_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	cases := []RankingsParams{
		{MarketCountry: tosstypes.MarketCountryKR, Duration: tosstypes.RankingDuration1d},
		{Type: tosstypes.RankingTypeTopGainers, Duration: tosstypes.RankingDuration1d},
		{Type: tosstypes.RankingTypeTopGainers, MarketCountry: tosstypes.MarketCountryKR},
	}
	for i, p := range cases {
		if _, err := c.Rankings(context.Background(), p); err == nil {
			t.Errorf("case %d: want error", i)
		}
	}
}
