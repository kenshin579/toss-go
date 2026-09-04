package order

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// --- 조회 ---

func TestList(t *testing.T) {
	// fixture = openapi 예시(pendingMixed): 2건, hasNext=false
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{"status": {"OPEN"}}}, "3", 200, testutil.Fixture(t, "orders.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{Status: StatusFilterOpen})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 2 || page.HasNext || page.NextCursor != nil {
		t.Fatalf("page = %+v", page)
	}
	o := page.Orders[0]
	if o.OrderID == "" || o.Symbol != "005930" || o.Side != SideBuy || o.OrderType != TypeLimit || o.TimeInForce != TimeInForceDay || o.Status != StatusPending {
		t.Errorf("order = %+v", o)
	}
	if o.Price == nil || o.Price.String() != "70000" || o.Quantity.String() != "10" || o.OrderAmount != nil || o.Currency != tosstypes.CurrencyKRW {
		t.Errorf("order numbers = %+v", o)
	}
	if o.OrderedAt.IsZero() || o.CanceledAt != nil {
		t.Errorf("times = %v %v", o.OrderedAt, o.CanceledAt)
	}
	if o.Execution.FilledQuantity.String() != "0" || o.Execution.AverageFilledPrice != nil || o.Execution.FilledAt != nil || o.Execution.SettlementDate != nil {
		t.Errorf("execution = %+v", o.Execution)
	}
}

func TestList_AllParams(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{
		"status": {"CLOSED"}, "symbol": {"005930"}, "from": {"2026-09-01"}, "to": {"2026-09-04"}, "cursor": {"c1"}, "limit": {"50"},
	}}, "3", 200, testutil.Fixture(t, "orders_empty.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{
		Status: StatusFilterClosed, Symbol: "005930", From: "2026-09-01", To: "2026-09-04", Cursor: "c1", Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 0 {
		t.Errorf("orders = %+v", page.Orders)
	}
}

func TestList_NextCursor(t *testing.T) {
	// fixture = openapi 예시(completedWithNextPage): hasNext=true, nextCursor 존재
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders", Query: url.Values{"status": {"CLOSED"}}}, "3", 200, testutil.Fixture(t, "orders_page.json"))
	defer done()
	page, err := New(hc, 3).List(context.Background(), ListParams{Status: StatusFilterClosed})
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
		t.Errorf("page = %+v", page)
	}
}

func TestList_RequiresStatus(t *testing.T) {
	if _, err := New(nil, 3).List(context.Background(), ListParams{}); err == nil {
		t.Error("want error for empty status")
	}
}

func TestGet(t *testing.T) {
	// fixture = openapi 예시(krLimitFilled)
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/orders/o-1"}, "3", 200, testutil.Fixture(t, "order_filled.json"))
	defer done()
	o, err := New(hc, 3).Get(context.Background(), "o-1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusFilled || o.Execution.FilledQuantity.String() != "10" {
		t.Errorf("order = %+v", o)
	}
	if o.Execution.AverageFilledPrice == nil || o.Execution.AverageFilledPrice.String() != "70000" {
		t.Errorf("avg = %v", o.Execution.AverageFilledPrice)
	}
	if o.Execution.Commission == nil || o.Execution.Commission.String() != "1400" || o.Execution.Tax == nil || o.Execution.Tax.String() != "0" {
		t.Errorf("cost = %+v", o.Execution)
	}
	if o.Execution.FilledAt == nil || o.Execution.SettlementDate == nil || *o.Execution.SettlementDate != "2026-03-30" {
		t.Errorf("settlement = %v %v", o.Execution.FilledAt, o.Execution.SettlementDate)
	}
}

