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

// TradingAmount 는 매수/매도 대금.
type TradingAmount struct {
	BuyAmount  decimal.Decimal `json:"buyAmount"`
	SellAmount decimal.Decimal `json:"sellAmount"`
}

// InstitutionBreakdown 은 기관 세부 7개 분류.
type InstitutionBreakdown struct {
	FinancialInvestment       TradingAmount `json:"financialInvestment"`
	Insurance                 TradingAmount `json:"insurance"`
	Trust                     TradingAmount `json:"trust"`
	PrivateEquityFund         TradingAmount `json:"privateEquityFund"`
	Bank                      TradingAmount `json:"bank"`
	OtherFinancialInstitution TradingAmount `json:"otherFinancialInstitution"`
	PensionFund               TradingAmount `json:"pensionFund"`
}

// InstitutionTradingAmount 는 기관 합계 + 세부 분류.
type InstitutionTradingAmount struct {
	TradingAmount
	Breakdown InstitutionBreakdown `json:"breakdown"`
}

// InvestorTradingRecord 는 투자자별 매매대금 1구간.
type InvestorTradingRecord struct {
	Date             tosstypes.Date           `json:"date"`
	UpdatedAt        time.Time                `json:"updatedAt"`
	Individual       TradingAmount            `json:"individual"`
	Foreigner        TradingAmount            `json:"foreigner"`
	Institution      InstitutionTradingAmount `json:"institution"`
	OtherCorporation TradingAmount            `json:"otherCorporation"`
}

// InvestorTradingPage 는 매매대금 한 페이지. NextUntil 을 다음 요청의 Until 로 넘기면 이어서 조회한다.
type InvestorTradingPage struct {
	Records   []InvestorTradingRecord `json:"records"`
	NextUntil *tosstypes.Date         `json:"nextUntil"`
}

// InvestorTradingParams 는 InvestorTrading 인자.
type InvestorTradingParams struct {
	Interval tosstypes.IndicatorInterval // 필수 (1d, 1w, 1mo, 1y)
	Count    int                         // 최대 100, 0 이면 서버 기본값(10)
	Until    tosstypes.Date              // 이 날짜(KST) 이하의 기록만. 비우면 최신부터
}

// InvestorTrading 은 투자자별 매매대금을 조회한다(GET /api/v1/market-indicators/{symbol}/investor-trading). Until 은 KST 기준 날짜.
// KOSPI, KOSDAQ 만 지원한다. 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) InvestorTrading(ctx context.Context, symbol string, p InvestorTradingParams) (*InvestorTradingPage, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Date(q, "until", p.Until)
	return fetch.One[InvestorTradingPage](ctx, c.http, "/api/v1/market-indicators/"+url.PathEscape(symbol)+"/investor-trading", q)
}
