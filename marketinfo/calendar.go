package marketinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// KRSession 은 국내 프리마켓/정규장 세션.
type KRSession struct {
	StartTime                   time.Time  `json:"startTime"`
	SinglePriceAuctionStartTime *time.Time `json:"singlePriceAuctionStartTime"` // 단일가 구간 시작. 슬롯별 의미는 KRIntegratedHours 필드 주석 참고. nil 이면 결손/휴장
	EndTime                     time.Time  `json:"endTime"`
}

// KRAfterMarketSession 은 국내 애프터마켓 세션.
type KRAfterMarketSession struct {
	StartTime                 time.Time  `json:"startTime"`
	SinglePriceAuctionEndTime *time.Time `json:"singlePriceAuctionEndTime"` // 단일가 구간 종료. 결손 시 nil
	EndTime                   time.Time  `json:"endTime"`
}

// KRIntegratedHours 는 통합(KRX+NXT) 거래 가능 시간. 휴장 세션은 nil.
type KRIntegratedHours struct {
	PreMarket     *KRSession            `json:"preMarket"`     // NXT 프리마켓(접속매매). SinglePriceAuctionStartTime = 시가단일가 시작(결손 시 nil). 휴장이면 nil
	RegularMarket *KRSession            `json:"regularMarket"` // KRX·NXT 정규장 합집합(가장 이른 시작~가장 늦은 종료). SinglePriceAuctionStartTime = 종가단일가 시작(KRX 기준, KRX 휴장 시 nil). 휴장이면 nil
	AfterMarket   *KRAfterMarketSession `json:"afterMarket"`   // 애프터마켓. 휴장이면 nil
}

// KRMarketDay 는 국내 하루 장 운영 정보. 휴장일이면 Integrated 가 nil.
type KRMarketDay struct {
	Date       tosstypes.Date     `json:"date"` // 영업일(KST 기준)
	Integrated *KRIntegratedHours `json:"integrated"`
}

// KRMarketCalendar 는 국내 장 운영 정보(오늘·직전·다음 영업일).
type KRMarketCalendar struct {
	Today               KRMarketDay `json:"today"`
	PreviousBusinessDay KRMarketDay `json:"previousBusinessDay"`
	NextBusinessDay     KRMarketDay `json:"nextBusinessDay"`
}

// USSession 은 해외 세션(시작·종료).
type USSession struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

// USMarketDay 는 해외 하루 장 운영 정보. 휴장 세션은 nil.
type USMarketDay struct {
	Date          tosstypes.Date `json:"date"`      // 영업일(미국 현지 기준). 세션 시각은 모두 KST 이며 RegularMarket/AfterMarket 은 KST 자정을 넘어간다(예: 22:30 → 익일 05:00)
	DayMarket     *USSession     `json:"dayMarket"` // 데이마켓(토스증권)
	PreMarket     *USSession     `json:"preMarket"`
	RegularMarket *USSession     `json:"regularMarket"`
	AfterMarket   *USSession     `json:"afterMarket"`
}

// USMarketCalendar 는 해외 장 운영 정보(오늘·직전·다음 영업일).
type USMarketCalendar struct {
	Today               USMarketDay `json:"today"`
	PreviousBusinessDay USMarketDay `json:"previousBusinessDay"`
	NextBusinessDay     USMarketDay `json:"nextBusinessDay"`
}

// KRMarketCalendar 는 국내 장 운영 정보를 조회한다(GET /api/v1/market-calendar/KR). date 는 KST 기준, 비면 오늘. 지원 범위 밖 날짜는 400 unsupported-date.
func (c *Client) KRMarketCalendar(ctx context.Context, date tosstypes.Date) (*KRMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[KRMarketCalendar](ctx, c.http, "/api/v1/market-calendar/KR", q)
}

// USMarketCalendar 는 해외 장 운영 정보를 조회한다(GET /api/v1/market-calendar/US). date 는 미국 현지 날짜(비면 오늘). tosstypes.NewDate 는 KST 변환이므로 미국 날짜는 Date(t.In(loc).Format("2006-01-02")) 로 직접 만든다.
func (c *Client) USMarketCalendar(ctx context.Context, date tosstypes.Date) (*USMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[USMarketCalendar](ctx, c.http, "/api/v1/market-calendar/US", q)
}
