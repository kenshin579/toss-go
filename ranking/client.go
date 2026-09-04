// Package ranking 은 토스 Open API 주식 랭킹(Ranking) 그룹. toss.Client.Ranking 으로 접근한다.
package ranking

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 랭킹 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
