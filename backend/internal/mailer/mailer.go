// Package mailer sends the few transactional emails the system needs, today
// only the "forgot password" link. Delivery sits behind one small interface
// so the SMTP client can be replaced (or left out) without touching the
// callers, the same way the SMS provider is.
//
// There is no queue and no retry: a message that cannot be handed to the
// relay is logged and dropped. The only message is a password reset link,
// and asking for another one is the natural retry.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// ErrNotConfigured is returned by a Mailer that has no relay to send through.
var ErrNotConfigured = errors.New("email is not configured")

// Message is one email. Both bodies are sent as a multipart/alternative, so
// a text-only client still shows something useful.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer hands a message to a relay. Send must be safe for concurrent use.
type Mailer interface {
	Send(ctx context.Context, message Message) error
}

// Address checks one recipient address and returns it trimmed.
func Address(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return "", fmt.Errorf("invalid email address")
	}
	return address, nil
}
