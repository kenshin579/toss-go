package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// PriceLimits 는 당일 상한가·하한가. 해외 종목 등 제한이 없으면 nil.
type PriceLimits struct {
	Timestamp       time.Time          `json:"timestamp"`
	UpperLimitPrice *decimal.Decimal   `json:"upperLimitPrice"`
	LowerLimitPrice *decimal.Decimal   `json:"lowerLimitPrice"`
	Currency        tosstypes.Currency `json:"currency"`
}

// PriceLimits 는 상/하한가를 조회한다(GET /api/v1/price-limits).
func (c *Client) PriceLimits(ctx context.Context, symbol string) (*PriceLimits, error) {
	if err := params.Require("symbol", symbol); err != nil {
		return nil, err
	}
	var out PriceLimits
	if err := c.http.Get(ctx, "/api/v1/price-limits", url.Values{"symbol": {symbol}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
