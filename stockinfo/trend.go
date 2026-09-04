package stockinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// TrendParams 는 매매동향 5종의 공통 인자. 국내(KR) 종목만 지원한다.
type TrendParams struct {
	Count int            // 최대 100, 0 이면 서버 기본값(10)
	Until tosstypes.Date // 이 날짜 이하(inclusive)의 기록만. 비우면 최신부터
}

// TrendPage 는 매매동향 한 페이지. NextUntil 을 다음 요청의 Until 로 넘기면 이어서 조회한다.
type TrendPage[T any] struct {
	NextUntil *tosstypes.Date `json:"nextUntil"` // 더 없으면 nil
	Records   []T             `json:"records"`
}

// TradingVolume 은 매수/매도/순매수 거래량.
type TradingVolume struct {
	BuyVolume    decimal.Decimal `json:"buyVolume"`
	SellVolume   decimal.Decimal `json:"sellVolume"`
	NetBuyVolume decimal.Decimal `json:"netBuyVolume"`
}

// InstitutionBreakdown 은 기관 세부 7개 분류.
type InstitutionBreakdown struct {
	FinancialInvestment       TradingVolume `json:"financialInvestment"`
	Insurance                 TradingVolume `json:"insurance"`
	Trust                     TradingVolume `json:"trust"`
	PrivateEquityFund         TradingVolume `json:"privateEquityFund"`
	Bank                      TradingVolume `json:"bank"`
	OtherFinancialInstitution TradingVolume `json:"otherFinancialInstitution"`
	PensionFund               TradingVolume `json:"pensionFund"`
}

// InstitutionTradingVolume 은 기관 합계 + 세부 분류(잠정치에는 nil).
type InstitutionTradingVolume struct {
	TradingVolume
	Breakdown *InstitutionBreakdown `json:"breakdown"`
}

// ForeignerHolding 은 외국인 보유 현황.
type ForeignerHolding struct {
	HoldingQuantity decimal.Decimal `json:"holdingQuantity"`
	LimitQuantity   decimal.Decimal `json:"limitQuantity"`
	HoldingRate     decimal.Decimal `json:"holdingRate"`
}

// CFDBalance 는 CFD 잔고 현황(T+1 반영).
type CFDBalance struct {
	BuyBalanceQuantity  decimal.Decimal `json:"buyBalanceQuantity"`
	BuyBalanceRate      decimal.Decimal `json:"buyBalanceRate"`
	SellBalanceQuantity decimal.Decimal `json:"sellBalanceQuantity"`
	SellBalanceRate     decimal.Decimal `json:"sellBalanceRate"`
}

// InvestorTradingRecord 는 투자자별 매매동향 1일. 당일 잠정 기록에는 Individual/OtherCorporation/
// Institution.Breakdown 이 nil 이며 확정치가 반영되는 저녁부터 채워진다.
type InvestorTradingRecord struct {
	Date             tosstypes.Date           `json:"date"`
	UpdatedAt        time.Time                `json:"updatedAt"`
	Individual       *TradingVolume           `json:"individual"`
	Foreigner        TradingVolume            `json:"foreigner"`
	Institution      InstitutionTradingVolume `json:"institution"`
	OtherCorporation *TradingVolume           `json:"otherCorporation"`
	ForeignerHolding *ForeignerHolding        `json:"foreignerHolding"`
	CFD              *CFDBalance              `json:"cfd"`
}

// ProgramTradesRecord 는 프로그램매매 동향 1일.
type ProgramTradesRecord struct {
	Date         tosstypes.Date `json:"date"`
	Arbitrage    TradingVolume  `json:"arbitrage"`    // 차익거래
	NonArbitrage TradingVolume  `json:"nonArbitrage"` // 비차익거래
}

// ShortSellingRecord 는 공매도 동향 1일.
type ShortSellingRecord struct {
	Date                   tosstypes.Date   `json:"date"`
	UpdatedAt              time.Time        `json:"updatedAt"`
	ShortSellingVolume     decimal.Decimal  `json:"shortSellingVolume"`
	ShortSellingAmount     decimal.Decimal  `json:"shortSellingAmount"`
	ShortSellingVolumeRate *decimal.Decimal `json:"shortSellingVolumeRate"` // 거래량 대비 비율
	ShortSellingAmountRate *decimal.Decimal `json:"shortSellingAmountRate"` // 거래대금 대비 비율
}

