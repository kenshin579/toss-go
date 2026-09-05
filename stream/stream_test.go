package stream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

func newTestStream(t *testing.T, ts *testServer, opts ...Option) *Stream {
	t.Helper()
	opts = append([]Option{WithBaseURL(ts.url), WithPingInterval(30 * time.Millisecond),
		WithBackoff(5*time.Millisecond, 20*time.Millisecond), WithCoalesceDelay(time.Millisecond)}, opts...)
	s, err := New(context.Background(), staticToken("tok"), opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStream_HandshakeSendsBearer(t *testing.T) {
	ts := newTestServer(t)
	newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	if h := ts.authHeaders(); len(h) == 0 || h[0] != "Bearer tok" {
		t.Errorf("Authorization = %v", h)
	}
}

func TestStream_SubscribeDeclaresFullSet(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	if err := s.Subscribe(context.Background(), Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })
	if err := s.Subscribe(context.Background(), Orderbook(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	// 두 번째 선언에는 첫 구독도 함께 들어 있어야 한다(full-replace)
	waitFor(t, "second declare", func() bool {
		d := declares(ts)
		return len(d) >= 2 && strings.Contains(d[len(d)-1], "trade:kr") && strings.Contains(d[len(d)-1], "orderbook:kr")
	})
}

func TestStream_UnsubscribeAndDeclare(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930", "000660"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	_ = s.Unsubscribe(ctx, Trade(tosstypes.MarketCountryKR, "000660"))
	waitFor(t, "unsubscribe declare", func() bool {
		d := declares(ts)
		return len(d) >= 2 && !strings.Contains(d[len(d)-1], "000660")
	})
	// Declare 는 집합을 통째로 바꾼다
	_ = s.Declare(ctx, Orderbook(tosstypes.MarketCountryUS, "AAPL"))
	waitFor(t, "replace declare", func() bool {
		d := declares(ts)
		last := d[len(d)-1]
		return len(d) >= 3 && strings.Contains(last, "AAPL") && !strings.Contains(last, "005930")
	})
	if got := s.Subscriptions(); len(got) != 1 || got[0].Type != "orderbook:us" {
		t.Errorf("Subscriptions = %+v", got)
	}
}

func TestStream_EmptyDeclareUnsubscribesAll(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	_ = s.Declare(ctx)
	waitFor(t, "empty declare", func() bool {
		d := declares(ts)
		return strings.TrimSpace(d[len(d)-1]) == "[]"
	})
}

func TestStream_PingIsRawText(t *testing.T) {
	ts := newTestServer(t)
	newTestStream(t, ts)
	waitFor(t, "ping", func() bool {
		for _, f := range ts.frames() {
			if f == "PING" { // JSON 이 아니라 순수 텍스트여야 한다
				return true
			}
			if strings.Contains(f, `"PING"`) {
				t.Errorf("PING 은 JSON 이 아니라 순수 텍스트여야 한다: %s", f)
			}
		}
		return false
	})
}

func TestStream_DispatchesEvents(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })

	ts.push(`{"type":"message","topic":"trade:kr:005930","data":{"price":"72000","volume":"5","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"KRW"}}`)
	select {
	case ev := <-s.Trades():
		if ev.Symbol != "005930" || ev.Price.String() != "72000" {
			t.Errorf("trade = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("trade event 미수신")
	}

	ts.push(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":null,"currency":"KRW","asks":[{"price":"72100","volume":"10"}],"bids":[]}}`)
	select {
	case ev := <-s.Orderbooks():
		if len(ev.Asks) != 1 || ev.Timestamp != nil {
			t.Errorf("orderbook = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("orderbook event 미수신")
	}
}

func TestStream_AckRejectRemovesFromSet(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryUS, "AAPL", "NOPE"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	ts.push(`{"type":"subscriptions","subscribed":["trade:us:AAPL"],"rejected":[{"target":"trade:us:NOPE","code":"stock-not-found","message":"없음"}]}`)

	select {
	case err := <-s.Errors():
		var re *RejectedError
		if !asRejected(err, &re) || re.Target != "trade:us:NOPE" {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rejected 알림 미수신")
	}
	// 거부된 항목은 집합에서 빠져 다음 선언에 포함되지 않는다
	waitFor(t, "set without NOPE", func() bool {
		for _, sub := range s.Subscriptions() {
			for _, c := range sub.Codes {
				if c == "NOPE" {
					return false
				}
			}
		}
		return true
	})
}

func TestStream_ErrorFrame(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.push(`{"type":"error","error":{"code":"rate-limit-exceeded","message":"too fast"}}`)
	select {
	case err := <-s.Errors():
		var de *DeclareError
		if !asDeclare(err, &de) || de.Code != "rate-limit-exceeded" {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("error frame 미수신")
	}
}

func TestStream_ReconnectsAndRedeclares(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930"))
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })

	ts.dropConn()
	select {
	case r := <-s.Reconnects():
		if r.Attempt < 1 {
			t.Errorf("reconnect = %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect 알림 미수신")
	}
	// 재연결 후 저장된 구독이 다시 선언돼야 한다
	waitFor(t, "re-declare", func() bool {
		if ts.connCount() < 2 {
			return false
		}
		d := declares(ts)
		return len(d) >= 2 && strings.Contains(d[len(d)-1], "005930")
	})
}

func TestStream_ServerShutdownCause(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.push(`{"type":"error","error":{"code":"server-shutdown","message":"배포"}}`)
	ts.dropConn()
	select {
	case r := <-s.Reconnects():
		if r.Cause != ReconnectServerShutdown {
			t.Errorf("cause = %q, want server-shutdown", r.Cause)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect 미수신")
	}
}

func TestStream_WithoutAutoReconnectClosesChannels(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithoutAutoReconnect())
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.dropConn()
	select {
	case _, ok := <-s.Trades():
		if ok {
			t.Error("자동 재연결이 꺼져 있으면 채널이 닫혀야 한다")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}
}

func TestStream_TradeBufferDropsOldest(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithTradeBuffer(2))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	for _, p := range []string{"1", "2", "3", "4"} {
		ts.push(`{"type":"message","topic":"trade:kr:005930","data":{"price":"` + p + `","volume":"1","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"KRW"}}`)
	}
	// 버퍼가 2 이므로 오래된 것이 버려지고 최신 2개가 남는다
	waitFor(t, "buffered trades", func() bool { return len(s.Trades()) == 2 })
	first := <-s.Trades()
	if first.Price.String() == "1" {
		t.Errorf("가득 찬 시세 버퍼는 오래된 것을 버려야 한다: %s", first.Price)
	}
}

func TestStream_OrderBackpressureReconnects(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithOrderBuffer(1))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ev := `{"type":"message","topic":"personal:order:3","data":{"event":"PENDING","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"PENDING","quantity":"1","currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","execution":{"filledQuantity":"0","averageFilledPrice":null,"filledAmount":null,"commission":null,"tax":null,"settlementDate":null}}}}`
	for i := 0; i < 5; i++ { // 소비하지 않고 버퍼를 넘긴다
		ts.push(ev)
	}
	select {
	case r := <-s.Reconnects():
		if r.Cause != ReconnectBackpressure {
			t.Errorf("cause = %q, want backpressure", r.Cause)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backpressure 재연결 미수신")
	}
}

func TestStream_MaxTopicsRejectedBeforeSend(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	codes := make([]string, MaxTopics+1)
	for i := range codes {
		codes[i] = fmt.Sprintf("%06d", i) // 고유해야 상한에 걸린다(중복 code 는 집합에서 합쳐진다)
	}
	err := s.Subscribe(context.Background(), Subscription{Type: "trade:kr", Codes: codes})
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("상한 초과는 요청 전에 거부돼야 한다: %v", err)
	}
}

func TestStream_CloseIsIdempotent(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("두 번째 Close 도 안전해야 한다: %v", err)
	}
	if err := s.Subscribe(context.Background(), Trade(tosstypes.MarketCountryKR, "005930")); err == nil {
		t.Error("닫힌 스트림에 Subscribe 하면 에러여야 한다")
	}
}

func asRejected(err error, target **RejectedError) bool { return errors.As(err, target) }

func asDeclare(err error, target **DeclareError) bool { return errors.As(err, target) }

// declares 는 스텁이 받은 프레임 중 선언(JSON 배열)만 고른다.
func declares(ts *testServer) []string {
	var out []string
	for _, f := range ts.frames() {
		if strings.HasPrefix(strings.TrimSpace(f), "[") {
			out = append(out, f)
		}
	}
	return out
}
