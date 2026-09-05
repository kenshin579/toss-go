package conditionalorder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/testutil"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// detailJSON 은 ConditionalOrderDetailResponse 스키마를 그대로 옮긴 응답 예시다(openapi 에 2xx 예시가 없다).
// 그룹(OCO/OTO)의 first/second 는 항상 같은 ConditionType 이므로 둘 다 STOP 이다.
// second.status=HOLDING 은 "선행 조건(OTO first) 체결 전 대기(leg 전용)" 이므로 OTO 여야 한다(OCO 에는
// 나올 수 없는 조합) — type 을 OTO 로, 아직 발동 전이므로 second.triggeredOrderId 는 null 이다.
const detailJSON = `{"result":{"conditionalOrderId":"c-1","type":"OTO","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"LIMIT","expireDate":"2026-12-31","first":{"type":"STOP","status":"WATCHING","triggerPrice":"65000","targetProfitRate":null,"orderPrice":"64900","triggeredOrderId":null},"second":{"type":"STOP","status":"HOLDING","triggerPrice":"63000","targetProfitRate":null,"orderPrice":"62900","triggeredOrderId":null},"createdAt":"2026-09-01T10:00:00+09:00"}}`

// detailProfitRateJSON 은 PROFIT_RATE 조건(퍼센트 단위 targetProfitRate, triggerPrice 는 null) 응답 예시다.
const detailProfitRateJSON = `{"result":{"conditionalOrderId":"c-2","type":"SINGLE","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"MARKET","expireDate":"2026-12-31","first":{"type":"PROFIT_RATE","status":"WATCHING","triggerPrice":null,"targetProfitRate":"10.5","orderPrice":null,"triggeredOrderId":null},"second":null,"createdAt":"2026-09-01T10:00:00+09:00"}}`

const listJSON = `{"result":{"conditionalOrders":[{"conditionalOrderId":"c-1","type":"SINGLE","status":"WATCHING","symbol":"005930","market":"KR","quantity":"10","orderType":"MARKET","expireDate":"2026-12-31","first":{"type":"STOP","status":"WATCHING","triggerPrice":"65000","targetProfitRate":null,"orderPrice":null,"triggeredOrderId":null},"second":null,"createdAt":"2026-09-01T10:00:00+09:00"}],"nextCursor":"cur-2","hasNext":true}}`

func TestGet(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders/c-1"}, "4", 200, []byte(detailJSON))
	defer done()
	got, err := New(hc, 4).Get(context.Background(), "c-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConditionalOrderID != "c-1" || got.Type != TypeOTO || got.Status != StatusWatching || got.Market != "KR" {
		t.Errorf("detail = %+v", got)
	}
	if got.Quantity.String() != "10" || got.OrderType != OrderTypeLimit || got.ExpireDate == nil || *got.ExpireDate != "2026-12-31" || got.CreatedAt.IsZero() {
		t.Errorf("detail fields = %+v", got)
	}
	if got.First.Type != ConditionStop || got.First.Status != ConditionWatching || got.First.TriggerPrice == nil || got.First.TriggerPrice.String() != "65000" {
		t.Errorf("first = %+v", got.First)
	}
	if got.First.OrderPrice == nil || got.First.OrderPrice.String() != "64900" || got.First.TargetProfitRate != nil || got.First.TriggeredOrderID != nil {
		t.Errorf("first optional = %+v", got.First)
	}
	if got.Second == nil || got.Second.Type != ConditionStop || got.Second.Status != ConditionHolding || got.Second.TriggerPrice == nil || got.Second.TriggerPrice.String() != "63000" || got.Second.TargetProfitRate != nil {
		t.Errorf("second = %+v", got.Second)
	}
	if got.Second.TriggeredOrderID != nil {
		// HOLDING(OTO 의 second, first 체결 전 대기)은 아직 발동 전이라 triggeredOrderId 가 없다.
		t.Errorf("triggeredOrderId = %v, want nil (not yet triggered)", *got.Second.TriggeredOrderID)
	}
}

