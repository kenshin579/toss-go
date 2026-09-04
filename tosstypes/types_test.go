package tosstypes

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate_Time(t *testing.T) {
	d := Date("2026-09-03")
	got, err := d.Time()
	if err != nil {
		t.Fatalf("Time: %v", err)
	}
	if got.Year() != 2026 || got.Month() != time.September || got.Day() != 3 {
		t.Errorf("got %v", got)
	}
	if _, off := got.Zone(); off != 9*3600 {
		t.Errorf("zone offset = %d, want KST(+9h)", off)
	}
}

func TestDate_Time_Invalid(t *testing.T) {
	if _, err := Date("2026/09/03").Time(); err == nil {
		t.Error("want error for invalid format")
	}
}

func TestDate_IsZero(t *testing.T) {
	if !Date("").IsZero() {
		t.Error("empty must be zero")
	}
	if Date("2026-09-03").IsZero() {
		t.Error("non-empty must not be zero")
	}
}

func TestNewDate(t *testing.T) {
	ts := time.Date(2026, 9, 3, 23, 59, 0, 0, KST)
	if got := NewDate(ts); got != "2026-09-03" {
		t.Errorf("NewDate(KST) = %q", got)
	}
	// UTC 15:30 = KST 다음날 00:30 → KST 기준 날짜
	utc := time.Date(2026, 9, 3, 15, 30, 0, 0, time.UTC)
	if got := NewDate(utc); got != "2026-09-04" {
		t.Errorf("NewDate(UTC) = %q, want KST date", got)
	}
}

func TestDate_JSON(t *testing.T) {
	var v struct {
		D Date  `json:"d"`
		P *Date `json:"p"`
		N *Date `json:"n"`
	}
	if err := json.Unmarshal([]byte(`{"d":"2026-09-03","p":"2026-09-02","n":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.D != "2026-09-03" || v.P == nil || *v.P != "2026-09-02" || v.N != nil {
		t.Errorf("decoded %+v", v)
	}
	out, _ := json.Marshal(v.D)
	if string(out) != `"2026-09-03"` {
		t.Errorf("marshal = %s", out)
	}

	var nul struct {
		D Date `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"d":null}`), &nul); err != nil || nul.D != "" {
		t.Errorf("null into value Date: %q %v", nul.D, err)
	}
	var np *Date
	if out, _ := json.Marshal(np); string(out) != "null" {
		t.Errorf("marshal nil *Date = %s", out)
	}
}
