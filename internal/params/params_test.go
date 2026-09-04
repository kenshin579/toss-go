package params

import (
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestRequire(t *testing.T) {
	if err := Require("symbol", ""); err == nil || err.Error() != "toss: symbol must not be empty" {
		t.Errorf("got %v", err)
	}
	if err := Require("symbol", " "); err == nil {
		t.Error("whitespace must be rejected")
	}
	if err := Require("symbol", "005930"); err != nil {
		t.Error(err)
	}
}

func TestSetters_SkipZero(t *testing.T) {
	v := url.Values{}
	Str(v, "s", "")
	Int(v, "n", 0)
	Bool(v, "b", nil)
	Time(v, "t", nil)
	Date(v, "d", "")
	if len(v) != 0 {
		t.Errorf("zero values must be skipped, got %v", v)
	}
}

func TestSetters_Values(t *testing.T) {
	v := url.Values{}
	f := false
	ts := time.Date(2026, 9, 1, 0, 0, 0, 0, tosstypes.KST)
	Str(v, "s", "x")
	Int(v, "n", 7)
	Bool(v, "b", &f)
	Time(v, "t", &ts)
	Date(v, "d", tosstypes.Date("2026-09-02"))
	want := "b=false&d=2026-09-02&n=7&s=x&t=2026-09-01T00%3A00%3A00%2B09%3A00"
	if got := v.Encode(); got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}
