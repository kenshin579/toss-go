package order

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Side 는 주문 방향.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// Type 은 호가 유형.
type Type string

const (
	TypeLimit  Type = "LIMIT"  // 지정가
	TypeMarket Type = "MARKET" // 시장가
)

// TimeInForce 는 주문 유효 조건. OrderType 과 결합해 주문 방식이 정해진다(LIMIT+CLS = LOC).
type TimeInForce string

const (
	TimeInForceDay   TimeInForce = "DAY" // 당일 유효. 미지정 시 서버 기본값
	TimeInForceClose TimeInForce = "CLS" // 장 마감(At the Close). 미국 주식 + LIMIT 만 지원
	TimeInForceOpen  TimeInForce = "OPG" // 장 개시(국내 시가단일가). 국내 전용
)

// Status 는 개별 주문의 상태.
type Status string

const (
	StatusPending         Status = "PENDING"          // 접수, 미체결
	StatusPendingCancel   Status = "PENDING_CANCEL"   // 취소 처리 중
	StatusPendingReplace  Status = "PENDING_REPLACE"  // 정정 처리 중
	StatusPartialFilled   Status = "PARTIAL_FILLED"   // 부분 체결
	StatusFilled          Status = "FILLED"           // 전량 체결
	StatusCanceled        Status = "CANCELED"         // 취소됨
	StatusRejected        Status = "REJECTED"         // 거부됨
	StatusCancelRejected  Status = "CANCEL_REJECTED"  // 취소 거부
	StatusReplaceRejected Status = "REPLACE_REJECTED" // 정정 거부
	StatusReplaced        Status = "REPLACED"         // 정정되어 대체됨
)

// StatusFilter 는 주문 목록 조회의 라이프사이클 그룹 필터다. 개별 주문의 Status 와 값 체계가 다르다.
type StatusFilter string

const (
	// StatusFilterOpen 은 진행 중 그룹 — PENDING, PARTIAL_FILLED, PENDING_CANCEL, PENDING_REPLACE.
	StatusFilterOpen StatusFilter = "OPEN"
	// StatusFilterClosed 는 종료 그룹 — FILLED, CANCELED, REJECTED, REPLACED, CANCEL_REJECTED,
	// REPLACE_REJECTED, PARTIAL_FILLED.
	StatusFilterClosed StatusFilter = "CLOSED"
)

// Execution 은 체결 정보. 미체결 주문은 FilledQuantity 가 0 이고 나머지는 nil 이다.
type Execution struct {
	FilledQuantity     decimal.Decimal  `json:"filledQuantity"`
	AverageFilledPrice *decimal.Decimal `json:"averageFilledPrice"` // 미체결이면 nil
	FilledAmount       *decimal.Decimal `json:"filledAmount"`
	Commission         *decimal.Decimal `json:"commission"`
	Tax                *decimal.Decimal `json:"tax"`
	FilledAt           *time.Time       `json:"filledAt"`
	SettlementDate     *tosstypes.Date  `json:"settlementDate"` // 결제일. 체결 전이면 nil
}

// Order 는 주문 1건.
type Order struct {
	OrderID     string             `json:"orderId"`
	Symbol      string             `json:"symbol"`
	Side        Side               `json:"side"`
	OrderType   Type               `json:"orderType"`
	TimeInForce TimeInForce        `json:"timeInForce"`
	Status      Status             `json:"status"`
	Price       *decimal.Decimal   `json:"price"` // 시장가 주문이면 nil
	Quantity    decimal.Decimal    `json:"quantity"`
	OrderAmount *decimal.Decimal   `json:"orderAmount"` // 금액 주문이면 설정, 수량 주문이면 nil
	Currency    tosstypes.Currency `json:"currency"`
	OrderedAt   time.Time          `json:"orderedAt"`
	CanceledAt  *time.Time         `json:"canceledAt"` // 취소되지 않았으면 nil
	Execution   Execution          `json:"execution"`
}

// Page 는 주문 목록 한 페이지. NextCursor 를 다음 요청의 Cursor 로 넘기면 이어서 조회한다.
type Page struct {
	Orders     []Order `json:"orders"`
	NextCursor *string `json:"nextCursor"` // 더 없으면 nil
	HasNext    bool    `json:"hasNext"`
}

// PlaceResult 는 주문 생성 결과.
type PlaceResult struct {
	OrderID       string  `json:"orderId"`
	ClientOrderID *string `json:"clientOrderId"` // 요청에 넣었을 때만 설정
}

// OperationResult 는 정정·취소 결과.
//
// OrderID 는 **정정/취소로 새로 발급된 주문 식별자**이며 원주문의 OrderID 와 다르다.
// 이후 조회·정정·취소에는 반드시 이 값을 써야 한다 — 원주문 ID 로 다시 취소를 시도하면
// 이미 처리된 주문을 다시 건드리게 된다.
type OperationResult struct {
	OrderID string `json:"orderId"` // 새로 발급된 주문 id(원주문 id 와 다름)
}