// CreditTradeDetail 은 신용융자/신용대주 상세.
type CreditTradeDetail struct {
	NewQuantity     decimal.Decimal `json:"newQuantity"`
	ReturnQuantity  decimal.Decimal `json:"returnQuantity"`
	BalanceQuantity decimal.Decimal `json:"balanceQuantity"`
	BalanceRate     decimal.Decimal `json:"balanceRate"`
	TradingRate     decimal.Decimal `json:"tradingRate"`
}

// CreditTradesRecord 는 신용거래 동향 1일. 데이터가 없는 항목은 nil.
type CreditTradesRecord struct {
	Date       tosstypes.Date     `json:"date"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	MarginLoan *CreditTradeDetail `json:"marginLoan"` // 신용융자
	StockLoan  *CreditTradeDetail `json:"stockLoan"`  // 신용대주
}

// SecuritiesLendingRecord 는 대차거래 동향 1일.
type SecuritiesLendingRecord struct {
	Date              tosstypes.Date  `json:"date"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	ExecutionQuantity decimal.Decimal `json:"executionQuantity"`
	RepaymentQuantity decimal.Decimal `json:"repaymentQuantity"`
	BalanceQuantity   decimal.Decimal `json:"balanceQuantity"`
	BalanceAmount     decimal.Decimal `json:"balanceAmount"`
}

// 페이지 타입 별칭.
type (
	InvestorTradingPage   = TrendPage[InvestorTradingRecord]
	ProgramTradesPage     = TrendPage[ProgramTradesRecord]
	ShortSellingPage      = TrendPage[ShortSellingRecord]
	CreditTradesPage      = TrendPage[CreditTradesRecord]
	SecuritiesLendingPage = TrendPage[SecuritiesLendingRecord]
)

// InvestorTrading 은 투자자별 매매동향(GET /api/v1/stocks/{symbol}/investor-trading).
func (c *Client) InvestorTrading(ctx context.Context, symbol string, p TrendParams) (*InvestorTradingPage, error) {
	return fetchTrend[InvestorTradingRecord](ctx, c.http, symbol, "investor-trading", p)
}

// ProgramTrades 는 프로그램매매 동향(GET /api/v1/stocks/{symbol}/program-trades).
func (c *Client) ProgramTrades(ctx context.Context, symbol string, p TrendParams) (*ProgramTradesPage, error) {
	return fetchTrend[ProgramTradesRecord](ctx, c.http, symbol, "program-trades", p)
}

// ShortSelling 은 공매도 동향(GET /api/v1/stocks/{symbol}/short-selling).
func (c *Client) ShortSelling(ctx context.Context, symbol string, p TrendParams) (*ShortSellingPage, error) {
	return fetchTrend[ShortSellingRecord](ctx, c.http, symbol, "short-selling", p)
}

// CreditTrades 는 신용거래 동향(GET /api/v1/stocks/{symbol}/credit-trades).
func (c *Client) CreditTrades(ctx context.Context, symbol string, p TrendParams) (*CreditTradesPage, error) {
	return fetchTrend[CreditTradesRecord](ctx, c.http, symbol, "credit-trades", p)
}

// SecuritiesLending 은 대차거래 동향(GET /api/v1/stocks/{symbol}/securities-lending).
func (c *Client) SecuritiesLending(ctx context.Context, symbol string, p TrendParams) (*SecuritiesLendingPage, error) {
	return fetchTrend[SecuritiesLendingRecord](ctx, c.http, symbol, "securities-lending", p)
}

func fetchTrend[T any](ctx context.Context, hc *httpclient.Client, symbol, segment string, p TrendParams) (*TrendPage[T], error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	q := url.Values{}
	params.Int(q, "count", p.Count)
	params.Date(q, "until", p.Until)
	return fetch.One[TrendPage[T]](ctx, hc, "/api/v1/stocks/"+url.PathEscape(symbol)+"/"+segment, q)
}
