package asset

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Amount 는 원화·달러 동시 표기 금액. USD 는 국내 전용 계좌 등에서 nil 일 수 있다.
type Amount struct {
	KRW decimal.Decimal  `json:"krw"`
	USD *decimal.Decimal `json:"usd"`
}

// OverviewMarketValue 는 계좌 전체 평가금액.
type OverviewMarketValue struct {
	Amount          Amount `json:"amount"`          // 평가금액
	AmountAfterCost Amount `json:"amountAfterCost"` // 비용(수수료·세금) 차감 후 평가금액
}

// OverviewProfitLoss 는 계좌 전체 손익.
type OverviewProfitLoss struct {
	Amount          Amount          `json:"amount"`
	AmountAfterCost Amount          `json:"amountAfterCost"`
	Rate            decimal.Decimal `json:"rate"`          // 수익률(소수 비율, 0.1179 = 11.79%)
	RateAfterCost   decimal.Decimal `json:"rateAfterCost"` // 비용 차감 후 수익률
}

// OverviewDailyProfitLoss 는 계좌 전체 일간 손익.
type OverviewDailyProfitLoss struct {
	Amount Amount          `json:"amount"`
	Rate   decimal.Decimal `json:"rate"`
}

// MarketValue 는 종목별 평가금액.
type MarketValue struct {
	PurchaseAmount  decimal.Decimal `json:"purchaseAmount"`  // 매입금액
	Amount          decimal.Decimal `json:"amount"`          // 평가금액
	AmountAfterCost decimal.Decimal `json:"amountAfterCost"` // 비용 차감 후 평가금액
}

// ProfitLoss 는 종목별 손익.
type ProfitLoss struct {
	Amount          decimal.Decimal `json:"amount"`
	AmountAfterCost decimal.Decimal `json:"amountAfterCost"`
	Rate            decimal.Decimal `json:"rate"`
	RateAfterCost   decimal.Decimal `json:"rateAfterCost"`
}

// DailyProfitLoss 는 종목별 일간 손익.
type DailyProfitLoss struct {
	Amount decimal.Decimal `json:"amount"`
	Rate   decimal.Decimal `json:"rate"`
}

// Cost 는 매도 시 예상 비용. Tax 는 미국 주식 등 해당 없으면 nil.
type Cost struct {
	Commission decimal.Decimal  `json:"commission"`
	Tax        *decimal.Decimal `json:"tax"`
}

// Item 은 보유 종목 1건.
type Item struct {
	Symbol               string                  `json:"symbol"`
	Name                 string                  `json:"name"`
	MarketCountry        tosstypes.MarketCountry `json:"marketCountry"`
	Currency             tosstypes.Currency      `json:"currency"`
	Quantity             decimal.Decimal         `json:"quantity"`
	LastPrice            decimal.Decimal         `json:"lastPrice"`
	AveragePurchasePrice decimal.Decimal         `json:"averagePurchasePrice"`
	MarketValue          MarketValue             `json:"marketValue"`
	ProfitLoss           ProfitLoss              `json:"profitLoss"`
	DailyProfitLoss      DailyProfitLoss         `json:"dailyProfitLoss"`
	Cost                 Cost                    `json:"cost"`
}

// Holdings 는 계좌 전체 요약과 보유 종목 목록.
type Holdings struct {
	TotalPurchaseAmount Amount                  `json:"totalPurchaseAmount"`
	MarketValue         OverviewMarketValue     `json:"marketValue"`
	ProfitLoss          OverviewProfitLoss      `json:"profitLoss"`
	DailyProfitLoss     OverviewDailyProfitLoss `json:"dailyProfitLoss"`
	Items               []Item                  `json:"items"`
}

// HoldingsParams 는 Holdings 의 선택 인자.
type HoldingsParams struct {
	Symbol string // 특정 종목만 조회. 비우면 전체
}

// Holdings 는 보유 주식을 조회한다(GET /api/v1/holdings). 인자의 zero value 면 전체를 조회한다.
// 보유하지 않은 종목을 지정하면 Items 가 빈 슬라이스로 돌아온다.
func (c *Client) Holdings(ctx context.Context, p HoldingsParams) (*Holdings, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	q := url.Values{}
	if p.Symbol != "" {
		if err := params.Symbol(p.Symbol); err != nil {
			return nil, err
		}
		q.Set("symbol", p.Symbol)
	}
	return fetch.One[Holdings](ctx, c.http, "/api/v1/holdings", q, c.accountSeq)
}
