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
	LastPrice  decimal.Decimal  `json:"lastPrice"`  // 현재가
	BasePrice  decimal.Decimal  `json:"basePrice"`  // 기준가. TOP_GAINERS/TOP_LOSERS 는 duration 시작 시점 가격, 그 외 유형은 전일 종가
	ChangeRate *decimal.Decimal `json:"changeRate"` // 등락률 = (lastPrice - basePrice) / basePrice, 소수 비율(0.0125 = 1.25%). basePrice 가 0 이면 nil. TOP_* 는 기간 등락률, 그 외는 전일 대비
}

// RankingItem 은 랭킹 1건.
type RankingItem struct {
	Rank          int                `json:"rank"`
	Symbol        string             `json:"symbol"`
	Currency      tosstypes.Currency `json:"currency"`
	Price         RankingPrice       `json:"price"`
	TradingVolume decimal.Decimal    `json:"tradingVolume"` // duration 누적. TOSS_SECURITIES_* 는 토스증권 체결 기준, 그 외는 시장 전체
	TradingAmount decimal.Decimal    `json:"tradingAmount"` // duration 누적. TOSS_SECURITIES_* 는 토스증권 체결 기준, 그 외는 시장 전체
}

// Rankings 는 랭킹 결과.
type Rankings struct {
	RankedAt *time.Time    `json:"rankedAt"` // 집계 시각. 집계되지 않은 조합이면 Rankings 가 비고 nil
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

// Rankings 는 주식 랭킹을 조회한다(GET /api/v1/rankings). TOP_GAINERS/TOP_LOSERS 에 duration=realtime 은 400 unsupported-ranking-duration. 집계되지 않은 조합은 빈 Rankings(RankedAt nil).
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
