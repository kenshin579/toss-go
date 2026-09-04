// Package marketdata 는 토스 Open API 시세(Market Data) 그룹 — 현재가·호가·체결·상하한가·캔들.
// toss.Client.MarketData 로 접근한다.
package marketdata

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시세 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