func TestGet_ProfitRateCondition(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders/c-2"}, "4", 200, []byte(detailProfitRateJSON))
	defer done()
	got, err := New(hc, 4).Get(context.Background(), "c-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.First.Type != ConditionProfitRate || got.First.TargetProfitRate == nil || got.First.TargetProfitRate.String() != "10.5" {
		t.Errorf("first = %+v", got.First)
	}
	if got.First.TriggerPrice != nil {
		t.Errorf("triggerPrice must be nil for PROFIT_RATE: %v", got.First.TriggerPrice)
	}
	if got.Second != nil {
		t.Errorf("second must be nil for SINGLE: %+v", got.Second)
	}
}

func TestList(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/api/v1/conditional-orders", Query: url.Values{"status": {"OPEN"}, "symbol": {"005930"}, "cursor": {"c0"}, "limit": {"10"}}}, "4", 200, []byte(listJSON))
	defer done()
	page, err := New(hc, 4).List(context.Background(), ListParams{Status: StatusFilterOpen, Symbol: "005930", Cursor: "c0", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.ConditionalOrders) != 1 || !page.HasNext || page.NextCursor == nil || *page.NextCursor != "cur-2" {
		t.Fatalf("page = %+v", page)
	}
	if page.ConditionalOrders[0].Second != nil {
		t.Errorf("second must be nil for SINGLE: %+v", page.ConditionalOrders[0].Second)
	}
}

func TestList_RequiresStatus(t *testing.T) {
	if _, err := New(nil, 4).List(context.Background(), ListParams{}); err == nil {
		t.Error("want error")
	}
}

func TestGet_RequiresID(t *testing.T) {
	if _, err := New(nil, 4).Get(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

func TestZeroAccountSeq(t *testing.T) {
	// http 는 nil — 검증을 통과해 실제로 요청을 보내려 하면 nil pointer dereference 로 즉시 드러난다.
	c := New(nil, 0)
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	if _, err := c.List(ctx, ListParams{Status: StatusFilterOpen}); err == nil {
		t.Error("List: want error for accountSeq=0")
	}
	if _, err := c.Get(ctx, "c-1"); err == nil {
		t.Error("Get: want error for accountSeq=0")
	}
	if _, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok}); err == nil {
		t.Error("Place: want error for accountSeq=0")
	}
	if _, err := c.Modify(ctx, "c-1", ModifyRequest{Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok}); err == nil {
		t.Error("Modify: want error for accountSeq=0")
	}
	if err := c.Cancel(ctx, "c-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=0")
	}

	cNeg := New(nil, -1)
	if err := cNeg.Cancel(ctx, "c-1"); err == nil {
		t.Error("Cancel: want error for accountSeq=-1")
	}
}

// --- 쓰기(요청 조립만 검증) ---

func captureBody(t *testing.T, path, method, wantAccount string, status int, respond []byte) (*Client, *string, func()) {
	t.Helper()
	var got string
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || r.Method != method {
			t.Errorf("%s %s, want %s %s", r.Method, r.URL.Path, method, path)
		}
		if a := r.Header.Get("X-Tossinvest-Account"); a != wantAccount {
			t.Errorf("account = %q, want %q", a, wantAccount)
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respond != nil {
			_, _ = w.Write(respond)
		}
	})
	return New(hc, 4), &got, done
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

func TestPlace_Single(t *testing.T) {
	c, body, done := captureBody(t, "/api/v1/conditional-orders", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-9","clientOrderId":"k1"}}`))
	defer done()
	res, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeSingle, Quantity: d("10"), OrderType: OrderTypeLimit,
		ExpireDate: "2026-12-31", ClientOrderID: "k1",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("65000"), OrderPrice: d("64900")},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","type":"SINGLE","quantity":"10","orderType":"LIMIT","expireDate":"2026-12-31","clientOrderId":"k1","first":{"orderSide":"SELL","triggerPrice":"65000","orderPrice":"64900"}}`)
	if res.ConditionalOrderID != "c-9" {
		t.Errorf("res = %+v", res)
	}
}

