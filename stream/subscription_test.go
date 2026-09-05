package stream

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestSubscriptionConstructors(t *testing.T) {
	if got := Trade(tosstypes.MarketCountryKR, "005930"); got.Type != "trade:kr" || len(got.Codes) != 1 || got.Codes[0] != "005930" {
		t.Errorf("Trade(KR) = %+v", got)
	}
	if got := Trade(tosstypes.MarketCountryUS, "AAPL", "TSLA"); got.Type != "trade:us" || len(got.Codes) != 2 {
		t.Errorf("Trade(US) = %+v", got)
	}
	if got := Orderbook(tosstypes.MarketCountryKR, "005930"); got.Type != "orderbook:kr" {
		t.Errorf("Orderbook = %+v", got)
	}
	// personal:order 의 codes 는 종목이 아니라 accountSeq 문자열이다
	if got := Order(3, 7); got.Type != "personal:order" || got.Codes[0] != "3" || got.Codes[1] != "7" {
		t.Errorf("Order = %+v", got)
	}
}

func TestSubscriptionSet_AddRemoveDeclare(t *testing.T) {
	s := newSubscriptionSet()
	if err := s.add(Trade(tosstypes.MarketCountryKR, "005930"), Orderbook(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	if err := s.add(Trade(tosstypes.MarketCountryKR, "000660")); err != nil {
		t.Fatal(err)
	}
	if n := s.count(); n != 3 {
		t.Errorf("count = %d, want 3 (채널×종목 조합 기준)", n)
	}
	// 같은 type 은 하나의 배열 원소로 합쳐진다
	body, err := s.declaration("d-1")
	if err != nil {
		t.Fatal(err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("declaration is not a JSON array: %s", body)
	}
	if len(arr) != 3 { // id 1개 + trade:kr 1개 + orderbook:kr 1개
		t.Fatalf("declaration = %s", body)
	}
	if arr[0]["id"] != "d-1" {
		t.Errorf("first element must be the request id: %s", body)
	}
	seen := map[string]int{}
	for _, e := range arr[1:] {
		typ, _ := e["type"].(string)
		codes, _ := e["codes"].([]any)
		seen[typ] = len(codes)
	}
	if seen["trade:kr"] != 2 || seen["orderbook:kr"] != 1 {
		t.Errorf("merged codes = %v (%s)", seen, body)
	}

	s.remove(Trade(tosstypes.MarketCountryKR, "005930"))
	if n := s.count(); n != 2 {
		t.Errorf("after remove count = %d", n)
	}
	// 마지막 code 를 지우면 type 자체가 사라진다
	s.remove(Orderbook(tosstypes.MarketCountryKR, "005930"))
	body, _ = s.declaration("d-2")
	_ = json.Unmarshal(body, &arr)
	for _, e := range arr[1:] {
		if e["type"] == "orderbook:kr" {
			t.Errorf("empty type must be dropped: %s", body)
		}
	}
}

func TestSubscriptionSet_EmptyDeclarationIsArray(t *testing.T) {
	s := newSubscriptionSet()
	body, err := s.declaration("")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `[]` {
		t.Errorf("empty declaration = %s, want []", body)
	}
}

func TestSubscriptionSet_Validation(t *testing.T) {
	s := newSubscriptionSet()
	if err := s.add(Subscription{Type: "trade:kr", Codes: []string{"삼성"}}); err == nil {
		t.Error("잘못된 심볼은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "personal:order", Codes: []string{"0"}}); err == nil {
		t.Error("accountSeq 0 은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "personal:order", Codes: []string{"abc"}}); err == nil {
		t.Error("accountSeq 는 숫자여야 한다")
	}
	if err := s.add(Subscription{Type: "bogus", Codes: []string{"x"}}); err == nil {
		t.Error("알 수 없는 type 은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "trade:kr"}); err == nil {
		t.Error("빈 codes 는 거부해야 한다")
	}
}

func TestSubscriptionSet_MaxTopics(t *testing.T) {
	s := newSubscriptionSet()
	codes := make([]string, MaxTopics)
	for i := range codes {
		// 6자리 고유 코드(플랜 원문의 "00000"+i%10 생성식은 10종만 나와 중복되므로, 정확히
		// MaxTopics 개의 서로 다른 코드를 만들도록 %06d 로 바꿨다).
		codes[i] = fmt.Sprintf("%06d", i)
	}
	if err := s.add(Subscription{Type: "trade:kr", Codes: codes}); err != nil {
		t.Fatalf("정확히 %d 건은 허용해야 한다: %v", MaxTopics, err)
	}
	if err := s.add(Trade(tosstypes.MarketCountryKR, "999999")); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("%d 건 초과는 거부해야 한다: %v", MaxTopics, err)
	}
}

func TestSubscriptionSet_RejectRemoves(t *testing.T) {
	s := newSubscriptionSet()
	_ = s.add(Trade(tosstypes.MarketCountryUS, "AAPL", "NOPE"))
	// ack 의 rejected[].target 은 full key 다
	s.reject("trade:us:NOPE")
	body, _ := s.declaration("")
	if strings.Contains(string(body), "NOPE") {
		t.Errorf("거부된 항목은 집합에서 빠져야 한다: %s", body)
	}
	if !strings.Contains(string(body), "AAPL") {
		t.Errorf("나머지는 유지돼야 한다: %s", body)
	}
}

func TestSubscriptionSet_Snapshot(t *testing.T) {
	s := newSubscriptionSet()
	_ = s.add(Trade(tosstypes.MarketCountryKR, "005930"), Order(3))
	got := s.snapshot()
	if len(got) != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
	// 스냅샷을 바꿔도 내부 집합은 영향받지 않는다
	got[0].Codes[0] = "999999"
	body, _ := s.declaration("")
	if strings.Contains(string(body), "999999") {
		t.Error("snapshot 은 복사본이어야 한다")
	}
}
