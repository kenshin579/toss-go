// Package marketinfo 는 토스 Open API 시장 정보(Market Info) 그룹 — 환율·국내/해외 장 운영 정보.
// toss.Client.MarketInfo 로 접근한다.
package marketinfo

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시장 정보 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