func TestPlace_OCOWithSecond(t *testing.T) {
	// OCO/OTO 는 지정가(LIMIT)만 지원한다 — MARKET 조합은 서버가 거부하므로 fixture 로 쓰지 않는다.
	c, body, done := captureBody(t, "/api/v1/conditional-orders", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-10"}}`))
	defer done()
	second := Condition{OrderSide: SideSell, TriggerPrice: d("70000"), OrderPrice: d("70000")}
	if _, err := c.Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeOCO, Quantity: d("5"), OrderType: OrderTypeLimit, ExpireDate: "2026-10-01",
		First:  Condition{OrderSide: SideSell, TriggerPrice: d("80000"), OrderPrice: d("80000")},
		Second: &second,
	}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"symbol":"005930","type":"OCO","quantity":"5","orderType":"LIMIT","expireDate":"2026-10-01","first":{"orderSide":"SELL","triggerPrice":"80000","orderPrice":"80000"},"second":{"orderSide":"SELL","triggerPrice":"70000","orderPrice":"70000"}}`)
}

func TestPlace_Validation(t *testing.T) {
	c := New(nil, 4) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}                          // MARKET 과 짝
	okLimit := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")} // LIMIT 과 짝
	second := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	sell1 := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}
	sell2 := Condition{OrderSide: SideSell, TriggerPrice: d("2"), OrderPrice: d("2")}
	buy2 := Condition{OrderSide: SideBuy, TriggerPrice: d("2"), OrderPrice: d("2")}
	cases := map[string]PlaceRequest{
		"empty symbol":                 {Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"empty type":                   {Symbol: "005930", Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"zero quantity":                {Symbol: "005930", Type: TypeSingle, OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok},
		"empty orderType":              {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), ExpireDate: "2026-12-31", First: ok},
		"empty expire":                 {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, First: ok},
		"no first side":                {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{TriggerPrice: d("1")}},
		"no trigger":                   {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{OrderSide: SideSell}},
		"market with orderPrice":       {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}},
		"limit without orderPrice":     {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: ok},
		"single with second":           {Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok, Second: &second},
		"oco without second":           {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: okLimit},
		"oco market":                   {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok, Second: &second},
		"oco first buy (must be sell)": {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: buy2, Second: &sell1},
		"oco price order (first must exceed second)": {Symbol: "005930", Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2},
		"oto first sell (must be buy)":               {Symbol: "005930", Type: TypeOTO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2},
	}
	for name, r := range cases {
		if _, err := c.Place(ctx, r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestModify(t *testing.T) {
	// 응답의 conditionalOrderId 는 요청 id 와 다르다 — 수정은 취소 후 재생성이라 새 ID 가 발급되고
	// 기존 "c-1" 은 무효화된다(ModifyRequest 문서화한 불변식을 실행 가능하게 고정).
	c, body, done := captureBody(t, "/api/v1/conditional-orders/c-1/modify", http.MethodPost, "4", 200, []byte(`{"result":{"conditionalOrderId":"c-1b"}}`))
	defer done()
	res, err := c.Modify(context.Background(), "c-1", ModifyRequest{
		Type: TypeSingle, Quantity: d("7"), OrderType: OrderTypeLimit, ExpireDate: "2026-11-30",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("66000"), OrderPrice: d("65900")},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, *body, `{"type":"SINGLE","quantity":"7","orderType":"LIMIT","expireDate":"2026-11-30","first":{"orderSide":"SELL","triggerPrice":"66000","orderPrice":"65900"}}`)
	if res.ConditionalOrderID != "c-1b" || res.ConditionalOrderID == "c-1" {
		t.Errorf("res.ConditionalOrderID = %q, want new id %q (not the modified-from id)", res.ConditionalOrderID, "c-1b")
	}
}

func TestPlace_WithKeyRetriesAndResendsSameKey(t *testing.T) {
	// 키가 있으면 1회 재시도하되, 두 요청의 clientOrderId 가 같아야 서버가 중복을 제거할 수 있다
	var n int32
	var keys []string
	var mu sync.Mutex
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body struct {
			ClientOrderID string `json:"clientOrderId"`
		}
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		keys = append(keys, body.ClientOrderID)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"conditionalOrderId":"c-9","clientOrderId":"k9"}}`))
	})
	defer done()
	if _, err := New(hc, 4).Place(context.Background(), PlaceRequest{
		Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31",
		First: Condition{OrderSide: SideSell, TriggerPrice: d("1")}, ClientOrderID: "k9",
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != "k9" || keys[1] != "k9" {
		t.Errorf("clientOrderId per attempt = %q, want [k9 k9]", keys)
	}
}

func TestModify_Validation(t *testing.T) {
	// Modify 도 Place 와 같은 validateCommon 을 타는지 고정한다(id 자체는 유효해 통과시키고, body 규칙만 어긴다).
	c := New(nil, 4) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	sell1 := Condition{OrderSide: SideSell, TriggerPrice: d("1"), OrderPrice: d("1")}
	sell2 := Condition{OrderSide: SideSell, TriggerPrice: d("2"), OrderPrice: d("2")}
	buy2 := Condition{OrderSide: SideBuy, TriggerPrice: d("2"), OrderPrice: d("2")}
	cases := map[string]ModifyRequest{
		"single with second": {
			Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31",
			First: Condition{OrderSide: SideSell, TriggerPrice: d("1")}, Second: &sell1,
		},
		"oco without second": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell2,
		},
		"market with orderPrice": {
			Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: sell1,
		},
		"oco first buy": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: buy2, Second: &sell1,
		},
		"oco price order (first must exceed second)": {
			Type: TypeOCO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2,
		},
		"oto first sell (must be buy)": {
			Type: TypeOTO, Quantity: d("1"), OrderType: OrderTypeLimit, ExpireDate: "2026-12-31", First: sell1, Second: &sell2,
		},
	}
	for name, r := range cases {
		if _, err := c.Modify(ctx, "c-1", r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCancel_NoContent(t *testing.T) {
	c, _, done := captureBody(t, "/api/v1/conditional-orders/c-1", http.MethodDelete, "4", 204, nil)
	defer done()
	if err := c.Cancel(context.Background(), "c-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestCancel_RequiresID(t *testing.T) {
	if err := New(nil, 4).Cancel(context.Background(), ""); err == nil {
		t.Error("want error")
	}
}

// countingUnauthorized 는 항상 401 invalid-token 을 돌려주며 요청 수를 센다.
func countingUnauthorized(t *testing.T) (*Client, *int32, func()) {
	t.Helper()
	var n int32
	hc, done := testutil.NewServerFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"invalid-token","message":""}}`))
	})
	return New(hc, 4), &n, done
}

func TestWrites_WithoutKeyAreNeverRetried(t *testing.T) {
	// 멱등성 키 없는 쓰기는 401 이어도 절대 재전송되지 않는다 — 중복 조건주문 방지의 핵심 불변식
	ctx := context.Background()
	ok := Condition{OrderSide: SideSell, TriggerPrice: d("1")}
	for name, call := range map[string]func(c *Client) error{
		"Place": func(c *Client) error {
			_, err := c.Place(ctx, PlaceRequest{Symbol: "005930", Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok})
			return err
		},
		"Modify": func(c *Client) error {
			_, err := c.Modify(ctx, "c-1", ModifyRequest{Type: TypeSingle, Quantity: d("1"), OrderType: OrderTypeMarket, ExpireDate: "2026-12-31", First: ok})
			return err
		},
		"Cancel": func(c *Client) error { return c.Cancel(ctx, "c-1") },
	} {
		c, n, done := countingUnauthorized(t)
		if err := call(c); err == nil {
			t.Errorf("%s: want error", name)
		}
		if got := atomic.LoadInt32(n); got != 1 {
			t.Errorf("%s: %d requests, want exactly 1 (재시도 금지)", name, got)
		}
		done()
	}
}
