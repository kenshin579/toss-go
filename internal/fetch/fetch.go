// Package fetch 는 그룹 패키지가 공유하는 제네릭 조회 헬퍼다. 검증·쿼리 조립은 호출 측이 하고,
// 여기서는 httpclient.Get 호출과 결과 포인터/슬라이스 반환만 담당한다.
package fetch

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

// One 은 result 객체 하나를 *T 로 디코딩한다. 실패 시 nil 과 에러.
func One[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values) (*T, error) {
	var out T
	if err := hc.Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List 는 result 배열을 []T 로 디코딩한다. 빈 배열은 nil 이 아닌 빈 슬라이스, 실패 시 nil 과 에러.
func List[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values) ([]T, error) {
	out := []T{}
	if err := hc.Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}
