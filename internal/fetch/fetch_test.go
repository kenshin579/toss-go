package fetch

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/internal/testutil"
)

type item struct {
	Symbol string `json:"symbol"`
}

func TestOne(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/one", Query: url.Values{"a": {"1"}}}, 200, []byte(`{"result":{"symbol":"X"}}`))
	defer done()
	got, err := One[item](context.Background(), hc, "/one", url.Values{"a": {"1"}})
	if err != nil || got == nil || got.Symbol != "X" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestOne_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/one"}, 404, []byte(`{"error":{"requestId":"r","code":"stock-not-found","message":""}}`))
	defer done()
	got, err := One[item](context.Background(), hc, "/one", nil)
	var ae *httpclient.APIError
	if got != nil || !errors.As(err, &ae) || ae.Code != "stock-not-found" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[{"symbol":"A"},{"symbol":"B"}]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if err != nil || len(got) != 2 || got[1].Symbol != "B" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList_EmptyIsNonNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestList_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 500, []byte(`oops`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if got != nil || err == nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}
