package mailer

import (
	"strings"
	"testing"
)

func TestNewSMTPChecksSettings(t *testing.T) {
	good := Settings{Host: "smtp-relay.example.net", Port: 587, Username: "user", Password: "key", From: "noreply@example.edu.bd", Security: StartTLS}
	if _, err := NewSMTP(good); err != nil {
		t.Fatalf("valid settings refused: %v", err)
	}
	for name, settings := range map[string]Settings{
		"no host":               {Port: 587, From: "a@example.test", Security: StartTLS},
		"port out of range":     {Host: "relay", Port: 0, From: "a@example.test", Security: StartTLS},
		"sender not an address": {Host: "relay", Port: 587, From: "not an address", Security: StartTLS},
		"unknown security":      {Host: "relay", Port: 587, From: "a@example.test", Security: "ssl"},
		// A password must never cross an unencrypted connection, even in a test.
		"password in the clear":     {Host: "relay", Port: 25, From: "a@example.test", Password: "key", Security: Insecure},
		"username with no password": {Host: "relay", Port: 587, From: "a@example.test", Username: "user", Security: StartTLS},
	} {
		if _, err := NewSMTP(settings); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// TestComposeCannotBeInjected is the point of sanitizeHeader and dotStuff: no
// part of a message may add a header of its own or end the message early.
func TestComposeCannotBeInjected(t *testing.T) {
	sender, err := NewSMTP(Settings{Host: "relay", Port: 587, From: "noreply@example.edu.bd",
		FromName: "Green Valley\r\nBcc: snoop@example.test", Security: StartTLS})
	if err != nil {
		t.Fatal(err)
	}
	body, err := sender.compose("parent@example.test", Message{
		Subject: "Reset\r\nBcc: snoop@example.test",
		Text:    "line one\n.\n.hidden\nlast line",
		HTML:    "<p>hello</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := string(body)
	headers, content, found := strings.Cut(message, "\r\n\r\n")
	if !found {
		t.Fatal("no header block")
	}
	// Nothing may start a header line of its own; an injected value may at
	// most end up as harmless text inside the header it belongs to.
	for _, line := range strings.Split(headers, "\r\n") {
		name, _, ok := strings.Cut(line, ":")
		if !ok || !allowedHeader(name) {
			t.Fatalf("unexpected header line %q in:\n%s", line, headers)
		}
	}
	// A display name with specials in it is quoted, so it stays one name.
	if !strings.HasPrefix(headers, `From: "Green ValleyBcc: snoop@example.test" <noreply@example.edu.bd>`) {
		t.Fatalf("the display name is not quoted:\n%s", headers)
	}
	// A Bangla subject travels as an encoded word.
	bangla, err := sender.compose("parent@example.test", Message{Subject: "পাসওয়ার্ড", Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bangla), "Subject: =?utf-8?b?") {
		t.Fatalf("a Bangla subject is not encoded:\n%s", bangla)
	}
	for _, want := range []string{"From: ", "To: parent@example.test", "MIME-Version: 1.0", "multipart/alternative"} {
		if !strings.Contains(headers, want) {
			t.Errorf("missing header %q", want)
		}
	}
	// A line that began with a dot is stuffed, so it cannot end the message.
	if !strings.Contains(content, "\r\n..\r\n") || !strings.Contains(content, "\r\n..hidden\r\n") {
		t.Errorf("leading dots are not stuffed:\n%s", content)
	}
	if strings.Contains(content, "line one\n") {
		t.Error("bare newlines were not converted to CRLF")
	}
	for _, want := range []string{"text/plain; charset=utf-8", "text/html; charset=utf-8", "<p>hello</p>"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing part %q", want)
		}
	}
}

// TestComposeSkipsAnEmptyPart keeps a text-only message from carrying an
// empty HTML alternative, which some clients render as a blank email.
func TestComposeSkipsAnEmptyPart(t *testing.T) {
	sender, err := NewSMTP(Settings{Host: "relay", Port: 587, From: "noreply@example.edu.bd", Security: StartTLS})
	if err != nil {
		t.Fatal(err)
	}
	body, err := sender.compose("parent@example.test", Message{Subject: "Hello", Text: "plain only"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "text/html") {
		t.Fatalf("an empty HTML part was included:\n%s", body)
	}
}

// allowedHeader lists the header names compose is allowed to write.
func allowedHeader(name string) bool {
	switch name {
	case "From", "To", "Subject", "Date", "MIME-Version", "Auto-Submitted", "X-Auto-Response-Suppress", "Content-Type":
		return true
	}
	return false
}

func TestAddressRejectsRubbish(t *testing.T) {
	if got, err := Address("  parent@example.test  "); err != nil || got != "parent@example.test" {
		t.Fatalf("trimmed address: %q %v", got, err)
	}
	for _, bad := range []string{"", "parent", "parent@", "a b@example.test", "Someone <a@example.test>"} {
		if _, err := Address(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
