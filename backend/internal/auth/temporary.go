package auth

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// TemporaryPassword returns a 14-character password in three groups of four
// (for example "k7Rm-2xQp-9Fhd"), avoiding look-alike letters and digits so
// it can be read off a printed slip. It is used once: the account must set
// its own password at the next sign-in.
func TemporaryPassword() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyzACDEFGHJKMNPQRTUVWXY2346789"
	var b strings.Builder
	for i := 0; i < 12; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String(), nil
}
