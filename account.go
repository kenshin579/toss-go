package toss

import (
	"context"

	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/order"
)

// AccountScope 는 특정 계좌(accountSeq)에 고정된 sub-client 묶음이다.
// 이 아래의 모든 요청에는 X-Tossinvest-Account 헤더가 자동으로 실린다.
// 여러 goroutine 에서 동시에 사용해도 안전하다.
type AccountScope struct {
	accountSeq int64

	Asset *asset.Client // 자산: 보유 주식
	Order *order.Client // 주문: 생성·정정·취소·조회·주문 정보
}

// AccountSeq 는 이 스코프가 고정된 계좌 일련번호를 돌려준다. 생성 후 바뀌지 않는다.
func (a *AccountScope) AccountSeq() int64 { return a.accountSeq }

// Account 는 accountSeq 에 고정된 스코프를 만든다. 네트워크 호출은 없다.
// accountSeq 는 Accounts 로 조회한다. 0 이하를 넘기면 스코프의 모든 호출이 요청 전에 에러를 낸다
// (계좌 헤더 없이 요청하면 서버가 account-header-required 를 돌려주므로 미리 막는다).
//
//	accts, _ := c.Accounts(ctx)
//	a := c.Account(accts[0].AccountSeq)
//	h, _ := a.Asset.Holdings(ctx, nil)
func (c *Client) Account(accountSeq int64) *AccountScope {
	return &AccountScope{
		accountSeq: accountSeq,
		Asset:      asset.New(c.http, accountSeq),
		Order:      order.New(c.http, accountSeq),
	}
}

// fetchList 는 루트에서 계좌 헤더 없이 목록을 조회한다.
func fetchList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	return fetch.List[T](ctx, c.http, path, nil, 0)
}
