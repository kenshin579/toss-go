// Package stockinfo 는 토스 Open API 종목 정보(Stock Info) 그룹 — 종목 메타·전체 목록·매수 유의사항·
// 투자자별/프로그램/공매도/신용/대차 매매동향. toss.Client.StockInfo 로 접근한다.
package stockinfo

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 종목 정보 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
