package order

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// BuyingPower 는 매수 가능 금액.
type BuyingPower struct {
	Currency        tosstypes.Currency `json:"currency"`
	CashBuyingPower decimal.Decimal    `json:"cashBuyingPower"` // 현금 매수 가능 금액
}

// SellableQuantity 는 판매 가능 수량.
type SellableQuantity struct {
	SellableQuantity decimal.Decimal `json:"sellableQuantity"`
}

// Commission 은 시장별 매매 수수료율. StartDate/EndDate 는 기간 한정 요율일 때만 설정된다.
type Commission struct {
	MarketCountry  tosstypes.MarketCountry `json:"marketCountry"`
	CommissionRate decimal.Decimal         `json:"commissionRate"` // 소수 비율(0.00015 = 0.015%)
	StartDate      *tosstypes.Date         `json:"startDate"`
	EndDate        *tosstypes.Date         `json:"endDate"`
}

// BuyingPower 는 매수 가능 금액을 조회한다(GET /api/v1/buying-power).
// 대표 에러: invalid-request(지원하지 않는 통화), account-not-found.
func (c *Client) BuyingPower(ctx context.Context, currency tosstypes.Currency) (*BuyingPower, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("currency", string(currency)); err != nil {
		return nil, err
	}
	return fetch.One[BuyingPower](ctx, c.http, "/api/v1/buying-power", url.Values{"currency": {string(currency)}}, c.accountSeq)
}

// SellableQuantity 는 판매 가능 수량을 조회한다(GET /api/v1/sellable-quantity).
func (c *Client) SellableQuantity(ctx context.Context, symbol string) (*SellableQuantity, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.One[SellableQuantity](ctx, c.http, "/api/v1/sellable-quantity", url.Values{"symbol": {symbol}}, c.accountSeq)
}

// Commissions 는 계좌의 매매 수수료율을 조회한다(GET /api/v1/commissions).
func (c *Client) Commissions(ctx context.Context) ([]Commission, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	return fetch.List[Commission](ctx, c.http, "/api/v1/commissions", nil, c.accountSeq)
}
