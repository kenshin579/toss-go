package indicators

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// Price 는 시장 지표(지수) 현재가.
type Price struct {
	Symbol    string          `json:"symbol"`
	Timestamp *time.Time      `json:"timestamp"` // 없으면 nil
	LastPrice decimal.Decimal `json:"lastPrice"` // 지수 포인트 또는 국채 수익률(%). 통화 없음
}

// Prices 는 시장 지표 현재가를 조회한다(GET /api/v1/market-indicators/prices). 최대 200개. 심볼 카탈로그는 패키지 주석 참고(KOSPI, KOSDAQ, KR_BOND_*).
// 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) Prices(ctx context.Context, symbols ...string) ([]Price, error) {
	joined, err := params.IndicatorSymbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Price](ctx, c.http, "/api/v1/market-indicators/prices", url.Values{"symbols": {joined}}, 0)
}
