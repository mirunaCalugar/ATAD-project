package links

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
)

const (
	GeneratedCodeLength = 7
	MinCodeLength       = 4
	MaxCodeLength       = 10
	codeAlphabet        = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
)

var (
	ErrInvalidURL  = errors.New("invalid url")
	ErrInvalidCode = errors.New("invalid code")
)

var (
	codePattern   = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9_-]{%d,%d}$`, MinCodeLength, MaxCodeLength))
	reservedCodes = map[string]bool{"links": true, "health": true, "version": true, "reset": true}
)

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: host is missing", ErrInvalidURL)
	}
	return nil
}

func ValidateCode(code string) error {
	if !codePattern.MatchString(code) {
		return fmt.Errorf("%w: must be %d to %d characters from A-Z a-z 0-9 - _", ErrInvalidCode, MinCodeLength, MaxCodeLength)
	}
	if reservedCodes[code] {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidCode, code)
	}
	return nil
}

func Generate() (string, error) {
	for {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		if !reservedCodes[code] {
			return code, nil
		}
	}
}

func randomCode() (string, error) {
	max := big.NewInt(int64(len(codeAlphabet)))
	b := make([]byte, GeneratedCodeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}
