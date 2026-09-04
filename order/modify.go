package order

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// validateKey 는 internal/params.ClientOrderIDFormat 에 위임한다(빈 값만 여기서 "멱등성 미적용"으로 통과시킨다).
func validateKey(id string) error {
	if id == "" {
		return nil // 멱등성 미적용
	}
	return params.ClientOrderIDFormat(id)
}

// ModifyRequest 는 주문 정정 요청.
// Quantity/Price 는 nil 이면 전송하지 않는다 — 값 0 과 "미전송" 을 구분하기 위해 포인터다.
// 국내 주식은 Quantity 가 필수(양의 정수)이고, 미국 주식은 Quantity 를 보낼 수 없다
// (보내면 400 us-modify-quantity-not-supported) — 가격만 정정할 수 있다. SDK 는 시장을 알 수 없으므로
// 이 규칙은 서버가 판단한다.
type ModifyRequest struct {
	OrderType        Type             // 필수
	Quantity         *decimal.Decimal // 국내 필수(양의 정수). 미국은 nil
	Price            *decimal.Decimal // LIMIT 필수, MARKET 은 nil 이어야 한다
	ConfirmHighValue bool
}

type modifyBody struct {
	OrderType             Type   `json:"orderType"`
	Quantity              string `json:"quantity,omitempty"`
	Price                 string `json:"price,omitempty"`
	ConfirmHighValueOrder bool   `json:"confirmHighValueOrder,omitempty"`
}

// Modify 는 주문의 가격 또는 수량을 정정한다(POST /api/v1/orders/{orderId}/modify).
// 정정은 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: already-filled, already-canceled, already-modified, already-processing,
// order-not-found, modify-restricted, order-hours-closed, us-modify-quantity-not-supported.
func (c *Client) Modify(ctx context.Context, orderID string, r ModifyRequest) (*OperationResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	if r.OrderType == "" {
		return nil, errors.New("toss: orderType is required")
	}
	if r.Quantity != nil {
		if !r.Quantity.IsPositive() {
			return nil, fmt.Errorf("toss: quantity must be positive (got %s)", r.Quantity)
		}
		if !r.Quantity.IsInteger() {
			return nil, fmt.Errorf("toss: quantity must be an integer (got %s)", r.Quantity)
		}
	}
	switch r.OrderType {
	case TypeLimit:
		if r.Price == nil || !r.Price.IsPositive() {
			return nil, errors.New("toss: price is required for LIMIT orders")
		}
	case TypeMarket:
		if r.Price != nil {
			return nil, fmt.Errorf("toss: price must not be set for MARKET orders (got %s)", r.Price)
		}
	}
	body := modifyBody{OrderType: r.OrderType, ConfirmHighValueOrder: r.ConfirmHighValue}
	if r.Quantity != nil {
		body.Quantity = r.Quantity.String()
	}
	if r.Price != nil {
		body.Price = r.Price.String()
	}
	return fetch.PostOne[OperationResult](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID)+"/modify", body, c.accountSeq, "")
}

// Cancel 은 주문을 취소한다(POST /api/v1/orders/{orderId}/cancel). 이미 체결된 주문은 취소할 수 없다.
// 취소는 멱등성 키를 받지 않으므로 401 재시도를 하지 않는다.
//
// 대표 에러: already-filled, already-canceled, already-processing, order-not-found,
// cancel-restricted, order-hours-closed.
func (c *Client) Cancel(ctx context.Context, orderID string) (*OperationResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	return fetch.PostOne[OperationResult](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID)+"/cancel", struct{}{}, c.accountSeq, "")
}
