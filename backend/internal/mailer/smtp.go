package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Security is how the connection to the relay is protected.
type Security string

const (
	// StartTLS connects in the clear and upgrades before authenticating. It
	// is what port 587 expects and the default.
	StartTLS Security = "starttls"
	// ImplicitTLS wraps the connection in TLS from the first byte (port 465).
	ImplicitTLS Security = "tls"
	// Insecure sends in the clear. Only for a fake relay on localhost in a
	// test; a password is refused with it.
	Insecure Security = "none"
)

// Settings describe the relay. Host is the only required field; a relay that
// needs no credentials (a test sink) leaves Username and Password empty.
type Settings struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the envelope sender and the From header address.
	From string
	// FromName is the display name shown to the reader; optional.
	FromName string
	Security Security
	// Timeout bounds the whole conversation with the relay.
	Timeout time.Duration
}

// SMTP sends through one relay. It holds no connection: each message opens
// one, which keeps a long-idle server from failing the first send of the day.
type SMTP struct{ settings Settings }

// NewSMTP checks the settings and returns a ready sender.
func NewSMTP(settings Settings) (*SMTP, error) {
	if strings.TrimSpace(settings.Host) == "" {
		return nil, errors.New("SMTP host is required")
	}
	if settings.Port < 1 || settings.Port > 65535 {
		return nil, errors.New("SMTP port must be between 1 and 65535")
	}
	from, err := Address(settings.From)
	if err != nil {
		return nil, errors.New("SMTP sender address is invalid")
	}
	settings.From = from
	switch settings.Security {
	case StartTLS, ImplicitTLS:
	case Insecure:
		if settings.Password != "" {
			return nil, errors.New("a password may not be sent over an unencrypted SMTP connection")
		}
	default:
		return nil, fmt.Errorf("unknown SMTP security %q: use starttls, tls or none", settings.Security)
	}
	if (settings.Username == "") != (settings.Password == "") {
		return nil, errors.New("SMTP username and password must be given together")
	}
	if settings.Timeout <= 0 {
		settings.Timeout = 20 * time.Second
	}
	settings.FromName = sanitizeHeader(settings.FromName)
	return &SMTP{settings: settings}, nil
}

// Sender is the address messages come from; the reset email names it as the
// address to reply to if the request was not the reader's.
func (s *SMTP) Sender() string { return s.settings.From }

// Send hands one message to the relay. The context bounds the attempt; the
// relay's own deadline bounds it too, so a hung server cannot hold a
// request open.
func (s *SMTP) Send(ctx context.Context, message Message) error {
	to, err := Address(message.To)
	if err != nil {
		return err
	}
	body, err := s.compose(to, message)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(s.settings.Timeout)
	if fromContext, ok := ctx.Deadline(); ok && fromContext.Before(deadline) {
		deadline = fromContext
	}
	address := net.JoinHostPort(s.settings.Host, fmt.Sprint(s.settings.Port))
	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	if s.settings.Security == ImplicitTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: s.settings.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect to the mail relay: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, s.settings.Host)
	if err != nil {
		return fmt.Errorf("greet the mail relay: %w", err)
	}
	defer client.Close()
	if s.settings.Security == StartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("the mail relay does not offer STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.settings.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start TLS with the mail relay: %w", err)
		}
	}
	if s.settings.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.settings.Username, s.settings.Password, s.settings.Host)); err != nil {
			return fmt.Errorf("sign in to the mail relay: %w", err)
		}
	}
	if err := client.Mail(s.settings.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("mail to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start the message: %w", err)
	}
	if _, err := writer.Write(body); err != nil {
		return fmt.Errorf("write the message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish the message: %w", err)
	}
	return client.Quit()
}

// compose builds a multipart/alternative message. Every header value is
// stripped of control characters first, so nothing in a message can inject
// one of its own.
func (s *SMTP) compose(to string, message Message) ([]byte, error) {
	var boundary [16]byte
	if _, err := rand.Read(boundary[:]); err != nil {
		return nil, err
	}
	mark := "fb-" + hex.EncodeToString(boundary[:])
	// net/mail quotes a display name that contains specials and encodes a
	// non-ASCII one, so a name can never break out into a header of its own.
	from := s.settings.From
	if s.settings.FromName != "" {
		from = (&mail.Address{Name: s.settings.FromName, Address: s.settings.From}).String()
	}
	var out strings.Builder
	write := func(name, value string) { out.WriteString(name + ": " + value + "\r\n") }
	write("From", from)
	write("To", to)
	write("Subject", mime.BEncoding.Encode("utf-8", sanitizeHeader(message.Subject)))
	write("Date", time.Now().UTC().Format(time.RFC1123Z))
	write("MIME-Version", "1.0")
	// A reset link is not an invitation to reply to a list or to resend.
	write("Auto-Submitted", "auto-generated")
	write("X-Auto-Response-Suppress", "All")
	write("Content-Type", `multipart/alternative; boundary="`+mark+`"`)
	out.WriteString("\r\n")
	for _, part := range []struct{ kind, body string }{{"text/plain", message.Text}, {"text/html", message.HTML}} {
		if strings.TrimSpace(part.body) == "" {
			continue
		}
		out.WriteString("--" + mark + "\r\n")
		out.WriteString("Content-Type: " + part.kind + "; charset=utf-8\r\n")
		out.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		out.WriteString(dotStuff(part.body))
		out.WriteString("\r\n")
	}
	out.WriteString("--" + mark + "--\r\n")
	return []byte(out.String()), nil
}

// sanitizeHeader removes the control characters that would end a header line
// and start another one.
func sanitizeHeader(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r < 0x20 {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
}

// dotStuff normalizes line endings and protects a line that begins with a
// dot, which would otherwise end the message early.
func dotStuff(body string) string {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, ".") {
			lines[i] = "." + line
		}
	}
	return strings.Join(lines, "\r\n")
}
