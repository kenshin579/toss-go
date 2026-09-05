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

func TestStream_StaleAckIsIgnored(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryUS, "AAPL"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	// 지나간 선언 id 로 온 ack 는 무시돼야 한다
	ts.push(`{"type":"subscriptions","id":"d-999","subscribed":[],"rejected":[{"target":"trade:us:AAPL","code":"stock-not-found","message":"stale"}]}`)
	time.Sleep(50 * time.Millisecond)
	found := false
	for _, sub := range s.Subscriptions() {
		for _, c := range sub.Codes {
			if c == "AAPL" {
				found = true
			}
		}
	}
	if !found {
		t.Error("낡은 ack 로 구독이 제거됐다")
	}

	// 양성 대조: 실제로 보낸 선언(lastDeclareID)과 같은 id 로 같은 거부가 오면 이번엔 제거돼야
	// 한다 — 프레임 경로 자체는 살아 있고, 위의 무시는 stale 판정 때문임을 증명한다.
	d := declares(ts)
	lastID := declareID(d[len(d)-1])
	if lastID == "" {
		t.Fatal("보낸 선언에서 id 를 못 찾았다")
	}
	ts.push(`{"type":"subscriptions","id":"` + lastID + `","subscribed":[],"rejected":[{"target":"trade:us:AAPL","code":"stock-not-found","message":"real"}]}`)
	waitFor(t, "set without AAPL after matching ack", func() bool {
		for _, sub := range s.Subscriptions() {
			for _, c := range sub.Codes {
				if c == "AAPL" {
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

func TestStream_RateLimitTriggersRedeclare(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithRateLimitRetryDelay(10*time.Millisecond))
	ctx := context.Background()
	if err := s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })
	before := len(declares(ts))
	ts.push(`{"type":"error","error":{"code":"rate-limit-exceeded","message":"too fast"}}`)
	// 에러는 사용자에게 전달되고
	select {
	case err := <-s.Errors():
		var de *DeclareError
		if !asDeclare(err, &de) || de.Code != "rate-limit-exceeded" {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rate-limit 에러 미수신")
	}
	// 잠시 뒤 같은 집합이 다시 선언돼야 한다
	waitFor(t, "redeclare after rate limit", func() bool {
		d := declares(ts)
		return len(d) > before && strings.Contains(d[len(d)-1], "005930")
	})
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
	// 예상 못한 연결 종료(dropConn)인데 자동 재연결이 꺼져 있으면 그 사실을 알려야 한다.
	found := false
drain:
	for {
		select {
		case err, ok := <-s.Errors():
			if !ok {
				break drain
			}
			if strings.Contains(err.Error(), "auto-reconnect is disabled") {
				found = true
			}
		case <-time.After(500 * time.Millisecond):
			break drain
		}
	}
	if !found {
		t.Error("자동 재연결이 꺼진 채 끊겼는데 그 사실을 알리는 에러가 없었다")
	}
}

// TestStream_DeliberateCloseWithoutAutoReconnectIsQuiet 는 사용자가 직접 Close 를 호출한
// 경우엔(예상 못한 끊김이 아니므로) auto-reconnect-disabled 진단 에러가 나오지 않아야 함을 본다.
func TestStream_DeliberateCloseWithoutAutoReconnectIsQuiet(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithoutAutoReconnect())
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err, ok := <-s.Errors():
		if ok {
			t.Errorf("직접 Close 했는데 진단 에러가 나왔다: %v", err)
		}
	default:
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

func TestStream_BackpressureDoesNotStormReconnects(t *testing.T) {
	// Orders() 를 소비하지 않으면 백프레셔로 계속 끊긴다. 이때 재연결이 백오프로 제한되지 않으면
	// 계정당 2연결·선언 5회/초 제한이 있는 서버를 향해 초당 수천 번 다이얼하게 된다.
	ts := newTestServer(t)
	newTestStream(t, ts, WithOrderBuffer(1), WithBackoff(50*time.Millisecond, 200*time.Millisecond))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ev := `{"type":"message","topic":"personal:order:3","data":{"event":"PENDING","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"PENDING","quantity":"1","currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","execution":{"filledQuantity":"0","averageFilledPrice":null,"filledAmount":null,"commission":null,"tax":null,"settlementDate":null}}}}`
	stop := make(chan struct{})
	go func() { // 연결이 살아날 때마다 계속 밀어 넣어 백프레셔를 유지한다
		for {
			select {
			case <-stop:
				return
			default:
				ts.push(ev)
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()
	time.Sleep(600 * time.Millisecond)
	close(stop)
	// 50ms 백오프면 600ms 동안 많아야 십여 회다. 폭주하면 수백~수천 회가 된다.
	if n := ts.connCount(); n > 30 {
		t.Errorf("600ms 동안 연결 %d회 — 백프레셔 재연결이 백오프로 제한되지 않는다", n)
	}
	if n := ts.connCount(); n < 2 {
		t.Errorf("재연결이 아예 일어나지 않았다(%d) — 테스트가 무의미하다", n)
	}
}

func TestStream_ClosesOldConnBeforeReconnect(t *testing.T) {
	// 계정당 동시 연결은 2개다. 재연결 전에 기존 연결을 닫지 않으면 새 연결이 자기 자신을 밀어내
	// 끊김이 반복된다. 클라이언트가 먼저 끊는 경로(백프레셔)로 여러 번 재연결시켜 확인한다.
	ts := newTestServer(t)
	// 백오프를 기본 테스트값(5~20ms)보다 넉넉히 잡는다 — CI 부하가 큰 환경에서 3번의 재연결이
	// 촘촘한 타이밍에 몰리면서 생기는 오탐을 줄인다(이 테스트는 타이밍이 아니라 동시 연결 수만 본다).
	s := newTestStream(t, ts, WithOrderBuffer(1), WithBackoff(30*time.Millisecond, 90*time.Millisecond))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ev := `{"type":"message","topic":"personal:order:3","data":{"event":"PENDING","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"PENDING","quantity":"1","currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","execution":{"filledQuantity":"0","averageFilledPrice":null,"filledAmount":null,"commission":null,"tax":null,"settlementDate":null}}}}`
	for reconnects := 0; reconnects < 3; reconnects++ {
		for i := 0; i < 4; i++ { // 소비하지 않아 버퍼를 넘긴다
			ts.push(ev)
		}
		select {
		case <-s.Reconnects():
		case <-time.After(3 * time.Second):
			t.Fatalf("재연결 %d 회차 미수신", reconnects+1)
		}
		// 스텁 서버는 옛 연결의 live-- 를 비동기로 처리한다(Read 에러 → cancel → outer select →
		// CloseNow → defer, 최소 goroutine 스케줄링 2~3홉). 클라이언트는 첫 재시도를 즉시(0 대기)
		// 하므로, 바로 다음 백프레셔를 몰아치면 그 비동기 처리가 아직 안 끝난 상태에서 새 연결이
		// 뜬 것처럼 관측될 수 있다(실제로 클라이언트가 옛 연결을 안 닫아서가 아니라, 스텁의 회계가
		// 새 다이얼을 못 따라간 것 — closeConn 은 프로그램 순서상 항상 다음 dial 보다 먼저 끝난다).
		// 다음 라운드로 넘어가기 전에 짧게 쉬어 회계가 안정되게 한다.
		time.Sleep(20 * time.Millisecond)
	}
	if got := ts.maxConcurrent(); got > 1 {
		t.Errorf("동시 연결 %d — 재연결 전에 기존 연결을 닫지 않았다", got)
	}
	if ts.connCount() < 4 {
		t.Errorf("총 연결 %d, 재연결이 실제로 일어났는지 확인 필요", ts.connCount())
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

func TestStream_InvalidOptionsFallBackToDefaults(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithTradeBuffer(0), WithOrderBuffer(-1), WithPingInterval(0), WithBackoff(0, 0))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	// 0 버퍼면 pushLossy 가 무한 스핀해 읽기 루프가 멈춘다 — 이벤트가 도착하면 정상이다
	ts.push(`{"type":"message","topic":"trade:kr:005930","data":{"price":"72000","volume":"5","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"KRW"}}`)
	select {
	case ev := <-s.Trades():
		if ev.Price.String() != "72000" {
			t.Errorf("ev = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("잘못된 버퍼 옵션이 스트림을 멈췄다")
	}
}

func TestPushLossy_CapOneKeepsNewest(t *testing.T) {
	ch := make(chan int, 1)
	pushLossy(ch, 1)
	pushLossy(ch, 2)
	if got := <-ch; got != 2 {
		t.Errorf("got %d, want 2 (가득 차면 최신을 유지해야 한다)", got)
	}
}

func TestPushLossy_CapZeroReturnsWithoutBlocking(t *testing.T) {
	ch := make(chan int) // cap 0 — 예전 구현은 여기서 무한 스핀했다
	done := make(chan struct{})
	go func() {
		pushLossy(ch, 1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cap 0 채널에서 pushLossy 가 반환되지 않았다(무한 스핀)")
	}
}

// TestSubscribe_ReturnsRejection 은 Subscribe 가 자신이 넣은 항목의 거부를 *RejectedError 로
// 돌려주는지 본다(C: ack 를 기다리는 Subscribe/Unsubscribe/Declare).
func TestSubscribe_ReturnsRejection(t *testing.T) {
	ts := newTestServer(t)
	ts.setAutoAck(false) // 이 테스트가 직접 ack 를 통제한다
	s := newTestStream(t, ts)
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Subscribe(ctx, Trade(tosstypes.MarketCountryUS, "AAPL", "NOPE")) }()
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	ts.push(`{"type":"subscriptions","subscribed":["trade:us:AAPL"],"rejected":[{"target":"trade:us:NOPE","code":"stock-not-found","message":"없음"}]}`)
	select {
	case err := <-errCh:
		var re *RejectedError
		if !asRejected(err, &re) || re.Target != "trade:us:NOPE" {
			t.Errorf("Subscribe err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe 가 반환되지 않았다")
	}
}

// TestSubscribe_ReturnsDeclareError 는 선언 자체가 실패(error 프레임)하면 Subscribe 가
// *DeclareError 를 돌려주는지 본다.
func TestSubscribe_ReturnsDeclareError(t *testing.T) {
	ts := newTestServer(t)
	ts.setAutoAck(false)
	s := newTestStream(t, ts)
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930")) }()
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	d := declares(ts)
	id := declareID(d[len(d)-1])
	if id == "" {
		t.Fatal("보낸 선언에서 id 를 못 찾았다")
	}
	ts.push(`{"type":"error","id":"` + id + `","error":{"code":"too-many-topics","message":"초과"}}`)
	select {
	case err := <-errCh:
		var de *DeclareError
		if !asDeclare(err, &de) || de.Code != "too-many-topics" {
			t.Errorf("Subscribe err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe 가 반환되지 않았다")
	}
}

// TestSubscribe_ContextCanceled 는 ack 가 오지 않는 상태에서 ctx 가 먼저 끝나면 Subscribe 가
// ctx.Err() 를 돌려주는지 본다.
func TestSubscribe_ContextCanceled(t *testing.T) {
	ts := newTestServer(t)
	ts.setAutoAck(false) // ack 를 주지 않아 Subscribe 가 계속 기다리게 한다
	s := newTestStream(t, ts)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930")) }()
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe 가 반환되지 않았다")
	}
}

// TestSubscribe_DisconnectUnblocksWaiter 는 ack 를 기다리는 도중 연결이 끊기면 Subscribe 가
// 영원히 막히지 않고 에러로 풀리는지 본다.
func TestSubscribe_DisconnectUnblocksWaiter(t *testing.T) {
	ts := newTestServer(t)
	ts.setAutoAck(false)
	s := newTestStream(t, ts)
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930")) }()
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	ts.dropConn() // ack 없이 연결을 끊는다
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("연결이 끊겼는데 Subscribe 가 nil 을 반환했다")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("연결이 끊겼는데도 Subscribe 가 풀리지 않았다")
	}
}

// TestSubscribe_SurvivesIDlessErrorFrame 은 서버가 error 프레임에 id 를 echo 하지 않아도(코드가
// 이미 그 비대칭을 가정한다) 호출자가 영원히 막히지 않는지 본다. 뒤이은 재선언의 ack 가
// full-replace 흡수 로직 덕분에 옛 배치의 대기자를 대신 풀어줘야 한다.
func TestSubscribe_SurvivesIDlessErrorFrame(t *testing.T) {
	ts := newTestServer(t)
	ts.setAutoAck(false)
	s := newTestStream(t, ts, WithRateLimitRetryDelay(10*time.Millisecond))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Subscribe(context.Background(), Trade(tosstypes.MarketCountryKR, "005930"))
	}()
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })
	ts.push(`{"type":"error","error":{"code":"rate-limit-exceeded","message":"too fast"}}`) // id 없음
	// 재선언이 나가면 그 ack 로 대기자가 풀려야 한다
	waitFor(t, "redeclare", func() bool { return len(declares(ts)) >= 2 })
	d := declares(ts)
	ts.push(`{"type":"subscriptions","id":"` + declareID(d[len(d)-1]) + `","subscribed":["trade:kr:005930"],"rejected":[]}`)
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Subscribe = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Subscribe 가 영원히 대기한다 — 고아 배치")
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
