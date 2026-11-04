package documents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

// branding is the school identity printed on documents, with its images.
type branding struct {
	school    schoolData
	logo      []byte
	logoExt   string
	signature []byte
	sigExt    string
	key       string // changes when anything printed changes
}

func (b branding) assets(into map[string][]byte) {
	if len(b.logo) > 0 {
		into["logo."+b.logoExt] = b.logo
	}
	if len(b.signature) > 0 {
		into["signature."+b.sigExt] = b.signature
	}
}

func (s *Service) branding(ctx context.Context, l lang) (branding, error) {
	var row struct {
		NameEN, NameBN, LogoURL, Address, AddressBN, Phone, Email, PrimaryColor, EIIN string
	}
	if err := s.db.WithContext(ctx).Raw("SELECT name_en, name_bn, logo_url, address, address_bn, phone, email, primary_color, eiin FROM school_settings WHERE id = 1").Scan(&row).Error; err != nil {
		return branding{}, err
	}
	var card struct {
		TitleEN, TitleBN string
		SignatureType    *string
		Signature        []byte
	}
	if err := s.db.WithContext(ctx).Raw("SELECT signatory_title_en AS title_en, signatory_title_bn AS title_bn, signature_type, signature FROM id_card_settings WHERE id = 1").Scan(&card).Error; err != nil {
		return branding{}, err
	}
	b := branding{school: schoolData{Name: l.pick(row.NameEN, row.NameBN), NameAlt: l.other(row.NameEN, row.NameBN), Accent: accentOf(row.PrimaryColor), Initials: initials(row.NameEN)}}
	contact := []string{}
	// The address in the document's language when the school gave one.
	for _, c := range []string{l.pick(row.Address, row.AddressBN), row.Phone, row.Email} {
		if strings.TrimSpace(c) != "" {
			contact = append(contact, l.num(strings.TrimSpace(c)))
		}
	}
	b.school.Contact = strings.Join(contact, " · ")
	// EIIN (six digits, school settings); the line is left out when empty.
	if eiin := strings.TrimSpace(row.EIIN); eiin != "" {
		b.school.EIIN = "EIIN: " + l.num(eiin)
	}
	b.school.Signatory = l.pick(card.TitleEN, card.TitleBN)
	if card.SignatureType != nil && len(card.Signature) > 0 {
		b.signature = card.Signature
		b.sigExt = map[string]string{"image/png": "png", "image/jpeg": "jpg"}[*card.SignatureType]
		if b.sigExt != "" {
			b.school.Signature = "signature." + b.sigExt
		} else {
			b.signature = nil
		}
	}
	if row.LogoURL != "" && s.Logo != nil {
		if raw, err := s.Logo(ctx, row.LogoURL); err == nil {
			switch http.DetectContentType(raw) {
			case "image/png":
				b.logo, b.logoExt = raw, "png"
			case "image/jpeg":
				b.logo, b.logoExt = raw, "jpg"
			}
			if b.logoExt != "" {
				b.school.Logo = "logo." + b.logoExt
			}
		}
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n%s\n%s\n", row.NameEN, row.NameBN, b.school.Contact, b.school.Accent, card.TitleEN, card.TitleBN, row.LogoURL)
	h.Write(b.logo)
	h.Write(b.signature)
	b.key = hex.EncodeToString(h.Sum(nil))
	return b, nil
}

// accentOf is the school colour, or the default blue.
func accentOf(c string) string {
	c = strings.TrimSpace(c)
	if len(c) == 7 && c[0] == '#' {
		if _, err := hex.DecodeString(c[1:]); err == nil {
			return c
		}
	}
	return "#1d4ed8"
}

func initials(name string) string {
	out := []rune{}
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		if len(r) > 0 && unicode.IsLetter(r[0]) {
			out = append(out, unicode.ToUpper(r[0]))
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "S"
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// The school logo

// LogoFetcher downloads the school's logo over HTTPS from a public address
// only (no private, loopback or link-local addresses), at most 1 MB, PNG or
// JPEG, and keeps it for an hour. A failure is remembered for 10 minutes and
// the documents use the school's initials instead.
type LogoFetcher struct {
	client *http.Client
	mu     sync.Mutex
	cache  map[string]logoEntry
}

type logoEntry struct {
	data    []byte
	err     error
	expires time.Time
}

var errLogo = errors.New("logo unavailable")

func NewLogoFetcher() *LogoFetcher {
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil || port != "443" {
				return nil, errLogo
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, errLogo
			}
			for _, ip := range ips {
				if !publicIP(ip.IP) {
					return nil, errLogo
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		MaxIdleConns:          1,
	}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 2 || req.URL.Scheme != "https" {
			return errLogo
		}
		return nil
	}}
	return &LogoFetcher{client: client, cache: map[string]logoEntry{}}
}

func publicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func (f *LogoFetcher) Fetch(ctx context.Context, raw string) ([]byte, error) {
	f.mu.Lock()
	if e, ok := f.cache[raw]; ok && time.Now().Before(e.expires) {
		f.mu.Unlock()
		return e.data, e.err
	}
	f.mu.Unlock()
	data, err := f.fetch(ctx, raw)
	ttl := time.Hour
	if err != nil {
		ttl = 10 * time.Minute
	}
	f.mu.Lock()
	f.cache[raw] = logoEntry{data: data, err: err, expires: time.Now().Add(ttl)}
	f.mu.Unlock()
	return data, err
}

func (f *LogoFetcher) fetch(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return nil, errLogo
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errLogo
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, errLogo
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errLogo
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errLogo
	}
	switch http.DetectContentType(data) {
	case "image/png", "image/jpeg":
		return data, nil
	}
	return nil, errLogo
}
