package stream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

// frameKind 는 수신 프레임 종류. 서버는 top-level "type" 으로 구분한다.
type frameKind int

const (
	frameUnknown frameKind = iota // 알 수 없는 type — 무시한다(서버가 프레임을 추가할 수 있다)
	frameSubscriptions
	frameMessage
	frameError
	framePong
)

// rejectedItem 은 ack 의 rejected[] 원소.
type rejectedItem struct {
	Target  string `json:"target"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// frame 은 디코딩된 수신 프레임.
type frame struct {
	kind  frameKind
	id    string // subscriptions/error 프레임에서 요청 id echo
	topic string // message 프레임
	data  json.RawMessage

	subscribed []string
	rejected   []rejectedItem

	errCode    string
	errMessage string
}

type wireFrame struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	Data       json.RawMessage `json:"data"`
	Subscribed []string        `json:"subscribed"`
	Rejected   []rejectedItem  `json:"rejected"`
	Error      *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeFrame 은 텍스트 프레임을 해석한다. 알 수 없는 type 은 frameUnknown 이며 에러가 아니다.
func decodeFrame(raw []byte) (frame, error) {
	var w wireFrame
	if err := json.Unmarshal(raw, &w); err != nil {
		return frame{}, fmt.Errorf("toss: decode frame: %w", err)
	}
	f := frame{id: w.ID, topic: w.Topic, data: w.Data, subscribed: w.Subscribed, rejected: w.Rejected}
	switch w.Type {
	case "subscriptions":
		f.kind = frameSubscriptions
	case "message":
		// 구조 검증은 여기서 한다 — 모든 수신 프레임이 지나는 단일 지점이고,
		// data 가 null/부재면 payload 언마샬이 에러 없이 zero value 를 만들어(가격 0 인 체결 등)
		// 사용자 채널로 흘러들어간다.
		if w.Topic == "" {
			return frame{}, fmt.Errorf("toss: message frame has no topic")
		}
		if len(w.Data) == 0 || string(w.Data) == "null" {
			return frame{}, fmt.Errorf("toss: message frame %q has no data", w.Topic)
		}
		f.kind = frameMessage
	case "error":
		f.kind = frameError
		if w.Error != nil && w.Error.Code != "" {
			f.errCode, f.errMessage = w.Error.Code, w.Error.Message
		} else {
			if w.Error != nil {
				f.errMessage = w.Error.Message
			}
			f.errCode = "unknown" // 서버가 새 에러 형태를 보내도 스트림을 죽이지 않되, 메시지는 진단 가능하게
		}
	case "pong":
		f.kind = framePong
	default:
		f.kind = frameUnknown
	}
	return f, nil
}

// topicKind 는 message 프레임의 topic 이 어느 채널인지 돌려준다.
func (f frame) topicKind() string {
	typ, _, ok := splitTopic(f.topic)
	if !ok {
		return ""
	}
	return typ
}

// prefix 는 subscription.go 의 typeTrade*/typeOrderbook* 와 짝을 이룬다. 시장 세그먼트는
// 검증하지 않는다 — 토스가 시장을 추가해도 디코딩이 깨지지 않도록 관용적으로 둔다.
func (f frame) marketSymbol(prefix string) (tosstypes.MarketCountry, string, error) {
	typ, code, ok := splitTopic(f.topic)
	if !ok || !strings.HasPrefix(typ, prefix+":") {
		return "", "", fmt.Errorf("toss: unexpected topic %q for %s event", f.topic, prefix)
	}
	market := tosstypes.MarketCountry(strings.ToUpper(strings.TrimPrefix(typ, prefix+":")))
	return market, code, nil
}

type wireTrade struct {
	Price     decimal.Decimal    `json:"price"`
	Volume    decimal.Decimal    `json:"volume"`
	Timestamp time.Time          `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
}

