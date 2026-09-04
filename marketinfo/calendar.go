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
	SinglePriceAuctionStartTime *time.Time `json:"singlePriceAuctionStartTime"` // 단일가 구간 시작. 결손 시 nil
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
	PreMarket     *KRSession            `json:"preMarket"`
	RegularMarket *KRSession            `json:"regularMarket"`
	AfterMarket   *KRAfterMarketSession `json:"afterMarket"`
}

// KRMarketDay 는 국내 하루 장 운영 정보. 휴장일이면 Integrated 가 nil.
type KRMarketDay struct {
	Date       tosstypes.Date     `json:"date"`
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
	Date          tosstypes.Date `json:"date"`
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

// KRMarketCalendar 는 국내 장 운영 정보를 조회한다(GET /api/v1/market-calendar/KR). date 가 비면 오늘.
func (c *Client) KRMarketCalendar(ctx context.Context, date tosstypes.Date) (*KRMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[KRMarketCalendar](ctx, c.http, "/api/v1/market-calendar/KR", q)
}

// USMarketCalendar 는 해외 장 운영 정보를 조회한다(GET /api/v1/market-calendar/US). date 가 비면 오늘.
func (c *Client) USMarketCalendar(ctx context.Context, date tosstypes.Date) (*USMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[USMarketCalendar](ctx, c.http, "/api/v1/market-calendar/US", q)
}
