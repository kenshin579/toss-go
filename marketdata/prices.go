package marketdata

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Price 는 종목 현재가.
type Price struct {
	Symbol    string             `json:"symbol"`
	Timestamp *time.Time         `json:"timestamp"` // 체결 시각. 시세가 없으면 null
	LastPrice decimal.Decimal    `json:"lastPrice"`
	Currency  tosstypes.Currency `json:"currency"`
}

// Prices 는 여러 종목의 현재가를 조회한다(GET /api/v1/prices). 없는 심볼은 결과에서 빠진다.
func (c *Client) Prices(ctx context.Context, symbols ...string) ([]Price, error) {
	if len(symbols) == 0 {
		return nil, errors.New("toss: symbols must not be empty")
	}
	var out []Price
	err := c.http.Get(ctx, "/api/v1/prices", url.Values{"symbols": {strings.Join(symbols, ",")}}, &out)
	return out, err
}
