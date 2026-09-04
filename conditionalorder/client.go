// Package conditionalorder 는 토스 Open API 조건주문 그룹 — 생성·수정·취소·목록·상세.
// toss.Client.Account(seq).ConditionalOrder 로 접근하며, 모든 요청에 계좌 헤더가 실린다.
//
// 조건주문은 트리거 조건(가격 도달)이 충족되면 실제 주문을 낸다. 이 API 로 만들 수 있는 조건은
// 가격 도달(STOP)뿐이다. 목표 수익률(PROFIT_RATE) 조건은 토스 앱 등에서 만든 것을 조회만 할 수 있다.
// SDK 는 요청 조립 오류만 사전 검증하고, 호가단위·잔고 등은 서버가 판단한다.
//
// 발동 세션: 국내는 KRX 정규장에서만, 해외는 거래 가능한 모든 시간대에 발동된다.
//
// 멱등성: PlaceRequest.ClientOrderID 를 채우면 같은 값으로 재요청할 때 서버가 중복 생성을 막고
// (10분 유효), SDK 가 401 토큰 오류에 1회 재시도한다. 키가 없으면 재시도하지 않는다. 같은 키로 다른
// 내용을 보내면 400 idempotency-key-conflict.
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
