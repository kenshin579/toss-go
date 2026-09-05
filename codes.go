package toss

// 자주 마주치는 토스 에러 코드. IsCode 와 함께 쓴다.
//
//	if toss.IsCode(err, toss.CodeInsufficientBuyingPower) { ... }
//
// 토스는 코드를 예고 없이 추가할 수 있으므로 이 목록은 편의일 뿐 전수가 아니다.
// 알 수 없는 코드도 그대로 *APIError.Code 에 담긴다.
const (
	CodeAccountHeaderRequired    = "account-header-required"
	CodeAccountNotFound          = "account-not-found"
	CodeAlreadyCanceled          = "already-canceled"
	CodeAlreadyFilled            = "already-filled"
	CodeConfirmHighValueRequired = "confirm-high-value-required"
	CodeInsufficientBuyingPower  = "insufficient-buying-power"
	CodeOrderNotFound            = "order-not-found"
	CodeOrderHoursClosed         = "order-hours-closed"
	CodePriceOutOfRange          = "price-out-of-range"
	CodeStockRestricted          = "stock-restricted"
	CodeRateLimitExceeded        = "rate-limit-exceeded"
	CodeIdempotencyKeyConflict   = "idempotency-key-conflict"
	CodeStockNotFound            = "stock-not-found"
	CodeRequestInProgress        = "request-in-progress"
	CodeAlreadyModified          = "already-modified"
	CodeAlreadyProcessing        = "already-processing"
	CodeModifyRestricted         = "modify-restricted"
	CodeCancelRestricted         = "cancel-restricted"
	CodeMaxOrderAmountExceeded   = "max-order-amount-exceeded"
	CodeConditionalOrderNotFound = "conditional-order-not-found"
)
