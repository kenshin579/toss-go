package toss

import (
	"context"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Account 는 계좌 하나의 식별 정보.
type Account struct {
	AccountNo   string                `json:"accountNo"`   // 계좌번호
	AccountSeq  int64                 `json:"accountSeq"`  // 계좌 일련번호. Client.Account 와 X-Tossinvest-Account 헤더에 쓴다
	AccountType tosstypes.AccountType `json:"accountType"` // 계좌 종류
}

// Accounts 는 계좌 목록을 조회한다(GET /api/v1/accounts).
// 계좌 헤더가 필요 없는 유일한 계좌 API 이며, 여기서 얻은 AccountSeq 로 Client.Account 를 만든다.
// Rate limit 그룹 ACCOUNT(초당 1회)이므로 반복 호출하지 말고 결과를 재사용한다.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	return fetchList[Account](ctx, c, "/api/v1/accounts")
}
