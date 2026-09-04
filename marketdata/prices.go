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

// Price 는 종목 현재가.
type Price struct {
	Symbol    string             `json:"symbol"`
	Timestamp *time.Time         `json:"timestamp"` // 체결 시각. 시세가 없으면 null
	LastPrice decimal.Decimal    `json:"lastPrice"`
	Currency  tosstypes.Currency `json:"currency"`
}

// Prices 는 여러 종목의 현재가를 조회한다(GET /api/v1/prices). 최대 200개. 없는 심볼은 결과에서 빠진다.
func (c *Client) Prices(ctx context.Context, symbols ...string) ([]Price, error) {
	joined, err := params.Symbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Price](ctx, c.http, "/api/v1/prices", url.Values{"symbols": {joined}}, 0)
}
