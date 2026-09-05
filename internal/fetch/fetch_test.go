package fetch

import (
	"context"
	"errors"
	"net/http"
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
	got, err := One[item](context.Background(), hc, "/one", url.Values{"a": {"1"}}, 0)
	if err != nil || got == nil || got.Symbol != "X" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestOne_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/one"}, 404, []byte(`{"error":{"requestId":"r","code":"stock-not-found","message":""}}`))
	defer done()
	got, err := One[item](context.Background(), hc, "/one", nil, 0)
	var ae *httpclient.APIError
	if got != nil || !errors.As(err, &ae) || ae.Code != "stock-not-found" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[{"symbol":"A"},{"symbol":"B"}]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil, 0)
	if err != nil || len(got) != 2 || got[1].Symbol != "B" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList_EmptyIsNonNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil, 0)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestList_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 500, []byte(`oops`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil, 0)
	if got != nil || err == nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestOne_SendsAccountHeader(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/h"}, "9", 200, []byte(`{"result":{"symbol":"X"}}`))
	defer done()
	if _, err := One[item](context.Background(), hc, "/h", nil, 9); err != nil {
		t.Fatal(err)
	}
}

func TestPostOne(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/p"}, "3", 200, []byte(`{"result":{"symbol":"P"}}`))
	defer done()
	got, err := PostOne[item](context.Background(), hc, "/p", map[string]string{"a": "b"}, 3, "k1")
	if err != nil || got == nil || got.Symbol != "P" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSend_NoContent(t *testing.T) {
	hc, done := testutil.NewServerWithHeader(t, testutil.Expect{Path: "/d"}, "2", 204, nil)
	defer done()
	if err := Send(context.Background(), hc, http.MethodDelete, "/d", nil, nil, 2); err != nil {
		t.Fatal(err)
	}
}
