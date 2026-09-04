package order

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// ListParams 는 주문 목록 조회 인자. Status 는 필수다.
type ListParams struct {
	Status StatusFilter   // 필수. OPEN(진행 중) 또는 CLOSED(종료)
	Symbol string         // 특정 종목만. 비우면 전체
	From   tosstypes.Date // 주문 생성일(orderedAt, KST) 기준 시작일(inclusive)
	To     tosstypes.Date // 주문 생성일 기준 종료일(inclusive)
	Cursor string         // 이전 응답의 NextCursor
	Limit  int            // 최대 100, 0 이면 서버 기본값(20). Status 가 OPEN 이면 무시되고 전량 반환된다
}

// List 는 주문 목록을 조회한다(GET /api/v1/orders).
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
	params.Date(q, "from", p.From)
	params.Date(q, "to", p.To)
	params.Str(q, "cursor", p.Cursor)
	params.Int(q, "limit", p.Limit)
	return fetch.One[Page](ctx, c.http, "/api/v1/orders", q, c.accountSeq)
}

// Get 은 주문 상세를 조회한다(GET /api/v1/orders/{orderId}). 없으면 404 order-not-found.
func (c *Client) Get(ctx context.Context, orderID string) (*Order, error) {
	if err := params.AccountSeq(c.accountSeq); err != nil {
		return nil, err
	}
	if err := params.Require("orderId", orderID); err != nil {
		return nil, err
	}
	return fetch.One[Order](ctx, c.http, "/api/v1/orders/"+url.PathEscape(orderID), nil, c.accountSeq)
}
