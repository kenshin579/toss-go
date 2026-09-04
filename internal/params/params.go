// Package params 는 쿼리 파라미터 조립과 필수값 검증 헬퍼다. zero-value 는 생략한다.
package params

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Require 는 필수 문자열이 비어 있으면 에러를 돌려준다. 값은 그대로 전송되므로 공백 트리밍은 하지 않는다.
func Require(name, v string) error {
	if v == "" {
		return fmt.Errorf("toss: %s must not be empty", name)
	}
	return nil
}

// symbolPattern 은 토스 심볼 규칙(openapi components.parameters.Symbol): 영문 대/소문자, 숫자, '.', '-'.
var symbolPattern = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

// MaxSymbols 는 symbols= 쿼리에 넣을 수 있는 최대 심볼 수(openapi: 최대 200개).
const MaxSymbols = 200

// Symbol 은 심볼 형식을 검증한다(빈 값·공백·허용 외 문자 거부). 요청을 보내기 전에 실패시켜 rate limit 을 아낀다.
func Symbol(v string) error {
	if !symbolPattern.MatchString(v) {
		return fmt.Errorf("toss: invalid symbol %q (allowed: A-Z a-z 0-9 . -)", v)
	}
	return nil
}

// Symbols 는 symbols= 쿼리 값을 만든다. 빈 목록, 형식 위반 원소, MaxSymbols 초과를 거부한다.
func Symbols(symbols []string) (string, error) {
	if len(symbols) == 0 {
		return "", errors.New("toss: symbols must not be empty")
	}
	if len(symbols) > MaxSymbols {
		return "", fmt.Errorf("toss: too many symbols %d (max %d)", len(symbols), MaxSymbols)
	}
	for _, s := range symbols {
		if err := Symbol(s); err != nil {
			return "", err
		}
	}
	return strings.Join(symbols, ","), nil
}

// Str 은 s 가 비어 있지 않으면 설정한다.
func Str(v url.Values, key, s string) {
	if s != "" {
		v.Set(key, s)
	}
}

// Int 는 n > 0 이면 설정한다. 스펙상 모든 integer 파라미터는 minimum 1 이라 0 은 "미지정" 으로 안전하다.
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
