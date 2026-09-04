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

// maxClientOrderIDLen 검증은 루트 toss 패키지와 중복을 피하려고(순환 임포트 방지) 여기서 최소 규칙만 확인한다.
const maxClientOrderIDLen = 36

func validateKey(id string) error {
	if id == "" {
		return nil // 멱등성 미적용
	}
	if len(id) > maxClientOrderIDLen {
		return errors.New("toss: clientOrderId too long (max 36)")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return errors.New("toss: invalid clientOrderId (allowed: A-Z a-z 0-9 - _)")
		}
	}
	return nil
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
	Symbol           string          // 필수
	Type             Type            // 필수. SINGLE/OCO/OTO
	Quantity         decimal.Decimal // 필수
	OrderType        OrderType       // 필수
	ExpireDate       tosstypes.Date  // 필수. 이 날짜까지 감시
	First            Condition       // 필수
	Second           *Condition      // OCO/OTO 에서 사용
	ClientOrderID    string          // 멱등성 키. 설정하면 401 토큰 오류에 1회 재시도한다
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

// ModifyRequest 는 조건주문 수정 요청. 생성과 같은 필드를 쓰되 clientOrderId 는 받지 않는다.
// 수정은 기존 조건주문을 취소하고 새로 생성하는 방식으로 동작해 응답에 새 conditionalOrderId 가 온다.
type ModifyRequest struct {
	Type             Type
	Quantity         decimal.Decimal
	OrderType        OrderType
	ExpireDate       tosstypes.Date
	First            Condition
	Second           *Condition
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
	case TypeOCO, TypeOTO:
		if second == nil {
			return fmt.Errorf("toss: second is required for %s conditional orders", typ)
		}
		if ot != OrderTypeLimit {
			return fmt.Errorf("toss: %s conditional orders support LIMIT only (got %s)", typ, ot)
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
