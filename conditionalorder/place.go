package conditionalorder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// validateKey 는 internal/params.ClientOrderIDFormat 에 위임한다(빈 값만 여기서 "멱등성 미적용"으로 통과시킨다).
func validateKey(id string) error {
	if id == "" {
		return nil // 멱등성 미적용
	}
	return params.ClientOrderIDFormat(id)
}

// Condition 은 생성·수정 요청의 조건 1개.
type Condition struct {
	OrderSide    Side            // 필수
	TriggerPrice decimal.Decimal // 필수. 이 가격에 도달하면 발동
	OrderPrice   decimal.Decimal // 그룹 orderType 이 LIMIT 이면 필수, MARKET 이면 반드시 zero(설정하면 사전 검증에서 거부)
}

type conditionBody struct {
	OrderSide    Side   `json:"orderSide"`
	TriggerPrice string `json:"triggerPrice"`
	OrderPrice   string `json:"orderPrice,omitempty"`
}

// validate 는 조건 1개의 형식을 검증한다. ot 는 그룹 공통 호가유형(orderType) — LIMIT 이면 OrderPrice 가
// 필수이고, MARKET 이면 OrderPrice 를 보내면 안 된다(openapi ConditionRequest.orderPrice).
func (c Condition) validate(field string, ot OrderType) error {
	if c.OrderSide == "" {
		return fmt.Errorf("toss: %s.orderSide is required", field)
	}
	if !c.TriggerPrice.IsPositive() {
		return fmt.Errorf("toss: %s.triggerPrice must be positive (got %s)", field, c.TriggerPrice)
	}
	switch ot {
	case OrderTypeLimit:
		if !c.OrderPrice.IsPositive() {
			return fmt.Errorf("toss: %s.orderPrice is required for LIMIT conditional orders", field)
		}
	case OrderTypeMarket:
		if !c.OrderPrice.IsZero() {
			return fmt.Errorf("toss: %s.orderPrice must not be set for MARKET conditional orders (got %s)", field, c.OrderPrice)
		}
	}
	return nil
}

func (c Condition) body() conditionBody {
	b := conditionBody{OrderSide: c.OrderSide, TriggerPrice: c.TriggerPrice.String()}
	if c.OrderPrice.IsPositive() {
		b.OrderPrice = c.OrderPrice.String()
	}
	return b
}

// PlaceRequest 는 조건주문 생성 요청.
type PlaceRequest struct {
	Symbol     string          // 필수
	Type       Type            // 필수. SINGLE/OCO/OTO
	Quantity   decimal.Decimal // 필수. 그룹 공통값 — OCO/OTO 는 동일 포지션이라 두 조건이 같은 수량을 쓴다
	OrderType  OrderType       // 필수
	ExpireDate tosstypes.Date  // 필수. 이 날짜까지 감시
	First      Condition       // 필수
	Second     *Condition      // OCO/OTO 에서 사용
	// ClientOrderID 는 멱등성 키다. 같은 값으로 재요청하면 서버가 중복 생성을 막고(10분 유효), 설정하면
	// SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다. 같은 키로 내용이 다른 요청을
	// 보내면 400 idempotency-key-conflict.
	ClientOrderID string
	// ConfirmHighValue 는 1억원 이상 주문 동의 여부. true 가 아니면 400 confirm-high-value-required.
	ConfirmHighValue bool
}

type placeBody struct {
	Symbol                string         `json:"symbol"`
	Type                  Type           `json:"type"`
	Quantity              string         `json:"quantity"`
	OrderType             OrderType      `json:"orderType"`
	ExpireDate            string         `json:"expireDate"`
	First                 conditionBody  `json:"first"`
	Second                *conditionBody `json:"second,omitempty"`
	ClientOrderID         string         `json:"clientOrderId,omitempty"`
	ConfirmHighValueOrder bool           `json:"confirmHighValueOrder,omitempty"`
}

// ModifyRequest 는 조건주문 수정 요청.
//
// **수정은 부분 변경이 아니라 전체 재설정이다.** 서버는 기존 조건주문을 취소하고 새로 생성하므로,
// 유지하려는 조건은 모두 다시 보내야 한다. 예를 들어 OCO 의 만료일만 바꾸려고 Type: TypeSingle 과
// First 하나만 보내면 나머지 한쪽 조건(예: 손절 다리)이 사라진다.
//
// 수정 후에는 **새 conditionalOrderId 가 발급되고 기존 ID 는 무효화**된다.
// 이후 조회·수정·취소에는 반환된 ConditionalOrderID 를 써야 한다.
// 생성과 달리 ExpireDate 가 필수이며, 종목은 conditionalOrderId 로 식별되므로 Symbol 은 없다.
type ModifyRequest struct {
	Type       Type
	Quantity   decimal.Decimal // 필수. 그룹 공통값 — OCO/OTO 는 동일 포지션이라 두 조건이 같은 수량을 쓴다
	OrderType  OrderType
	ExpireDate tosstypes.Date
	First      Condition
	Second     *Condition
	// ConfirmHighValue 는 1억원 이상 주문 동의 여부. true 가 아니면 400 confirm-high-value-required.
	ConfirmHighValue bool
}

type modifyBody struct {
	Type                  Type           `json:"type"`
	Quantity              string         `json:"quantity"`
	OrderType             OrderType      `json:"orderType"`
	ExpireDate            string         `json:"expireDate"`
	First                 conditionBody  `json:"first"`
	Second                *conditionBody `json:"second,omitempty"`
	ConfirmHighValueOrder bool           `json:"confirmHighValueOrder,omitempty"`
}

