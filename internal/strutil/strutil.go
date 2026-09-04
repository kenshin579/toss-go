// Package strutil 은 에러 메시지 정리용 소형 문자열 헬퍼다.
package strutil

import (
	"strings"
	"unicode/utf8"
)

// Truncate 는 s 의 연속 공백·개행을 한 칸으로 합친 뒤 최대 n 바이트로 자른다. UTF-8 문자 경계를 지킨다.
func Truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
