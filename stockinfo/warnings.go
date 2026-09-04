package stockinfo

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Warning 은 매수 유의사항 1건.
type Warning struct {
	WarningType tosstypes.WarningType `json:"warningType"`
	Exchange    *string               `json:"exchange"`
	StartDate   *tosstypes.Date       `json:"startDate"`
	EndDate     *tosstypes.Date       `json:"endDate"`
}

// Warnings 는 종목의 매수 유의사항을 조회한다(GET /api/v1/stocks/{symbol}/warnings). 없으면 빈 슬라이스.
func (c *Client) Warnings(ctx context.Context, symbol string) ([]Warning, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.List[Warning](ctx, c.http, "/api/v1/stocks/"+url.PathEscape(symbol)+"/warnings", nil)
}
