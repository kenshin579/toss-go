// Package params 는 쿼리 파라미터 조립과 필수값 검증 헬퍼다. zero-value 는 생략한다.
package params

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Require 는 필수 문자열이 비어 있으면 에러를 돌려준다.
func Require(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("toss: %s must not be empty", name)
	}
	return nil
}

// Str 은 s 가 비어 있지 않으면 설정한다.
func Str(v url.Values, key, s string) {
	if s != "" {
		v.Set(key, s)
	}
}

// Int 는 n > 0 이면 설정한다.
func Int(v url.Values, key string, n int) {
	if n > 0 {
		v.Set(key, strconv.Itoa(n))
	}
}

// Bool 은 b 가 nil 이 아니면 설정한다(false 도 명시 전송).
func Bool(v url.Values, key string, b *bool) {
	if b != nil {
		v.Set(key, strconv.FormatBool(*b))
	}
}

// Time 은 t 가 nil 이 아니면 RFC3339 로 설정한다. `+` 는 url.Values 인코딩이 `%2B` 로 처리한다.
func Time(v url.Values, key string, t *time.Time) {
	if t != nil {
		v.Set(key, t.Format(time.RFC3339))
	}
}

// Date 는 d 가 비어 있지 않으면 설정한다.
func Date(v url.Values, key string, d tosstypes.Date) {
	if !d.IsZero() {
		v.Set(key, d.String())
	}
}