func TestGet_RequiresID(t *testing.T) {
	if _, err := New(nil, 3).Get(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestBuyingPower(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/buying-power", Query: url.Values{"currency": {"KRW"}}}, "3", 200, testutil.Fixture(t, "buying_power.json"))
	defer done()
	bp, err := New(hc, 3).BuyingPower(context.Background(), tosstypes.CurrencyKRW)
	if err != nil {
		t.Fatal(err)
	}
	if bp.Currency != tosstypes.CurrencyKRW || bp.CashBuyingPower.String() != "5000000" {
		t.Errorf("bp = %+v", bp)
	}
}

func TestBuyingPower_RequiresCurrency(t *testing.T) {
	if _, err := New(nil, 3).BuyingPower(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestSellableQuantity(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/sellable-quantity", Query: url.Values{"symbol": {"005930"}}}, "3", 200, testutil.Fixture(t, "sellable_quantity.json"))
	defer done()
	s, err := New(hc, 3).SellableQuantity(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if s.SellableQuantity.String() != "100" {
		t.Errorf("sellable = %+v", s)
	}
}

func TestCommissions(t *testing.T) {
	// fixture = openapi 예시(standard): KR/US 2건
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/commissions"}, "3", 200, testutil.Fixture(t, "commissions.json"))
	defer done()
	cs, err := New(hc, 3).Commissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("commissions = %+v", cs)
	}
	if cs[0].MarketCountry != tosstypes.MarketCountryKR || cs[0].CommissionRate.String() != "0.00015" {
		t.Errorf("kr = %+v", cs[0])
	}
	if cs[0].StartDate == nil || *cs[0].StartDate != "2026-01-01" || cs[1].StartDate != nil {
		t.Errorf("dates = %v %v", cs[0].StartDate, cs[1].StartDate)
	}
}

// --- 계좌 검증 (accountSeq <= 0 은 요청 전에 실패해야 한다) ---

func TestZeroAccountSeq(t *testing.T) {
	// http 는 nil — 검증을 통과해 실제로 요청을 보내려 하면 nil pointer dereference 로 즉시 드러난다.
	c := New(nil, 0)
	ctx := context.Background()
	if _, err := c.List(ctx, ListParams{Status: StatusFilterOpen}); err == nil {
		t.Error("List: want error for accountSeq=0")
	}
	if _, err := c.Get(ctx, "o-1"); err == nil {
		t.Error("Get: want error for accountSeq=0")
	}
	if _, err := c.BuyingPower(ctx, tosstypes.CurrencyKRW); err == nil {
		t.Error("BuyingPower: want error for accountSeq=0")
	}
	if _, err := c.SellableQuantity(ctx, "005930"); err == nil {
		t.Error("SellableQuantity: want error for accountSeq=0")
	}
	if _, err := c.Commissions(ctx); err == nil {
		t.Error("Commissions: want error for accountSeq=0")
	}
	if _, err := c.Place(ctx, Request{Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("10"), Price: d("70000")}); err == nil {
		t.Error("Place: want error for accountSeq=0")
	}
	if _, err := c.PlaceAmount(ctx, AmountRequest{Symbol: "AAPL", Side: SideBuy, OrderAmount: d("100")}); err == nil {
		t.Error("PlaceAmount: want error for accountSeq=0")
	}
	if _, err := c.Modify(ctx, "o-1", ModifyRequest{OrderType: TypeLimit, Price: d("71000"), Quantity: d("5")}); err == nil {
		t.Error("Modify: want error for accountSeq=0")
	}
	if _, err := c.Cancel(ctx, "o-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=0")
	}

	cNeg := New(nil, -1)
	if _, err := cNeg.Cancel(ctx, "o-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=-1")
	}
}

// --- 쓰기(요청 조립만 검증. 실주문은 절대 내지 않는다) ---

// captureBody 는 요청 바디를 그대로 돌려주는 스텁이다.
func captureBody(t *testing.T, path, wantAccount string, respond []byte) (*Client, *string, func()) {
	t.Helper()
	var got string
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if a := r.Header.Get("X-Tossinvest-Account"); a != wantAccount {
			t.Errorf("account = %q, want %q", a, wantAccount)
		}
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		got = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(respond)
	})
	return New(hc, 3), &got, done
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got is not JSON: %s", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %s", want)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("body =\n  %s\nwant\n  %s", gb, wb)
	}
}

