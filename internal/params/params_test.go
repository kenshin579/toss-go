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
	if err := Require("symbol", " "); err != nil {
		t.Error("whitespace is not empty; format is validated by Symbol")
	}
	if err := Require("symbol", "005930"); err != nil {
		t.Error(err)
	}
}

func TestSymbol(t *testing.T) {
	for _, ok := range []string{"005930", "AAPL", "BRK.B", "BF-B", "aapl"} {
		if err := Symbol(ok); err != nil {
			t.Errorf("Symbol(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", " 005930", "005930 ", "삼성", "A/B", "a,b"} {
		if err := Symbol(bad); err == nil {
			t.Errorf("Symbol(%q) must fail", bad)
		}
	}
}

func TestIndicatorSymbol(t *testing.T) {
	for _, ok := range []string{"KOSPI", "KOSDAQ", "KR_BOND_10Y", "kr_bond_2y"} {
		if err := IndicatorSymbol(ok); err != nil {
			t.Errorf("IndicatorSymbol(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "BRK.B", "BF-B", "KR BOND", "코스피"} {
		if err := IndicatorSymbol(bad); err == nil {
			t.Errorf("IndicatorSymbol(%q) must fail", bad)
		}
	}
	if err := Symbol("KR_BOND_10Y"); err == nil {
		t.Error("stock Symbol must reject '_'")
	}
	if got, err := IndicatorSymbols([]string{"KOSPI", "KR_BOND_10Y"}); err != nil || got != "KOSPI,KR_BOND_10Y" {
		t.Errorf("IndicatorSymbols = %q, %v", got, err)
	}
	if _, err := IndicatorSymbols([]string{"KOSPI", "BRK.B"}); err == nil {
		t.Error("IndicatorSymbols must reject '.'")
	}
}

func TestSymbols(t *testing.T) {
	if got, err := Symbols([]string{"005930", "AAPL"}); err != nil || got != "005930,AAPL" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := Symbols(nil); err == nil {
		t.Error("empty must fail")
	}
	if _, err := Symbols([]string{"005930", ""}); err == nil {
		t.Error("empty element must fail")
	}
	many := make([]string, MaxSymbols+1)
	for i := range many {
		many[i] = "A"
	}
	if _, err := Symbols(many); err == nil {
		t.Error("over max must fail")
	}
	if _, err := Symbols(many[:MaxSymbols]); err != nil {
		t.Errorf("exactly max must pass: %v", err)
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
