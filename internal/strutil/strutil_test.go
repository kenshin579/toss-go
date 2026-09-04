package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	if got := Truncate("  a \n\n b  ", 100); got != "a b" {
		t.Errorf("collapse = %q", got)
	}
	if got := Truncate("abcdef", 3); got != "abc" {
		t.Errorf("ascii = %q", got)
	}
	s := strings.Repeat("가", 100) // 300 bytes
	got := Truncate(s, 200)
	if !utf8.ValidString(got) || len(got) != 198 || utf8.RuneCountInString(got) != 66 {
		t.Errorf("rune boundary: len=%d runes=%d valid=%v", len(got), utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	if got := Truncate("", 5); got != "" {
		t.Errorf("empty = %q", got)
	}
}
