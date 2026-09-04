// Package fetch 는 그룹 패키지가 공유하는 제네릭 요청 헬퍼다. 검증·쿼리 조립은 호출 측이 하고,
// 여기서는 httpclient 호출과 결과 포인터/슬라이스 반환만 담당한다.
package fetch

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

// One 은 GET 으로 result 객체 하나를 *T 로 디코딩한다. accountSeq 가 0 이 아니면 계좌 헤더를 붙인다.
// GET 은 항상 재시도되므로(httpclient.canRetry) IdempotencyKey 를 넘길 필요가 없다.
func One[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values, accountSeq int64) (*T, error) {
	var out T
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: path, Query: q, AccountSeq: accountSeq, Out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// List 는 GET 으로 result 배열을 []T 로 디코딩한다. 빈 배열은 nil 이 아닌 빈 슬라이스, 실패 시 nil 과 에러.
func List[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values, accountSeq int64) ([]T, error) {
	out := []T{}
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: path, Query: q, AccountSeq: accountSeq, Out: &out}); err != nil {
		return nil, err
	}
	return out, nil
}

// PostOne 은 POST 로 body 를 보내고 result 를 *T 로 디코딩한다.
// clientOrderID 는 body 에 실린 멱등성 키를 그대로 넘긴다(없으면 빈 문자열) — 401 재시도 허용 여부를 결정한다.
func PostOne[T any](ctx context.Context, hc *httpclient.Client, path string, body any, accountSeq int64, clientOrderID string) (*T, error) {
	var out T
	if err := hc.Do(ctx, httpclient.Request{Method: http.MethodPost, Path: path, Body: body, AccountSeq: accountSeq, IdempotencyKey: clientOrderID, Out: &out}); err != nil {
		return nil, err
	}
	return &out, nil
}

// Send 는 응답 본문을 쓰지 않는 요청(예: DELETE 204)을 보낸다. q 는 필요 없으면 nil.
func Send(ctx context.Context, hc *httpclient.Client, method, path string, q url.Values, body any, accountSeq int64) error {
	return hc.Do(ctx, httpclient.Request{Method: method, Path: path, Query: q, Body: body, AccountSeq: accountSeq})
}