// tradeEvent 는 message 프레임을 체결 이벤트로 해석한다.
func (f frame) tradeEvent() (TradeEvent, error) {
	market, symbol, err := f.marketSymbol("trade")
	if err != nil {
		return TradeEvent{}, err
	}
	var w wireTrade
	if err := json.Unmarshal(f.data, &w); err != nil {
		return TradeEvent{}, fmt.Errorf("toss: decode trade data: %w", err)
	}
	// 값 검증 — 빈 객체나 일부 필드만 담긴 payload 는 언마샬이 통과해 zero value(가격 0 체결)를 만든다.
	// 체결에 시각·가격·수량이 없는 경우는 없으므로 이벤트로 내보내지 않고 에러로 돌린다.
	if w.Timestamp.IsZero() || !w.Price.IsPositive() || !w.Volume.IsPositive() {
		return TradeEvent{}, fmt.Errorf("toss: incomplete trade data for %q (price=%s volume=%s)", f.topic, w.Price, w.Volume)
	}
	return TradeEvent{
		Market: market, Symbol: symbol,
		Price: w.Price, Volume: w.Volume, Timestamp: w.Timestamp, Currency: w.Currency,
	}, nil
}

type wireOrderbook struct {
	Timestamp *time.Time         `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
	Asks      []Level            `json:"asks"`
	Bids      []Level            `json:"bids"`
}

// orderbookEvent 는 message 프레임을 호가 이벤트로 해석한다.
// 체결·주문과 달리 값 검증을 하지 않는다 — 장 시작 전처럼 호가가 비어 있는 상태가 실제로 존재한다.
func (f frame) orderbookEvent() (OrderbookEvent, error) {
	market, symbol, err := f.marketSymbol("orderbook")
	if err != nil {
		return OrderbookEvent{}, err
	}
	var w wireOrderbook
	if err := json.Unmarshal(f.data, &w); err != nil {
		return OrderbookEvent{}, fmt.Errorf("toss: decode orderbook data: %w", err)
	}
	return OrderbookEvent{
		Market: market, Symbol: symbol,
		Timestamp: w.Timestamp, Currency: w.Currency, Asks: w.Asks, Bids: w.Bids,
	}, nil
}

type wireOrder struct {
	Event OrderEventType `json:"event"`
	// AccountSeq 는 와이어 문서화용. 실제 값은 topic 에서 파싱한다(스펙상 동일).
	// 불일치해도 에러로 만들지 않는다 — 무손실 채널의 이벤트를 버리는 대가가 더 크다.
	AccountSeq string      `json:"accountSeq"`
	Order      order.Order `json:"order"`
}

// orderEvent 는 message 프레임을 주문 이벤트로 해석한다.
// 스트림의 주문 스냅샷에는 execution.filledAt 이 없어 Order.Execution.FilledAt 은 항상 nil 이다.
func (f frame) orderEvent() (OrderEvent, error) {
	typ, code, ok := splitTopic(f.topic)
	if !ok || typ != typePersonalOrder {
		return OrderEvent{}, fmt.Errorf("toss: unexpected topic %q for order event", f.topic)
	}
	var w wireOrder
	if err := json.Unmarshal(f.data, &w); err != nil {
		return OrderEvent{}, fmt.Errorf("toss: decode order data: %w", err)
	}
	seq, err := strconv.ParseInt(code, 10, 64)
	if err != nil {
		return OrderEvent{}, fmt.Errorf("toss: invalid accountSeq in topic %q: %w", f.topic, err)
	}
	// 값 검증 — 빈 payload 는 언마샬이 통과한다. 무손실 채널이라 빈 이벤트를 흘리면 소비자가 잘못 판단한다.
	if w.Event == "" || w.Order.OrderID == "" {
		return OrderEvent{}, fmt.Errorf("toss: incomplete order data for %q (event=%q orderId=%q)", f.topic, w.Event, w.Order.OrderID)
	}
	return OrderEvent{Event: w.Event, AccountSeq: seq, Order: w.Order}, nil
}
