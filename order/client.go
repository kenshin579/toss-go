// Package order 는 토스 Open API 주문 그룹 — 주문 생성·정정·취소, 주문 조회, 주문 정보.
// toss.Client.Account(seq).Order 로 접근하며, 모든 요청에 계좌 헤더가 실린다.
//
// 주문은 실제 체결로 이어진다. SDK 는 요청 조립 오류(필수 누락·형식)만 사전 검증하고,
// 호가단위·잔고·거래시간 같은 상태 의존 규칙은 서버가 판단한다 — 에러는 *toss.APIError 로 온다.
//
// 멱등성: PlaceRequest.ClientOrderID 를 채우면 (1) 같은 값으로 재요청 시 토스가 이전 주문 결과를
// 그대로 돌려주고(10분), (2) SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다.
// 같은 키로 내용이 다른 요청을 보내면 400 idempotency-key-conflict 를 받는다.
package order

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 주문 sub-client. accountSeq 에 고정된다.
type Client struct {
	http       *httpclient.Client
	accountSeq int64
}

// New 는 internal 용도 — toss.Client.Account 가 호출한다.
func New(hc *httpclient.Client, accountSeq int64) *Client {
	return &Client{http: hc, accountSeq: accountSeq}
}
