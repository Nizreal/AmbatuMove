package coach

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnavailable       = errors.New("AI provider unavailable")
	ErrRateLimited       = errors.New("AI request quota exceeded")
	ErrProviderQuota     = errors.New("AI provider quota exhausted")
	ErrProviderConfig    = errors.New("AI provider rejected configuration")
	ErrMalformedResponse = errors.New("AI provider response was malformed")
	ErrConsentRequired   = errors.New("external AI consent is required")
	ErrInvalid           = errors.New("invalid coach request")
	ErrNotFound          = errors.New("coach context not found")
	ErrBusy              = errors.New("AI provider concurrency limit reached")
	ErrTimeout           = errors.New("AI provider request timed out")
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ProviderResult struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
	RetryAfter   time.Duration
}

type ProviderFailure struct {
	Cause error
	Delay time.Duration
}

func (e *ProviderFailure) Error() string { return e.Cause.Error() }
func (e *ProviderFailure) Unwrap() error { return e.Cause }
func ProviderRetryAfter(err error) time.Duration {
	var failure *ProviderFailure
	if errors.As(err, &failure) {
		return failure.Delay
	}
	return 0
}

// Provider deliberately uses application-owned types so coach logic does not
// depend on a vendor SDK or its request/response structures.
type Provider interface {
	Name() string
	Generate(context.Context, string, []Message, int) (ProviderResult, error)
}
