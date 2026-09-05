// Package stream 은 토스증권 Open API 의 실시간 웹소켓 스트림이다 — 체결·호가·본인 주문 이벤트.
//
// toss.Client.Stream 으로 만든다. 연결 하나 위에서 선언형(full-replace) 구독으로 동작하며,
// SDK 가 현재 구독 집합을 들고 있다가 Subscribe/Unsubscribe 때마다 전체를 다시 선언한다.
//
//	s, _ := c.Stream(ctx)
//	defer s.Close()
//	s.Subscribe(ctx, stream.Trade(tosstypes.MarketCountryKR, "005930"))
//	for t := range s.Trades() { ... }
//
// # 전달 보장
//
// 시세(Trades·Orderbooks)는 LOSSY 다 — 소비가 밀리면 중간 이벤트가 버려지고 최신이 우선한다.
// 주문(Orders)은 연결 세션 안에서 LOSSLESS 지만, **끊긴 구간의 이벤트는 재전송되지 않는다.**
// 따라서 Reconnects() 신호를 받으면 REST(Account(seq).Order.List)로 주문 상태를 재동기화해야 한다.
//
// # 한도
//
// 계정당 동시 연결 2개, 연결당 구독 100건(채널×종목 조합), 선언 5회/초.
// keepalive PING 은 SDK 가 알아서 보낸다(기본 60초).
package stream

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

// TradeEvent 는 실시간 체결 1건.
type TradeEvent struct {
	Market    tosstypes.MarketCountry // 이 이벤트가 속한 시장
	Symbol    string                  // 종목 심볼
	Price     decimal.Decimal         // 체결가
	Volume    decimal.Decimal         // 체결 수량
	Timestamp time.Time               // 체결 시각
	Currency  tosstypes.Currency
}

// Level 은 호가 한 단계.
type Level struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

// OrderbookEvent 는 실시간 호가 스냅샷.
type OrderbookEvent struct {
	Market    tosstypes.MarketCountry
	Symbol    string
	Timestamp *time.Time // 데이터 미제공 시 nil
	Currency  tosstypes.Currency
	Asks      []Level // 매도호가(낮은 가격순)
	Bids      []Level // 매수호가(높은 가격순)
}

// OrderEventType 은 주문 이벤트 종류. unknown 값도 그대로 보존된다.
type OrderEventType string

const (
	OrderEventPending         OrderEventType = "PENDING"          // 접수
	OrderEventPartialFill     OrderEventType = "PARTIAL_FILL"     // 부분 체결
	OrderEventFill            OrderEventType = "FILL"             // 전량 체결
	OrderEventCanceling       OrderEventType = "CANCELING"        // 취소 중
	OrderEventCanceled        OrderEventType = "CANCELED"         // 취소됨
	OrderEventReplacing       OrderEventType = "REPLACING"        // 정정 중
	OrderEventReplaced        OrderEventType = "REPLACED"         // 정정됨
	OrderEventRejected        OrderEventType = "REJECTED"         // 주문 거부
	OrderEventCancelRejected  OrderEventType = "CANCEL_REJECTED"  // 취소 거부
	OrderEventReplaceRejected OrderEventType = "REPLACE_REJECTED" // 정정 거부
)

// OrderEvent 는 본인 계좌의 주문 이벤트.
type OrderEvent struct {
	Event      OrderEventType
	AccountSeq int64 // 구독 시 넣은 accountSeq(topic 에서 파싱)

	// Order 는 REST 주문 조회와 동일한 스냅샷이다.
	// 단 스트림에는 Execution.FilledAt 이 내려오지 않아 항상 nil 이다 — 체결 시각이 필요하면
	// Account(seq).Order.Get 으로 다시 조회한다.
	Order order.Order
}

// ReconnectCause 는 재연결이 일어난 이유.
type ReconnectCause string

const (
	// ReconnectServerShutdown 은 서버 배포로 인한 종료(server-shutdown 프레임 수신).
	ReconnectServerShutdown ReconnectCause = "server-shutdown"
	// ReconnectBackpressure 는 Orders() 를 소비하지 않아 SDK 가 연결을 끊은 경우.
	// 주문 이벤트가 유실됐을 수 있으니 REST 로 재동기화한다.
	ReconnectBackpressure ReconnectCause = "backpressure"
	// ReconnectReadError 는 읽기 실패·비정상 종료.
	ReconnectReadError ReconnectCause = "read-error"
)

// Reconnect 는 재연결이 성공했음을 알린다.
//
// 끊긴 구간의 주문 이벤트는 재전송되지 않으므로, 이 신호를 받으면 REST 로 주문 상태를 재동기화해야 한다.
type Reconnect struct {
	Attempt int            // 몇 번째 시도에서 성공했는지(1부터)
	Cause   ReconnectCause // 직전 연결이 끊긴 이유
	At      time.Time
}

// ConnectError 는 웹소켓 핸드셰이크 실패. 401 토큰 문제, 403 허용 IP 미등록, 503 서버 오류 등.
type ConnectError struct {
	StatusCode int
	Err        error
}

func (e *ConnectError) Error() string {
	return fmt.Sprintf("toss: websocket handshake failed (status %d): %v", e.StatusCode, e.Err)
}

func (e *ConnectError) Unwrap() error { return e.Err }

// DeclareError 는 구독 선언 전체가 실패했을 때의 error 프레임.
// 기존 구독은 유지된다. Code 는 wrong-format·no-type·invalid-type·no-codes·too-many-topics·
// too-many·rate-limit-exceeded·internal-error·server-shutdown 중 하나이며 unknown 값도 올 수 있다.
// 단 server-shutdown 은 SDK 가 에러로 취급하지 않고 재연결 사유(ReconnectServerShutdown)로 처리한다.
type DeclareError struct {
	ID      string // 요청에 id 를 보냈을 때만 채워진다
	Code    string
	Message string
}

func (e *DeclareError) Error() string {
	return fmt.Sprintf("toss: declare failed: %s: %s", e.Code, e.Message)
}

// RejectedError 는 구독 선언의 일부 항목이 거부됐을 때다. 나머지 항목은 정상 구독된다.
// 거부된 항목은 SDK 가 구독 집합에서 자동으로 제거한다 — 원인을 고치기 전에 재선언해도 다시 거부되기 때문이다.
// Code 는 stock-not-found·symbol-market-mismatch·account-not-found 중 하나.
type RejectedError struct {
	Target  string // full key. 예: trade:us:NOPE
	Code    string
	Message string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("toss: subscription rejected %s: %s: %s", e.Target, e.Code, e.Message)
}
