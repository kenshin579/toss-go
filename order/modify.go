package order

import (
	"context"
	"errors"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// clientOrderIDPattern 검증은 루트 toss 패키지와 중복을 피하려고 여기서 최소 규칙만 확인한다.
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

// ModifyRequest 는 주문 정정 요청.
// 국내 주식은 Quantity 가 필수(양의 정수)이고, 미국 주식은 Quantity 를 보낼 수 없다
// (보내면 400 us-modify-quantity-not-supported) — 가격만 정정할 수 있다.
type ModifyRequest struct {
	OrderType        Type            // 필수
	Quantity         decimal.Decimal // 국내 필수, 미국은 0(미전송)
	Price            decimal.Decimal // 0 이면 미전송
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
	body := modifyBody{OrderType: r.OrderType, ConfirmHighValueOrder: r.ConfirmHighValue}
	if r.Quantity.IsPositive() {
		body.Quantity = r.Quantity.String()
	}
	if r.Price.IsPositive() {
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
