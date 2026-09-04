package toss

import (
	"errors"
	"os"
)

// NewClientFromEnv 는 TOSS_CLIENT_ID / TOSS_CLIENT_SECRET 환경변수로 Client 를 만든다.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	id, secret := os.Getenv("TOSS_CLIENT_ID"), os.Getenv("TOSS_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, errors.New("toss: TOSS_CLIENT_ID and TOSS_CLIENT_SECRET must be set")
	}
	return NewClient(id, secret, opts...)
}
