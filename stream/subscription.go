package stream

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// MaxTopics 는 연결 하나가 가질 수 있는 최대 구독 수(채널×종목 조합 기준).
const MaxTopics = 100

// 구독 type 문자열.
const (
	typeTradeKR       = "trade:kr"
	typeTradeUS       = "trade:us"
	typeOrderbookKR   = "orderbook:kr"
	typeOrderbookUS   = "orderbook:us"
	typePersonalOrder = "personal:order"
)

// Subscription 은 구독 선언의 원소 하나다. 생성자(Trade·Orderbook·PersonalOrder)를 쓴다.
type Subscription struct {
	Type  string   // 예: trade:kr, orderbook:us, personal:order
	Codes []string // 시세는 종목 symbol, personal:order 는 accountSeq 문자열
}

// Trade 는 실시간 체결 구독을 만든다. 국내는 통합 시세(KRX+NXT)다.
func Trade(market tosstypes.MarketCountry, symbols ...string) Subscription {
	return Subscription{Type: marketType("trade", market), Codes: append([]string(nil), symbols...)}
}

// Orderbook 은 실시간 호가 구독을 만든다. 국내는 통합 시세(KRX+NXT)다.
func Orderbook(market tosstypes.MarketCountry, symbols ...string) Subscription {
	return Subscription{Type: marketType("orderbook", market), Codes: append([]string(nil), symbols...)}
}

// PersonalOrder 는 본인 계좌의 주문 이벤트 구독을 만든다(와이어 type: personal:order).
// codes 에 들어가는 값은 종목이 아니라 계좌 accountSeq 다(Client.Accounts 로 조회).
func PersonalOrder(accountSeqs ...int64) Subscription {
	codes := make([]string, len(accountSeqs))
	for i, seq := range accountSeqs {
		codes[i] = strconv.FormatInt(seq, 10)
	}
	return Subscription{Type: typePersonalOrder, Codes: codes}
}

func marketType(prefix string, market tosstypes.MarketCountry) string {
	return prefix + ":" + strings.ToLower(string(market))
}

// validate 는 구독 원소의 형식을 검사한다. 시세는 종목 심볼 규칙, 주문은 양의 정수 accountSeq.
func (s Subscription) validate() error {
	switch s.Type {
	case typeTradeKR, typeTradeUS, typeOrderbookKR, typeOrderbookUS:
		if len(s.Codes) == 0 {
			return fmt.Errorf("toss: %s: codes must not be empty", s.Type)
		}
		for _, c := range s.Codes {
			if err := params.Symbol(c); err != nil {
				return err
			}
		}
	case typePersonalOrder:
		if len(s.Codes) == 0 {
			return fmt.Errorf("toss: %s: codes must not be empty", s.Type)
		}
		for _, c := range s.Codes {
			seq, err := strconv.ParseInt(c, 10, 64)
			if err != nil || seq <= 0 {
				return fmt.Errorf("toss: %s: codes must be positive accountSeq (got %q)", s.Type, c)
			}
		}
	default:
		return fmt.Errorf("toss: unknown subscription type %q", s.Type)
	}
	return nil
}

// subscriptionSet 은 현재 구독 전체를 들고 있다. 프로토콜이 full-replace 라 부분 전송이 없다.
// 같은 code 를 두 번 넣어도 1건이다(프로토콜이 topic 집합이라 참조 카운트가 없다) — 두 번
// 구독한 뒤 한 번 해제하면 완전히 빠진다.
type subscriptionSet struct {
	mu sync.Mutex
	m  map[string]map[string]struct{} // type -> code set
}

func newSubscriptionSet() *subscriptionSet {
	return &subscriptionSet{m: map[string]map[string]struct{}{}}
}

// add 는 구독을 집합에 넣는다. 상한을 넘으면 아무것도 바꾸지 않고 에러를 낸다.
func (s *subscriptionSet) add(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 먼저 결과 크기를 계산해 상한을 검사한다(부분 적용 방지).
	next := s.cloneLocked()
	mergeInto(next, subs)
	if n := countTopics(next); n > MaxTopics {
		return fmt.Errorf("toss: too many topics: %d (max %d)", n, MaxTopics)
	}
	s.m = next
	return nil
}

