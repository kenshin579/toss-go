// Package indicators 는 토스 Open API 시장 지표(Market Indicators) 그룹 — 지수 현재가·캔들·투자자별 매매대금.
// toss.Client.MarketIndicators 로 접근한다.
package indicators

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시장 지표 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
