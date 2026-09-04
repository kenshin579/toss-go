package marketinfo

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestExchangeRate(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/exchange-rate", Query: url.Values{"baseCurrency": {"USD"}, "quoteCurrency": {"KRW"}}}, 200, testutil.Fixture(t, "exchange_rate.json"))
	defer done()
	fx, err := New(hc).ExchangeRate(context.Background(), tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fx.BaseCurrency != tosstypes.CurrencyUSD || fx.QuoteCurrency != tosstypes.CurrencyKRW || fx.Rate.String() != "1359.63" || fx.MidRate.String() != "1359.13" || fx.BasisPoint.String() != "4" || fx.RateChangeType != tosstypes.RateChangeTypeDown {
		t.Errorf("fx = %+v", fx)
	}
	if fx.ValidFrom.IsZero() || !fx.ValidUntil.After(fx.ValidFrom) {
		t.Errorf("validity = %v ~ %v", fx.ValidFrom, fx.ValidUntil)
	}
}

func TestExchangeRate_At(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, tosstypes.KST)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/exchange-rate", Query: url.Values{"baseCurrency": {"USD"}, "quoteCurrency": {"KRW"}, "dateTime": {"2026-09-01T09:00:00+09:00"}}}, 200, testutil.Fixture(t, "exchange_rate.json"))
	defer done()
	if _, err := New(hc).ExchangeRate(context.Background(), tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, &at); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeRate_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.ExchangeRate(context.Background(), "", tosstypes.CurrencyKRW, nil); err == nil {
		t.Error("want error for empty base")
	}
	if _, err := c.ExchangeRate(context.Background(), tosstypes.CurrencyUSD, "", nil); err == nil {
		t.Error("want error for empty quote")
	}
}

func TestKRMarketCalendar(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR", Query: url.Values{"date": {"2026-09-04"}}}, 200, testutil.Fixture(t, "market_calendar_kr.json"))
	defer done()
	cal, err := New(hc).KRMarketCalendar(context.Background(), "2026-09-04")
	if err != nil {
		t.Fatal(err)
	}
	if cal.Today.Date != "2026-09-04" || cal.PreviousBusinessDay.Date == "" || cal.NextBusinessDay.Date == "" {
		t.Errorf("cal = %+v", cal)
	}
	ih := cal.Today.Integrated
	if ih == nil || ih.PreMarket == nil || ih.RegularMarket == nil || ih.AfterMarket == nil {
		t.Fatalf("Integrated = %+v", ih)
	}
	if ih.RegularMarket.StartTime.Hour() != 9 || ih.RegularMarket.EndTime.Hour() != 15 || ih.RegularMarket.SinglePriceAuctionStartTime == nil || ih.RegularMarket.SinglePriceAuctionStartTime.Minute() != 20 {
		t.Errorf("RegularMarket = %+v", ih.RegularMarket)
	}
	if ih.AfterMarket.SinglePriceAuctionEndTime == nil || ih.AfterMarket.SinglePriceAuctionEndTime.Minute() != 40 {
		t.Errorf("AfterMarket = %+v", ih.AfterMarket)
	}
}

func TestKRMarketCalendar_NoDate(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR"}, 200, testutil.Fixture(t, "market_calendar_kr.json"))
	defer done()
	if _, err := New(hc).KRMarketCalendar(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestKRMarketCalendar_Holiday(t *testing.T) {
	body := []byte(`{"result":{"today":{"date":"2026-10-03","integrated":null},"previousBusinessDay":{"date":"2026-10-02","integrated":{"preMarket":null,"regularMarket":{"startTime":"2026-10-02T09:00:00+09:00","singlePriceAuctionStartTime":null,"endTime":"2026-10-02T15:30:00+09:00"},"afterMarket":null}},"nextBusinessDay":{"date":"2026-10-05","integrated":null}}}`)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR"}, 200, body)
	defer done()
	cal, err := New(hc).KRMarketCalendar(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cal.Today.Integrated != nil || cal.PreviousBusinessDay.Integrated == nil || cal.PreviousBusinessDay.Integrated.PreMarket != nil || cal.PreviousBusinessDay.Integrated.RegularMarket.SinglePriceAuctionStartTime != nil {
		t.Errorf("cal = %+v", cal)
	}
}

func TestUSMarketCalendar(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/US"}, 200, testutil.Fixture(t, "market_calendar_us.json"))
	defer done()
	cal, err := New(hc).USMarketCalendar(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cal.Today.Date == "" || cal.PreviousBusinessDay.Date == "" || cal.NextBusinessDay.Date == "" {
		t.Errorf("cal = %+v", cal)
	}
	if s := cal.NextBusinessDay.RegularMarket; s != nil && !s.EndTime.After(s.StartTime) {
		t.Errorf("RegularMarket = %+v", s)
	}
}
