package fees

import (
	"context"
	"crypto/rand"
	"strings"
)

// Gateway is the boundary for mobile financial services. A verified bKash or
// Nagad client may implement it in a separately approved milestone; until
// then only SimulationGateway exists, and nothing it approves is recorded as a
// payment.
type Gateway interface {
	Initiate(ctx context.Context, provider string, amountCents int64) (string, error)
	Verify(ctx context.Context, provider, reference, otp string) error
}

// SimulationCode is the only code the simulator accepts. It is public on
// purpose: a simulation proves nothing about money.
const SimulationCode = "123456"

// SimulationReferencePrefix starts every simulated reference, so one can never
// be mistaken for a provider transaction ID.
const SimulationReferencePrefix = "SIM"

// SimulationGateway imitates a provider checkout for disposable test
// installations. It never contacts a provider.
type SimulationGateway struct{}

const referenceAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (SimulationGateway) Initiate(_ context.Context, provider string, amountCents int64) (string, error) {
	if (provider != "bkash" && provider != "nagad") || amountCents <= 0 {
		return "", ErrInvalid
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(SimulationReferencePrefix)
	b.WriteString(map[string]string{"bkash": "BK", "nagad": "NG"}[provider])
	for _, v := range raw {
		b.WriteByte(referenceAlphabet[int(v)%len(referenceAlphabet)])
	}
	return b.String(), nil
}

func (SimulationGateway) Verify(_ context.Context, provider, reference, otp string) error {
	if (provider != "bkash" && provider != "nagad") || !strings.HasPrefix(reference, SimulationReferencePrefix) {
		return ErrInvalid
	}
	if otp != SimulationCode {
		return ErrDeclined
	}
	return nil
}
