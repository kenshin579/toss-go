// Package asset 은 토스 Open API 자산(Asset) 그룹 — 보유 주식.
// toss.Client.Account(seq).Asset 으로 접근하며, 모든 요청에 계좌 헤더가 실린다.
package asset

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 자산 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