// validateCommon 은 Place/Modify 가 공유하는 그룹 필드 검증이다. symbol 은 Place 에서만 있으므로
// 여기서 다루지 않는다(호출부가 별도로 params.Symbol 을 검증한다).
//
// type↔second 조합(openapi ConditionalOrderCreateRequest.second/orderType 설명): SINGLE 은 second 를
// 보내면 안 되고, OCO/OTO 는 second 가 필수이며 지정가(LIMIT)만 지원한다.
// OCO 는 양쪽 다 매도이며 first 감시가 > second 감시가 여야 하고, OTO 는 first 매수·second 매도다.
func validateCommon(typ Type, qty decimal.Decimal, ot OrderType, expire tosstypes.Date, first Condition, second *Condition) error {
	if typ == "" {
		return errors.New("toss: type is required")
	}
	if !qty.IsPositive() {
		return fmt.Errorf("toss: quantity must be positive (got %s)", qty)
	}
	if ot == "" {
		return errors.New("toss: orderType is required")
	}
	if expire.IsZero() {
		return errors.New("toss: expireDate is required")
	}
	switch typ {
	case TypeSingle:
		if second != nil {
			return errors.New("toss: second must not be set for SINGLE conditional orders")
		}
	case TypeOCO:
		if second == nil {
			return errors.New("toss: second is required for OCO conditional orders")
		}
		if ot != OrderTypeLimit {
			return fmt.Errorf("toss: OCO conditional orders support LIMIT only (got %s)", ot)
		}
		// openapi: "first/second 모두 매도(SELL)이며 first 감시가 > 현재가 > second 감시가 여야 합니다."
		// 현재가 비교는 상태 의존이라 서버 몫이고, 방향·순서만 사전에 막는다.
		if first.OrderSide != SideSell || second.OrderSide != SideSell {
			return fmt.Errorf("toss: OCO requires both conditions to be SELL (got first=%s second=%s)", first.OrderSide, second.OrderSide)
		}
		if !first.TriggerPrice.GreaterThan(second.TriggerPrice) {
			return fmt.Errorf("toss: OCO requires first.triggerPrice > second.triggerPrice (got %s, %s)", first.TriggerPrice, second.TriggerPrice)
		}
	case TypeOTO:
		if second == nil {
			return errors.New("toss: second is required for OTO conditional orders")
		}
		if ot != OrderTypeLimit {
			return fmt.Errorf("toss: OTO conditional orders support LIMIT only (got %s)", ot)
		}
		// openapi: "first 는 매수(BUY), second 는 매도(SELL)입니다."
		if first.OrderSide != SideBuy || second.OrderSide != SideSell {
			return fmt.Errorf("toss: OTO requires first=BUY and second=SELL (got first=%s second=%s)", first.OrderSide, second.OrderSide)
		}
	}
	if err := first.validate("first", ot); err != nil {
		return err
	}
	if second != nil {
		if err := second.validate("second", ot); err != nil {
			return err
		}
	}
	return nil
}

// Place 는 조건주문을 생성한다(POST /api/v1/conditional-orders).
//
// 대표 에러: invalid-request(orderSide·triggerPrice·orderPrice 형식 오류, 호가단위 불일치 등),
// stock-not-found.
func (c *Client) Place(ctx context.Context, r PlaceRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if err := validateCommon(r.Type, r.Quantity, r.OrderType, r.ExpireDate, r.First, r.Second); err != nil {
		return nil, err
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Type: r.Type, Quantity: r.Quantity.String(), OrderType: r.OrderType,
		ExpireDate: r.ExpireDate.String(), First: r.First.body(),
		ClientOrderID: r.ClientOrderID, ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if r.Second != nil {
		sb := r.Second.body()
		body.Second = &sb
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/conditional-orders", body, c.accountSeq, r.ClientOrderID)
}

// Modify 는 조건주문을 수정한다(POST /api/v1/conditional-orders/{id}/modify).
// 전체 재설정이며 성공 시 새 conditionalOrderId 를 돌려준다(기존 ID 무효).
// 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: invalid-request(호가단위 불일치 등), conditional-order-not-found.
func (c *Client) Modify(ctx context.Context, id string, r ModifyRequest) (*Result, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return nil, err
	}
	if err := validateCommon(r.Type, r.Quantity, r.OrderType, r.ExpireDate, r.First, r.Second); err != nil {
		return nil, err
	}
	body := modifyBody{
		Type: r.Type, Quantity: r.Quantity.String(), OrderType: r.OrderType,
		ExpireDate: r.ExpireDate.String(), First: r.First.body(), ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if r.Second != nil {
		sb := r.Second.body()
		body.Second = &sb
	}
	return fetch.PostOne[Result](ctx, c.http, "/api/v1/conditional-orders/"+url.PathEscape(id)+"/modify", body, c.accountSeq, "")
}

// Cancel 은 조건주문을 취소한다(DELETE /api/v1/conditional-orders/{id}). 성공 시 본문이 없다(204).
// 취소는 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: conditional-order-not-found.
func (c *Client) Cancel(ctx context.Context, id string) error {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return err
	}
	return fetch.Send(ctx, c.http, http.MethodDelete, "/api/v1/conditional-orders/"+url.PathEscape(id), nil, nil, c.accountSeq)
}
