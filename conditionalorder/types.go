package conditionalorder

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Type 은 조건주문 구성.
type Type string

const (
	TypeSingle Type = "SINGLE" // 조건 1개
	TypeOCO    Type = "OCO"    // 둘 중 하나가 발동하면 나머지 취소(One-Cancels-Other)
	TypeOTO    Type = "OTO"    // 첫 조건 발동 후 두 번째가 활성화(One-Triggers-Other)
)

// OrderType 은 조건 충족 시 낼 주문의 호가 유형.
type OrderType string

const (
	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"
)

// Side 는 조건 충족 시 낼 주문의 방향.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// Status 는 조건주문 전체 상태 — 살아있는 조건(leg)의 상태를 대표로 따른다.
// leg 전용 상태인 HOLDING/CANCELED 는 최상위 status 로는 내려오지 않는다.
type Status string

const (
	StatusWatching  Status = "WATCHING"  // 조건 감시 중
	StatusPaused    Status = "PAUSED"    // 일시 중지
	StatusOrdering  Status = "ORDERING"  // 조건 충족 — 주문 생성 진행 중
	StatusOrdered   Status = "ORDERED"   // 주문 생성됨
	StatusCompleted Status = "COMPLETED" // 완료
	StatusExpired   Status = "EXPIRED"   // 만료
)

// StatusFilter 는 목록 조회의 라이프사이클 그룹 필터. 개별 Status 와 값 체계가 다르다.
type StatusFilter string

const (
	// StatusFilterOpen 은 진행 중 — WATCHING, PAUSED, ORDERING, ORDERED.
	StatusFilterOpen StatusFilter = "OPEN"
	// StatusFilterClosed 는 종료 — COMPLETED, EXPIRED.
	StatusFilterClosed StatusFilter = "CLOSED"
)

// ConditionType 은 개별 조건(leg)의 종류. 그룹(OCO/OTO)의 first/second 는 항상 같은 타입이다.
type ConditionType string

const (
	ConditionStop       ConditionType = "STOP"        // 가격 트리거
	ConditionProfitRate ConditionType = "PROFIT_RATE" // 목표 수익률(%) 트리거
)

// ConditionStatus 는 개별 조건(leg)의 상태.
type ConditionStatus string

const (
	ConditionWatching  ConditionStatus = "WATCHING"
	ConditionHolding   ConditionStatus = "HOLDING" // 선행 조건(OTO first) 체결 전 대기(leg 전용)
	ConditionPaused    ConditionStatus = "PAUSED"
	ConditionOrdering  ConditionStatus = "ORDERING"
	ConditionOrdered   ConditionStatus = "ORDERED"
	ConditionCompleted ConditionStatus = "COMPLETED"
	ConditionExpired   ConditionStatus = "EXPIRED"
	ConditionCanceled  ConditionStatus = "CANCELED" // 완료된 OCO 에서 자동취소된 반대편 조건(leg 전용)
)

// ConditionDetail 은 조회 응답의 개별 조건(leg).
type ConditionDetail struct {
	Type             ConditionType    `json:"type"`
	Status           ConditionStatus  `json:"status"`
	TriggerPrice     *decimal.Decimal `json:"triggerPrice"`     // STOP 조건에만
	TargetProfitRate *decimal.Decimal `json:"targetProfitRate"` // PROFIT_RATE 조건에만. 퍼센트 단위(10.5 = +10.5%)
	OrderPrice       *decimal.Decimal `json:"orderPrice"`       // 그룹 orderType 이 LIMIT 일 때 발동 주문 가격. MARKET 이면 nil
	TriggeredOrderID *string          `json:"triggeredOrderId"` // 발동해서 생성된 주문 id. 미발동이면 nil
}

// Detail 은 조건주문 1건. 목록·상세가 같은 스키마를 쓴다.
type Detail struct {
	ConditionalOrderID string                  `json:"conditionalOrderId"`
	Type               Type                    `json:"type"`
	Status             Status                  `json:"status"`
	Symbol             string                  `json:"symbol"`
	Market             tosstypes.MarketCountry `json:"market"` // KR / US
	Quantity           decimal.Decimal         `json:"quantity"`
	OrderType          OrderType               `json:"orderType"`
	ExpireDate         *tosstypes.Date         `json:"expireDate"`
	First              ConditionDetail         `json:"first"`
	Second             *ConditionDetail        `json:"second"` // SINGLE 이면 nil
	CreatedAt          time.Time               `json:"createdAt"`
}

// Page 는 조건주문 목록 한 페이지.
type Page struct {
	ConditionalOrders []Detail `json:"conditionalOrders"`
	NextCursor        *string  `json:"nextCursor"` // 더 없으면 nil
	HasNext           bool     `json:"hasNext"`
}

// PlaceResult 는 조건주문 생성 결과.
type PlaceResult struct {
	ConditionalOrderID string  `json:"conditionalOrderId"`
	ClientOrderID      *string `json:"clientOrderId"` // 요청에 넣었을 때만 설정
}

// Result 는 조건주문 수정 결과.
type Result struct {
	ConditionalOrderID string `json:"conditionalOrderId"`
}
