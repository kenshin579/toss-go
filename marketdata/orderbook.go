package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// OrderbookEntry 는 호가 한 단계(가격·잔량).
type OrderbookEntry struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

// Orderbook 은 매도/매수 호가.
type Orderbook struct {
	Timestamp *time.Time         `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
	Asks      []OrderbookEntry   `json:"asks"` // 매도 호가(낮은 가격부터)
	Bids      []OrderbookEntry   `json:"bids"` // 매수 호가(높은 가격부터)
}

// Orderbook 은 호가를 조회한다(GET /api/v1/orderbook).
func (c *Client) Orderbook(ctx context.Context, symbol string) (*Orderbook, error) {
	if err := params.Require("symbol", symbol); err != nil {
		return nil, err
	}
	var out Orderbook
	if err := c.http.Get(ctx, "/api/v1/orderbook", url.Values{"symbol": {symbol}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
