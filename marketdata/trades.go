package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Trade 는 체결 1건.
type Trade struct {
	Price     decimal.Decimal    `json:"price"`
	Volume    decimal.Decimal    `json:"volume"`
	Timestamp time.Time          `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
}

// Trades 는 최근 체결 내역을 조회한다(GET /api/v1/trades). count 는 최대 50, 0 이면 서버 기본값(50).
func (c *Client) Trades(ctx context.Context, symbol string, count int) ([]Trade, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	q := url.Values{"symbol": {symbol}}
	params.Int(q, "count", count)
	return fetch.List[Trade](ctx, c.http, "/api/v1/trades", q)
}
