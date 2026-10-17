package fees

import (
	"context"
	"crypto/rand"
	"strings"
)

// Gateway is the boundary for mobile financial services. Production bKash or
// Nagad clients can replace MockGateway without changing the service.
type Gateway interface {
	Initiate(ctx context.Context, provider string, amountCents int64) (string, error)
	Verify(ctx context.Context, provider, reference, otp string) error
}

// MockOTP is the only one-time password the mock gateway accepts.
const MockOTP = "123456"

// MockGateway simulates a provider checkout: it issues a random transaction
// reference and approves only MockOTP. It never contacts a real provider.
type MockGateway struct{}

const referenceAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (MockGateway) Initiate(_ context.Context, provider string, amountCents int64) (string, error) {
	if (provider != "bkash" && provider != "nagad") || amountCents <= 0 {
		return "", ErrInvalid
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(map[string]string{"bkash": "BK", "nagad": "NG"}[provider])
	for _, v := range raw {
		b.WriteByte(referenceAlphabet[int(v)%len(referenceAlphabet)])
	}
	return b.String(), nil
}

func (MockGateway) Verify(_ context.Context, provider, reference, otp string) error {
	if (provider != "bkash" && provider != "nagad") || reference == "" {
		return ErrInvalid
	}
	if otp != MockOTP {
		return ErrDeclined
	}
	return nil
}
