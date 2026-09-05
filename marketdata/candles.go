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

// Candle 은 OHLCV 봉 1개.
type Candle struct {
	Timestamp  time.Time          `json:"timestamp"` // 봉 시작 시각
	OpenPrice  decimal.Decimal    `json:"openPrice"`
	HighPrice  decimal.Decimal    `json:"highPrice"`
	LowPrice   decimal.Decimal    `json:"lowPrice"`
	ClosePrice decimal.Decimal    `json:"closePrice"`
	Volume     decimal.Decimal    `json:"volume"`
	Currency   tosstypes.Currency `json:"currency"`
}

// CandlePage 는 캔들 한 페이지. NextBefore 를 다음 요청의 Before 로 넘기면 이어서 조회한다.
type CandlePage struct {
	Candles    []Candle   `json:"candles"`
	NextBefore *time.Time `json:"nextBefore"` // 더 없으면 nil
}

// CandlesParams 는 Candles 인자.
type CandlesParams struct {
	Symbol   string             // 필수
	Interval tosstypes.Interval // 필수 (1m, 1d)
	Count    int                // 최대 200, 0 이면 서버 기본값(100)
	Before   *time.Time         // 이 시각 이하(inclusive)의 봉만. nil 이면 최신부터
	Adjusted *bool              // 수정주가 적용 여부. nil 이면 서버 기본값(true)
}

// Candles 는 캔들 차트를 조회한다(GET /api/v1/candles).
func (c *Client) Candles(ctx context.Context, p CandlesParams) (*CandlePage, error) {
	if err := params.Symbol(p.Symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"symbol": {p.Symbol}, "interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Time(q, "before", p.Before)
	params.Bool(q, "adjusted", p.Adjusted)
	return fetch.One[CandlePage](ctx, c.http, "/api/v1/candles", q, 0)
}
