package stream

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	want, _ := time.Parse(time.RFC3339, "2026-03-25T09:30:42.000+09:00")
	if !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", ev.Timestamp, want)
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

func TestDecodeFrame_Orderbook_WithTimestamp(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":"2026-06-18T23:30:00.000+09:00","currency":"KRW","asks":[{"price":"71500","volume":"5"}],"bids":[{"price":"71400","volume":"10"}]}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderbookEvent()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-06-18T23:30:00.000+09:00")
	if ev.Timestamp == nil || !ev.Timestamp.Equal(want) {
		t.Errorf("non-nil timestamp = %v, want %v", ev.Timestamp, want)
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

func TestDecodeFrame_MessageStructure(t *testing.T) {
	for name, raw := range map[string]string{
		"no topic":    `{"type":"message","data":{"price":"1"}}`,
		"empty topic": `{"type":"message","topic":"","data":{"price":"1"}}`,
		"null data":   `{"type":"message","topic":"trade:kr:005930","data":null}`,
		"no data":     `{"type":"message","topic":"trade:kr:005930"}`,
	} {
		if _, err := decodeFrame([]byte(raw)); err == nil {
			t.Errorf("%s: 구조가 깨진 message 프레임은 에러여야 한다(가격 0 이벤트 방지)", name)
		}
	}
}

func TestFrame_ConverterErrors(t *testing.T) {
	// 값이 깨진 payload 는 zero value 가 아니라 에러여야 한다
	bad := map[string]string{
		"non-numeric price": `{"type":"message","topic":"trade:kr:005930","data":{"price":"abc","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
		"empty price":       `{"type":"message","topic":"trade:kr:005930","data":{"price":"","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
		"bad timestamp":     `{"type":"message","topic":"trade:kr:005930","data":{"price":"1","volume":"1","timestamp":"nope","currency":"KRW"}}`,
	}
	for name, raw := range bad {
		f, err := decodeFrame([]byte(raw))
		if err != nil {
			t.Fatalf("%s: decode = %v", name, err)
		}
		if _, err := f.tradeEvent(); err == nil {
			t.Errorf("%s: 값 오류는 에러여야 한다", name)
		}
	}
	// accountSeq 가 숫자가 아닌 topic
	f, _ := decodeFrame([]byte(`{"type":"message","topic":"personal:order:abc","data":{"event":"FILL","accountSeq":"abc","order":{}}}`))
	if _, err := f.orderEvent(); err == nil {
		t.Error("숫자가 아닌 accountSeq 는 에러여야 한다")
	}
	// 변환기 × 잘못된 topic 조합
	mismatch := []struct {
		topic string
		conv  func(frame) error
	}{
		{"orderbook:kr:005930", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"personal:order:3", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"trade:kr:005930", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"personal:order:3", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"trade:kr:005930", func(f frame) error { _, err := f.orderEvent(); return err }},
		{"orderbook:kr:005930", func(f frame) error { _, err := f.orderEvent(); return err }},
	}
	for _, m := range mismatch {
		f, err := decodeFrame([]byte(`{"type":"message","topic":"` + m.topic + `","data":{}}`))
		if err != nil {
			t.Fatalf("decode(%s) = %v", m.topic, err)
		}
		if err := m.conv(f); err == nil {
			t.Errorf("%s: 다른 채널 변환기는 에러여야 한다", m.topic)
		}
	}
}

// TestDecodeFrame_AsyncAPIExamples 는 docs/api/asyncapi.json 의 실제 예시 payload(stream/testdata/frames/*.json)로
// decodeFrame + 해당 변환기가 성공하는지 확인한다. 손으로 적은 테스트 리터럴이 스펙과 어긋나는 것을 막는 회귀 테스트다.
func TestDecodeFrame_AsyncAPIExamples(t *testing.T) {
	cases := []struct {
		file string
		conv func(frame) error
	}{
		{"trade.json", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"orderbook.json", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"order.json", func(f frame) error { _, err := f.orderEvent(); return err }},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "frames", c.file))
			if err != nil {
				t.Fatal(err)
			}
			f, err := decodeFrame(raw)
			if err != nil {
				t.Fatalf("decodeFrame: %v", err)
			}
			if f.kind != frameMessage {
				t.Fatalf("kind = %v", f.kind)
			}
			if err := c.conv(f); err != nil {
				t.Errorf("converter: %v", err)
			}
		})
	}
}

// FuzzDecodeFrame 은 임의의 입력에도 decodeFrame·변환기가 panic 하지 않음을 확인한다.
func FuzzDecodeFrame(f *testing.F) {
	f.Add(`{"type":"message","topic":"trade:kr:005930","data":{"price":"1","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`)
	f.Add(`{"type":"subscriptions","subscribed":["trade:kr:005930"],"rejected":[]}`)
	f.Add(`{"type":"error","error":{"code":"x","message":"y"}}`)
	f.Add(`{"type":"pong"}`)
	f.Fuzz(func(t *testing.T, raw string) {
		fr, err := decodeFrame([]byte(raw))
		if err != nil {
			return
		}
		if fr.kind != frameMessage {
			return
		}
		// 어떤 입력에도 panic 하지 않아야 한다
		_, _ = fr.tradeEvent()
		_, _ = fr.orderbookEvent()
		_, _ = fr.orderEvent()
	})
}