func TestPlace_LimitBuy(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-9","clientOrderId":"k1"}}`))
	defer done()
	res, err := c.Place(context.Background(), Request{
		Symbol: "005930", Side: SideBuy, OrderType: TypeLimit,
		Quantity: d("10"), Price: d("70000"), ClientOrderID: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","side":"BUY","orderType":"LIMIT","quantity":"10","price":"70000","clientOrderId":"k1"}`)
	if res.OrderID != "o-9" || res.ClientOrderID == nil || *res.ClientOrderID != "k1" {
		t.Errorf("res = %+v", res)
	}
}

func TestPlace_MarketSellOmitsPriceAndTIF(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-10"}}`))
	defer done()
	if _, err := c.Place(context.Background(), Request{Symbol: "AAPL", Side: SideSell, OrderType: TypeMarket, Quantity: d("1.5")}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"AAPL","side":"SELL","orderType":"MARKET","quantity":"1.5"}`)
}

func TestPlace_TimeInForceAndConfirm(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-11"}}`))
	defer done()
	if _, err := c.Place(context.Background(), Request{
		Symbol: "AAPL", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("200"),
		TimeInForce: TimeInForceClose, ConfirmHighValue: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":"1","price":"200","timeInForce":"CLS","confirmHighValueOrder":true}`)
}

func TestPlaceAmount(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders", "3", []byte(`{"result":{"orderId":"o-12"}}`))
	defer done()
	if _, err := c.PlaceAmount(context.Background(), AmountRequest{Symbol: "AAPL", Side: SideBuy, OrderAmount: d("100"), ClientOrderID: "k2"}); err != nil {
		t.Fatal(err)
	}
	// orderType 은 SDK 가 MARKET 으로 채운다(스키마상 유일값)
	assertJSON(t, *body, `{"symbol":"AAPL","side":"BUY","orderType":"MARKET","orderAmount":"100","clientOrderId":"k2"}`)
}

func TestPlace_Validation(t *testing.T) {
	c := New(nil, 3) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	cases := map[string]Request{
		"empty symbol":   {Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"bad symbol":     {Symbol: "삼성", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"empty side":     {Symbol: "005930", OrderType: TypeLimit, Quantity: d("1"), Price: d("1")},
		"empty type":     {Symbol: "005930", Side: SideBuy, Quantity: d("1"), Price: d("1")},
		"zero quantity":  {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Price: d("1")},
		"neg quantity":   {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("-1"), Price: d("1")},
		"limit no price": {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1")},
		"bad key":        {Symbol: "005930", Side: SideBuy, OrderType: TypeLimit, Quantity: d("1"), Price: d("1"), ClientOrderID: "has space"},
	}
	for name, r := range cases {
		if _, err := c.Place(ctx, r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := c.PlaceAmount(ctx, AmountRequest{Symbol: "AAPL", Side: SideBuy}); err == nil {
		t.Error("zero orderAmount: want error")
	}
}

func TestModify(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders/o-1/modify", "3", []byte(`{"result":{"orderId":"o-1"}}`))
	defer done()
	res, err := c.Modify(context.Background(), "o-1", ModifyRequest{OrderType: TypeLimit, Price: d("71000"), Quantity: d("5")})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"orderType":"LIMIT","price":"71000","quantity":"5"}`)
	if res.OrderID != "o-1" {
		t.Errorf("res = %+v", res)
	}
}

func TestModify_Validation(t *testing.T) {
	c := New(nil, 3)
	if _, err := c.Modify(context.Background(), "", ModifyRequest{OrderType: TypeLimit}); err == nil {
		t.Error("empty orderId: want error")
	}
	if _, err := c.Modify(context.Background(), "o-1", ModifyRequest{}); err == nil {
		t.Error("empty orderType: want error")
	}
}

func TestCancel(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/orders/o-1/cancel", "3", []byte(`{"result":{"orderId":"o-1"}}`))
	defer done()
	res, err := c.Cancel(context.Background(), "o-1")
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{}`) // 취소는 빈 바디
	if res.OrderID != "o-1" {
		t.Errorf("res = %+v", res)
	}
}

func TestCancel_RequiresID(t *testing.T) {
	if _, err := New(nil, 3).Cancel(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}
