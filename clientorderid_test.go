package toss

import (
	"strings"
	"testing"
)

func TestNewClientOrderID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewClientOrderID()
		if err := ValidateClientOrderID(id); err != nil {
			t.Fatalf("generated id %q invalid: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestValidateClientOrderID(t *testing.T) {
	for _, ok := range []string{"a", "A-b_C9", strings.Repeat("x", 36)} {
		if err := ValidateClientOrderID(ok); err != nil {
			t.Errorf("ValidateClientOrderID(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 37), "has space", "한글", "a.b", "a/b"} {
		if err := ValidateClientOrderID(bad); err == nil {
			t.Errorf("ValidateClientOrderID(%q) must fail", bad)
		}
	}
}
