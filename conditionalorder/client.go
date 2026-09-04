// Package conditionalorder 는 토스 Open API 조건주문 그룹 — 생성·수정·취소·목록·상세.
// toss.Client.Account(seq).ConditionalOrder 로 접근하며, 모든 요청에 계좌 헤더가 실린다.
//
// 조건주문은 트리거 조건(가격 도달·목표 수익률)이 충족되면 실제 주문을 낸다.
// SDK 는 요청 조립 오류만 사전 검증하고, 호가단위·잔고 등은 서버가 판단한다.
package conditionalorder

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 조건주문 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
