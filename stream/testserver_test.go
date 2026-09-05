package stream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testServer 는 실제 웹소켓 업그레이드를 하는 스텁이다.
// 수신한 텍스트 프레임을 기록하고, 테스트가 원하는 프레임을 밀어 넣을 수 있다.
//
// 기본적으로 클라이언트가 보낸 선언(빈 배열이 아닌 JSON 배열)에는 자동으로 성공 ack 를 돌려준다 —
// 실제 서버 동작과 같고, Subscribe/Unsubscribe/Declare 가 ack 를 기다리는 지금 구조에서 그렇게
// 하지 않으면 대부분의 테스트가 멈춰 버린다. 특정 ack(거부·에러·낡은 id 등)를 직접 통제해야 하는
// 테스트는 setAutoAck(false) 로 끄고 push 로 원하는 프레임을 보낸다.
type testServer struct {
	srv *httptest.Server
	url string

	mu       sync.Mutex
	received []string      // 클라이언트가 보낸 텍스트 프레임(선언·PING)
	conns    int           // 총 연결 수(재연결 검증용)
	authHdr  []string      // 연결별 Authorization 헤더
	pushCh   chan string   // 서버 → 클라이언트로 밀어 넣을 프레임
	closeCh  chan struct{} // 현재 연결을 강제 종료하라는 신호
	live     int           // 현재 살아 있는 연결 수
	maxLive  int           // 관측된 최대 동시 연결 수
	autoAck  bool          // 선언을 받으면 자동으로 성공 ack 를 돌려줄지
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{pushCh: make(chan string, 64), closeCh: make(chan struct{}, 8), autoAck: true}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ts.mu.Lock()
		ts.conns++
		ts.authHdr = append(ts.authHdr, r.Header.Get("Authorization"))
		ts.live++
		if ts.live > ts.maxLive {
			ts.maxLive = ts.live
		}
		ts.mu.Unlock()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() { // 읽기 루프: 수신 프레임 기록 + PING 에 pong 응답 + 선언에 자동 ack
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					// 연결이 죽었다는 것을 서버가 처음 아는 지점이다(핸들러 바깥의 defer 로
					// 미루면 outer select 가 ctx.Done() 을 관측하고 CloseNow 를 부르는 goroutine
					// 스케줄링 홉이 최소 1~2번 더 끼어들어, 그 사이 새 연결이 뜬 것처럼 관측될 수
					// 있다 — live-- 는 여기 한 곳에서만 한다(핸들러 쪽에는 defer 로 두지 않는다).
					ts.mu.Lock()
					ts.live--
					ts.mu.Unlock()
					cancel()
					return
				}
				s := string(data)
				ts.mu.Lock()
				ts.received = append(ts.received, s)
				autoAck := ts.autoAck
				ts.mu.Unlock()
				if s == "PING" {
					_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"pong"}`))
					continue
				}
				trimmed := strings.TrimSpace(s)
				if autoAck && strings.HasPrefix(trimmed, "[") && trimmed != "[]" {
					if id := declareID(s); id != "" {
						ack := `{"type":"subscriptions","id":"` + id + `"}`
						_ = c.Write(ctx, websocket.MessageText, []byte(ack))
					}
				}
			}
		}()
		for {
			select {
			case <-ctx.Done():
				_ = c.CloseNow()
				return
			case <-ts.closeCh:
				_ = c.CloseNow()
				return
			case msg := <-ts.pushCh:
				if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
					return
				}
			}
		}
	}))
	ts.url = "ws" + strings.TrimPrefix(ts.srv.URL, "http")
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *testServer) push(msg string) { ts.pushCh <- msg }

func (ts *testServer) dropConn() { ts.closeCh <- struct{}{} }

func (ts *testServer) connCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.conns
}

func (ts *testServer) frames() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.received...)
}

func (ts *testServer) authHeaders() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.authHdr...)
}

// maxConcurrent 는 지금까지 관측된 최대 동시 연결 수다(재연결 전에 기존 연결을 닫는지 검증용).
func (ts *testServer) maxConcurrent() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.maxLive
}

// setAutoAck 는 선언에 대한 자동 ack 를 켜고 끈다. 거부·에러·낡은 id 등 ack 내용을 테스트가
// 직접 통제해야 할 때 false 로 끈다.
func (ts *testServer) setAutoAck(v bool) {
	ts.mu.Lock()
	ts.autoAck = v
	ts.mu.Unlock()
}

// declareID 는 선언 프레임(JSON 배열)의 첫 원소가 {"id":"..."} 형태면 그 id 를 돌려준다.
// 없으면 빈 문자열.
func declareID(raw string) string {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arr); err != nil || len(arr) == 0 {
		return ""
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arr[0], &first); err != nil {
		return ""
	}
	return first.ID
}

// waitFor 는 조건이 참이 될 때까지 최대 2초 기다린다.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// staticToken 은 고정 토큰을 돌려주는 TokenFunc.
func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}
