package marketinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// ExchangeRate 는 환율 고시.
type ExchangeRate struct {
	BaseCurrency   tosstypes.Currency       `json:"baseCurrency"`
	QuoteCurrency  tosstypes.Currency       `json:"quoteCurrency"`
	Rate           decimal.Decimal          `json:"rate"`
	MidRate        decimal.Decimal          `json:"midRate"`    // 매매기준율
	BasisPoint     decimal.Decimal          `json:"basisPoint"` // (rate - midRate) / midRate * 10000
	RateChangeType tosstypes.RateChangeType `json:"rateChangeType"`
	ValidFrom      time.Time                `json:"validFrom"`
	ValidUntil     time.Time                `json:"validUntil"`
}

// ExchangeRate 는 환율을 조회한다(GET /api/v1/exchange-rate). at 이 nil 이면 현재 고시. 해당 통화쌍/시각의 고시가 없으면 404 exchange-rate-not-found.
func (c *Client) ExchangeRate(ctx context.Context, base, quote tosstypes.Currency, at *time.Time) (*ExchangeRate, error) {
	if err := params.Require("baseCurrency", string(base)); err != nil {
		return nil, err
	}
	if err := params.Require("quoteCurrency", string(quote)); err != nil {
		return nil, err
	}
	q := url.Values{"baseCurrency": {string(base)}, "quoteCurrency": {string(quote)}}
	params.Time(q, "dateTime", at)
	return fetch.One[ExchangeRate](ctx, c.http, "/api/v1/exchange-rate", q)
}
