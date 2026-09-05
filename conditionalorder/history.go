package conditionalorder

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// ListParams 는 조건주문 목록 조회 인자. Status 는 필수다.
type ListParams struct {
	Status StatusFilter // 필수. OPEN / CLOSED
	Symbol string       // 특정 종목만. 비우면 전체
	Cursor string       // 이전 응답의 NextCursor
	Limit  int          // 최대 100, 0 이면 서버 기본값(20)
}

// List 는 조건주문 목록을 조회한다(GET /api/v1/conditional-orders).
// 이 API 로 등록한 조건주문뿐 아니라 다른 채널(토스증권 앱 등)에서 등록한 것도 함께 반환된다.
//
// 대표 에러: invalid-request(잘못된 status).
func (c *Client) List(ctx context.Context, p ListParams) (*Page, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("status", string(p.Status)); err != nil {
		return nil, err
	}
	q := url.Values{"status": {string(p.Status)}}
	if p.Symbol != "" {
		if err := params.Symbol(p.Symbol); err != nil {
			return nil, err
		}
		q.Set("symbol", p.Symbol)
	}
	params.Str(q, "cursor", p.Cursor)
	params.Int(q, "limit", p.Limit)
	return fetch.One[Page](ctx, c.http, "/api/v1/conditional-orders", q, c.accountSeq)
}

// Get 은 조건주문 상세를 조회한다(GET /api/v1/conditional-orders/{id}). 진행 중 + 종료된 조건주문을
// 모두 조회할 수 있다.
//
// 대표 에러: conditional-order-not-found.
func (c *Client) Get(ctx context.Context, id string) (*Detail, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("conditionalOrderId", id); err != nil {
		return nil, err
	}
	return fetch.One[Detail](ctx, c.http, "/api/v1/conditional-orders/"+url.PathEscape(id), nil, c.accountSeq)
}
