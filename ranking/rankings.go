package ranking

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// RankingPrice 는 랭킹 항목의 가격 정보.
type RankingPrice struct {
	LastPrice  decimal.Decimal  `json:"lastPrice"`
	BasePrice  decimal.Decimal  `json:"basePrice"`  // 기준가(전일 종가)
	ChangeRate *decimal.Decimal `json:"changeRate"` // 등락률(소수). 없으면 nil
}

// RankingItem 은 랭킹 1건.
type RankingItem struct {
	Rank          int                `json:"rank"`
	Symbol        string             `json:"symbol"`
	Currency      tosstypes.Currency `json:"currency"`
	Price         RankingPrice       `json:"price"`
	TradingVolume decimal.Decimal    `json:"tradingVolume"`
	TradingAmount decimal.Decimal    `json:"tradingAmount"`
}

// Rankings 는 랭킹 결과.
type Rankings struct {
	RankedAt *time.Time    `json:"rankedAt"`
	Rankings []RankingItem `json:"rankings"`
}

// RankingsParams 는 Rankings 인자. Type/MarketCountry/Duration 필수.
type RankingsParams struct {
	Type                     tosstypes.RankingType
	MarketCountry            tosstypes.MarketCountry
	Duration                 tosstypes.RankingDuration // TOP_GAINERS/TOP_LOSERS 는 realtime 미지원
	ExcludeInvestmentCaution *bool                     // 투자주의 종목 제외. nil 이면 서버 기본값
	Count                    int                       // 최대 100, 0 이면 서버 기본값(100)
}

// Rankings 는 주식 랭킹을 조회한다(GET /api/v1/rankings).
func (c *Client) Rankings(ctx context.Context, p RankingsParams) (*Rankings, error) {
	if err := params.Require("type", string(p.Type)); err != nil {
		return nil, err
	}
	if err := params.Require("marketCountry", string(p.MarketCountry)); err != nil {
		return nil, err
	}
	if err := params.Require("duration", string(p.Duration)); err != nil {
		return nil, err
	}
	q := url.Values{"type": {string(p.Type)}, "marketCountry": {string(p.MarketCountry)}, "duration": {string(p.Duration)}}
	params.Bool(q, "excludeInvestmentCaution", p.ExcludeInvestmentCaution)
	params.Int(q, "count", p.Count)
	return fetch.One[Rankings](ctx, c.http, "/api/v1/rankings", q)
}
