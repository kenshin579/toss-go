// Package indicators 는 토스 Open API 시장 지표(Market Indicators) 그룹 — 지수 현재가·캔들·투자자별 매매대금.
// toss.Client.MarketIndicators 로 접근한다.
// 지표 심볼 카탈로그(1.2.14 기준): KOSPI, KOSDAQ, KR_BOND_2Y, KR_BOND_3Y, KR_BOND_5Y, KR_BOND_10Y, KR_BOND_20Y, KR_BOND_30Y. 지수는 포인트, 국채는 수익률(%) 단위이며 통화 필드가 없다. 지원하지 않는 심볼은 400 unsupported-symbol.
package indicators

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시장 지표 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
