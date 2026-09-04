package order

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// PlaceRequest 는 수량 기준 주문 생성 요청.
type PlaceRequest struct {
	Symbol           string          // 필수
	Side             Side            // 필수
	OrderType        Type            // 필수
	Quantity         decimal.Decimal // 필수. 소수점 수량은 미국 주식 시장가 매도 전용이며 정규장 종료 1시간 전까지만 접수된다
	Price            decimal.Decimal // LIMIT 이면 필수. MARKET 이면 반드시 zero(전달하면 400 invalid-request)
	TimeInForce      TimeInForce     // 비우면 서버 기본값 DAY
	ClientOrderID    string          // 멱등성 키(10분). 설정하면 401 토큰 오류에 1회 재시도한다
	ConfirmHighValue bool            // 1억원 이상 주문에 true 가 아니면 400 confirm-high-value-required
}

// AmountRequest 는 금액 기준 주문 생성 요청 — 미국 주식 시장가 전용.
// 체결 수량은 시장가에 따라 정해지며, 정규장 시작~정규장 종료 1시간 전에만 접수된다.
type AmountRequest struct {
	Symbol           string
	Side             Side
	OrderAmount      decimal.Decimal // 필수(USD)
	ClientOrderID    string
	ConfirmHighValue bool
}

type placeBody struct {
	Symbol                string `json:"symbol"`
	Side                  Side   `json:"side"`
	OrderType             Type   `json:"orderType"`
	Quantity              string `json:"quantity,omitempty"`
	OrderAmount           string `json:"orderAmount,omitempty"`
	Price                 string `json:"price,omitempty"`
	TimeInForce           string `json:"timeInForce,omitempty"`
	ClientOrderID         string `json:"clientOrderId,omitempty"`
	ConfirmHighValueOrder bool   `json:"confirmHighValueOrder,omitempty"`
}

// Place 는 수량 기준 주문을 생성한다(POST /api/v1/orders).
//
// 대표 에러: insufficient-buying-power, order-hours-closed, invalid-request, price-out-of-range,
// stock-restricted, confirm-high-value-required, request-in-progress, idempotency-key-conflict.
func (c *Client) Place(ctx context.Context, r PlaceRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if r.Side == "" {
		return nil, errors.New("toss: side is required")
	}
	if r.OrderType == "" {
		return nil, errors.New("toss: orderType is required")
	}
	if !r.Quantity.IsPositive() {
		return nil, fmt.Errorf("toss: quantity must be positive (got %s)", r.Quantity)
	}
	if r.OrderType == TypeLimit && !r.Price.IsPositive() {
		return nil, fmt.Errorf("toss: price is required for LIMIT orders (got %s)", r.Price)
	}
	if r.OrderType == TypeMarket && !r.Price.IsZero() {
		return nil, fmt.Errorf("toss: price must not be set for MARKET orders (got %s)", r.Price)
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Side: r.Side, OrderType: r.OrderType,
		Quantity: r.Quantity.String(), TimeInForce: string(r.TimeInForce),
		ClientOrderID: r.ClientOrderID, ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	if !r.Price.IsZero() {
		body.Price = r.Price.String()
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/orders", body, c.accountSeq, r.ClientOrderID)
}

// PlaceAmount 는 금액 기준 주문을 생성한다(POST /api/v1/orders). 미국 주식 시장가 전용이며
// orderType 은 SDK 가 MARKET 으로 채운다.
//
// 대표 에러: invalid-request(미국 시장가 외 요청 등), amount-order-outside-regular-hours,
// insufficient-buying-power, max-order-amount-exceeded, confirm-high-value-required,
// idempotency-key-conflict.
func (c *Client) PlaceAmount(ctx context.Context, r AmountRequest) (*PlaceResult, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(r.Symbol); err != nil {
		return nil, err
	}
	if r.Side == "" {
		return nil, errors.New("toss: side is required")
	}
	if !r.OrderAmount.IsPositive() {
		return nil, fmt.Errorf("toss: orderAmount must be positive (got %s)", r.OrderAmount)
	}
	if err := validateKey(r.ClientOrderID); err != nil {
		return nil, err
	}
	body := placeBody{
		Symbol: r.Symbol, Side: r.Side, OrderType: TypeMarket,
		OrderAmount:           r.OrderAmount.String(),
		ClientOrderID:         r.ClientOrderID,
		ConfirmHighValueOrder: r.ConfirmHighValue,
	}
	return fetch.PostOne[PlaceResult](ctx, c.http, "/api/v1/orders", body, c.accountSeq, r.ClientOrderID)
}
