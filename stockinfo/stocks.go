package stockinfo

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// KRMarketDetail 은 국내 종목의 시장 상세. 해외 종목은 nil.
type KRMarketDetail struct {
	LiquidationTrading  bool  `json:"liquidationTrading"`  // 정리매매 여부
	NXTSupported        bool  `json:"nxtSupported"`        // NXT 거래 지원 여부
	KRXTradingSuspended bool  `json:"krxTradingSuspended"` // KRX 거래정지 여부
	NXTTradingSuspended *bool `json:"nxtTradingSuspended"` // NXT 거래정지 여부. NXT 미지원 종목은 nil
}

// Stock 은 종목 기본 정보.
type Stock struct {
	Symbol             string                 `json:"symbol"`
	Name               string                 `json:"name"`
	EnglishName        string                 `json:"englishName"`
	ISINCode           string                 `json:"isinCode"`
	Market             tosstypes.Market       `json:"market"`
	SecurityType       tosstypes.SecurityType `json:"securityType"`
	IsCommonShare      bool                   `json:"isCommonShare"`
	Status             tosstypes.StockStatus  `json:"status"`
	Currency           tosstypes.Currency     `json:"currency"`
	ListDate           *tosstypes.Date        `json:"listDate"`
	DelistDate         *tosstypes.Date        `json:"delistDate"`
	SharesOutstanding  decimal.Decimal        `json:"sharesOutstanding"`
	LeverageFactor     *decimal.Decimal       `json:"leverageFactor"` // 레버리지 ETF/ETN 배수. 해당 없으면 nil
	KoreanMarketDetail *KRMarketDetail        `json:"koreanMarketDetail"`
}

// Stocks 는 여러 종목의 기본 정보를 조회한다(GET /api/v1/stocks). 최대 200개. 없는 심볼은 결과에서 빠진다.
func (c *Client) Stocks(ctx context.Context, symbols ...string) ([]Stock, error) {
	joined, err := params.Symbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Stock](ctx, c.http, "/api/v1/stocks", url.Values{"symbols": {joined}})
}

// ListedStock 은 마켓별 전체 종목 목록의 항목.
type ListedStock struct {
	Symbol        string                 `json:"symbol"`
	Name          string                 `json:"name"`
	SecurityType  tosstypes.SecurityType `json:"securityType"`
	IsCommonShare bool                   `json:"isCommonShare"`
	ISINCode      string                 `json:"isinCode"`
}

// ListStocksParams 는 ListStocks 인자.
type ListStocksParams struct {
	Market       tosstypes.Market       // 필수
	Status       tosstypes.StockStatus  // 비우면 서버 기본값(ACTIVE)
	SecurityType tosstypes.SecurityType // 비우면 전체
	CommonShare  *bool                  // nil 이면 전체
}

// ListStocks 는 마켓의 전체 종목을 조회한다(GET /api/v1/stocks/all). Rate limit 그룹 STOCK_ALL(1/s).
func (c *Client) ListStocks(ctx context.Context, p ListStocksParams) ([]ListedStock, error) {
	if err := params.Require("market", string(p.Market)); err != nil {
		return nil, err
	}
	q := url.Values{"market": {string(p.Market)}}
	params.Str(q, "status", string(p.Status))
	params.Str(q, "securityType", string(p.SecurityType))
	params.Bool(q, "commonShare", p.CommonShare)
	return fetch.List[ListedStock](ctx, c.http, "/api/v1/stocks/all", q)
}