// remove 는 구독을 집합에서 뺀다. 마지막 code 가 빠지면 type 자체를 지운다.
// 오타 난 Unsubscribe 가 조용히 무시되지 않도록 형식도 검증한다.
func (s *subscriptionSet) remove(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range subs {
		codes := s.m[sub.Type]
		if codes == nil {
			continue
		}
		for _, c := range sub.Codes {
			delete(codes, c)
		}
		if len(codes) == 0 {
			delete(s.m, sub.Type)
		}
	}
	return nil
}

// replace 는 집합을 통째로 바꾼다(Declare). 상한을 넘으면 기존 집합을 그대로 둔다.
func (s *subscriptionSet) replace(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	next := map[string]map[string]struct{}{}
	mergeInto(next, subs)
	if n := countTopics(next); n > MaxTopics {
		return fmt.Errorf("toss: too many topics: %d (max %d)", n, MaxTopics)
	}
	s.mu.Lock()
	s.m = next
	s.mu.Unlock()
	return nil
}

// reject 는 ack 의 rejected[].target(full key)을 집합에서 제거한다.
// 거부된 항목은 원인을 고치기 전에는 재선언해도 다시 거부되므로 자동으로 뺀다.
func (s *subscriptionSet) reject(target string) {
	typ, code, ok := splitTopic(target)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if codes := s.m[typ]; codes != nil {
		delete(codes, code)
		if len(codes) == 0 {
			delete(s.m, typ)
		}
	}
}

// count 는 구독 수(채널×종목 조합)를 센다.
func (s *subscriptionSet) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return countTopics(s.m)
}

// snapshot 은 현재 집합의 복사본을 돌려준다.
func (s *subscriptionSet) snapshot() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subscription, 0, len(s.m))
	for _, typ := range sortedKeys(s.m) {
		out = append(out, Subscription{Type: typ, Codes: sortedCodes(s.m[typ])})
	}
	return out
}

// declaration 은 현재 집합 전체를 선언 배열(JSON)로 만든다.
// 집합이 비어 있으면 id 와 무관하게 `[]` 를 돌려준다 — 토스는 빈 배열만 전체 구독 해제로 해석하며,
// id 만 담긴 배열의 의미는 정의돼 있지 않다(대신 그 선언은 ack 를 짝지을 수 없다).
// id 가 비어 있지 않고 구독이 있으면 첫 원소로 넣어 ack·error 프레임에서 echo 받는다.
func (s *subscriptionSet) declaration(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) == 0 {
		return []byte("[]"), nil
	}
	arr := make([]any, 0, len(s.m)+1)
	if id != "" {
		arr = append(arr, map[string]string{"id": id})
	}
	for _, typ := range sortedKeys(s.m) {
		arr = append(arr, map[string]any{"type": typ, "codes": sortedCodes(s.m[typ])})
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return nil, fmt.Errorf("toss: encode declaration: %w", err)
	}
	return b, nil
}

func (s *subscriptionSet) cloneLocked() map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(s.m))
	for typ, codes := range s.m {
		c := make(map[string]struct{}, len(codes))
		for k := range codes {
			c[k] = struct{}{}
		}
		out[typ] = c
	}
	return out
}

// mergeInto 는 구독들을 type→code 집합에 병합한다.
func mergeInto(dst map[string]map[string]struct{}, subs []Subscription) {
	for _, sub := range subs {
		codes := dst[sub.Type]
		if codes == nil {
			codes = map[string]struct{}{}
			dst[sub.Type] = codes
		}
		for _, c := range sub.Codes {
			codes[c] = struct{}{}
		}
	}
}

// countTopics 는 잠금과 무관한 순수 함수로, 구독 수(채널×종목 조합)를 센다.
func countTopics(m map[string]map[string]struct{}) int {
	n := 0
	for _, codes := range m {
		n += len(codes)
	}
	return n
}

func sortedKeys(m map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCodes(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// splitTopic 은 full key(trade:us:AAPL, personal:order:3)를 type 과 code 로 나눈다.
// type 자체가 콜론을 포함하므로 마지막 콜론을 기준으로 자른다.
func splitTopic(topic string) (typ, code string, ok bool) {
	i := strings.LastIndex(topic, ":")
	if i <= 0 || i == len(topic)-1 {
		return "", "", false
	}
	return topic[:i], topic[i+1:], true
}
