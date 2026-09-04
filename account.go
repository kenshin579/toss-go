package toss

import (
	"context"

	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/internal/fetch"
)

// AccountScope 는 특정 계좌(accountSeq)에 고정된 sub-client 묶음이다.
// 이 아래의 모든 요청에는 X-Tossinvest-Account 헤더가 자동으로 실린다.
// 여러 goroutine 에서 동시에 사용해도 안전하다.
type AccountScope struct {
	AccountSeq int64

	Asset *asset.Client // 자산: 보유 주식
}

// Account 는 accountSeq 에 고정된 스코프를 만든다. 네트워크 호출은 없다.
// accountSeq 는 Accounts 로 조회한다.
//
//	accts, _ := c.Accounts(ctx)
//	a := c.Account(accts[0].AccountSeq)
//	h, _ := a.Asset.Holdings(ctx, nil)
func (c *Client) Account(accountSeq int64) *AccountScope {
	return &AccountScope{
		AccountSeq: accountSeq,
		Asset:      asset.New(c.http, accountSeq),
	}
}

// fetchList 는 루트에서 계좌 헤더 없이 목록을 조회한다.
func fetchList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	return fetch.List[T](ctx, c.http, path, nil, 0)
}
