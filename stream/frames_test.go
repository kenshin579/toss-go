package stream

import (
	"testing"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestSplitTopic(t *testing.T) {
	cases := map[string][2]string{
		"trade:us:AAPL":       {"trade:us", "AAPL"},
		"orderbook:kr:005930": {"orderbook:kr", "005930"},
		"personal:order:3":    {"personal:order", "3"},
		"trade:us:BRK.B":      {"trade:us", "BRK.B"},
	}
	for in, want := range cases {
		typ, code, ok := splitTopic(in)
		if !ok || typ != want[0] || code != want[1] {
			t.Errorf("splitTopic(%q) = %q,%q,%v", in, typ, code, ok)
		}
	}
	for _, bad := range []string{"", "trade", ":", "trade:"} {
		if _, _, ok := splitTopic(bad); ok {
			t.Errorf("splitTopic(%q) must fail", bad)
		}
	}
}

func TestDecodeFrame_Trade(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"trade:us:AAPL","data":{"price":"185.25","volume":"3","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"USD"}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameMessage {
		t.Fatalf("kind = %v", f.kind)
	}
	ev, err := f.tradeEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Market != tosstypes.MarketCountryUS || ev.Symbol != "AAPL" {
		t.Errorf("topic parse = %+v", ev)
	}
	if ev.Price.String() != "185.25" || ev.Volume.String() != "3" || ev.Currency != tosstypes.CurrencyUSD {
		t.Errorf("data = %+v", ev)
	}
	if ev.Timestamp.IsZero() || ev.Timestamp.Minute() != 30 {
		t.Errorf("timestamp = %v", ev.Timestamp)
	}
}

func TestDecodeFrame_Orderbook(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":null,"currency":"KRW","asks":[{"price":"72100","volume":"8500"},{"price":"72200","volume":"100"}],"bids":[{"price":"72000","volume":"500"}]}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderbookEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Market != tosstypes.MarketCountryKR || ev.Symbol != "005930" || ev.Currency != tosstypes.CurrencyKRW {
		t.Errorf("ev = %+v", ev)
	}
	if ev.Timestamp != nil {
		t.Errorf("null timestamp must decode to nil: %v", ev.Timestamp)
	}
	if len(ev.Asks) != 2 || ev.Asks[0].Price.String() != "72100" || ev.Asks[0].Volume.String() != "8500" {
		t.Errorf("asks = %+v", ev.Asks)
	}
	if len(ev.Bids) != 1 || ev.Bids[0].Price.String() != "72000" {
		t.Errorf("bids = %+v", ev.Bids)
	}
}

func TestDecodeFrame_Order(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"personal:order:3","data":{"event":"FILL","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"FILLED","price":"70000","quantity":"10","orderAmount":null,"currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","canceledAt":null,"execution":{"filledQuantity":"10","averageFilledPrice":"70000","filledAmount":"700000","commission":"1400","tax":"0","settlementDate":"2026-03-30"}}}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Event != OrderEventFill || ev.AccountSeq != 3 {
		t.Errorf("ev = %+v", ev)
	}
	if ev.Order.OrderID != "o-1" || ev.Order.Status != "FILLED" || ev.Order.Quantity.String() != "10" {
		t.Errorf("order = %+v", ev.Order)
	}
	if ev.Order.Execution.AverageFilledPrice == nil || ev.Order.Execution.AverageFilledPrice.String() != "70000" {
		t.Errorf("execution = %+v", ev.Order.Execution)
	}
	// 스트림에는 filledAt 이 없다
	if ev.Order.Execution.FilledAt != nil {
		t.Errorf("스트림 주문 스냅샷에는 filledAt 이 없어야 한다: %v", ev.Order.Execution.FilledAt)
	}
	if ev.Order.Execution.SettlementDate == nil || *ev.Order.Execution.SettlementDate != "2026-03-30" {
		t.Errorf("settlementDate = %v", ev.Order.Execution.SettlementDate)
	}
}

func TestDecodeFrame_SubscriptionsAck(t *testing.T) {
	raw := []byte(`{"type":"subscriptions","id":"d-1","subscribed":["trade:us:AAPL"],"rejected":[{"target":"trade:us:NOPE","code":"stock-not-found","message":"없음"}]}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameSubscriptions || f.id != "d-1" {
		t.Fatalf("f = %+v", f)
	}
	if len(f.subscribed) != 1 || f.subscribed[0] != "trade:us:AAPL" {
		t.Errorf("subscribed = %v", f.subscribed)
	}
	if len(f.rejected) != 1 || f.rejected[0].Target != "trade:us:NOPE" || f.rejected[0].Code != "stock-not-found" {
		t.Errorf("rejected = %+v", f.rejected)
	}
}

func TestDecodeFrame_ErrorAndPong(t *testing.T) {
	f, err := decodeFrame([]byte(`{"type":"error","id":"d-2","error":{"code":"rate-limit-exceeded","message":"too fast"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameError || f.errCode != "rate-limit-exceeded" || f.id != "d-2" {
		t.Errorf("f = %+v", f)
	}
	f, err = decodeFrame([]byte(`{"type":"pong"}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != framePong {
		t.Errorf("f = %+v", f)
	}
}

func TestDecodeFrame_UnknownTypeIsIgnored(t *testing.T) {
	f, err := decodeFrame([]byte(`{"type":"brand-new-frame","x":1}`))
	if err != nil {
		t.Fatalf("알 수 없는 프레임은 에러가 아니라 무시 대상이어야 한다: %v", err)
	}
	if f.kind != frameUnknown {
		t.Errorf("kind = %v", f.kind)
	}
}

func TestDecodeFrame_Malformed(t *testing.T) {
	if _, err := decodeFrame([]byte(`not json`)); err == nil {
		t.Error("깨진 JSON 은 에러여야 한다")
	}
}

func TestFrame_TopicMismatch(t *testing.T) {
	f, _ := decodeFrame([]byte(`{"type":"message","topic":"orderbook:kr:005930","data":{}}`))
	if _, err := f.tradeEvent(); err == nil {
		t.Error("호가 topic 을 체결로 해석하면 에러여야 한다")
	}
}
