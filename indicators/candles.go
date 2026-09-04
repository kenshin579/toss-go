package indicators

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Candle 은 시장 지표 OHLCV 봉 1개(지수 포인트 또는 국채 수익률 %).
type Candle struct {
	Timestamp  time.Time       `json:"timestamp"` // 봉 시작 시각
	OpenPrice  decimal.Decimal `json:"openPrice"`
	HighPrice  decimal.Decimal `json:"highPrice"`
	LowPrice   decimal.Decimal `json:"lowPrice"`
	ClosePrice decimal.Decimal `json:"closePrice"`
	Volume     decimal.Decimal `json:"volume"`
}

// CandlePage 는 캔들 한 페이지. NextBefore 를 다음 요청의 Before 로 넘기면 이어서 조회한다.
type CandlePage struct {
	Candles    []Candle   `json:"candles"`
	NextBefore *time.Time `json:"nextBefore"` // 더 없으면 nil
}

// CandlesParams 는 Candles 인자.
type CandlesParams struct {
	Interval tosstypes.Interval // 필수. 1m 은 KOSPI/KOSDAQ 만, KR_BOND_* 는 1d 만(그 외 400 invalid-request)
	Count    int                // 최대 200, 0 이면 서버 기본값(100)
	Before   *time.Time         // 이 시각 이하의 봉만. nil 이면 최신부터
}

// Candles 는 시장 지표 캔들을 조회한다(GET /api/v1/market-indicators/{symbol}/candles).
// 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) Candles(ctx context.Context, symbol string, p CandlesParams) (*CandlePage, error) {
	if err := params.IndicatorSymbol(symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Time(q, "before", p.Before)
	return fetch.One[CandlePage](ctx, c.http, "/api/v1/market-indicators/"+url.PathEscape(symbol)+"/candles", q, 0)
}
