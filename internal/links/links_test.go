package links

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"https", "https://go.dev/doc/effective_go", false},
		{"http with port and query", "http://example.org:8080/a?b=c", false},
		{"ftp scheme", "ftp://example.org/file", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"no scheme", "example.org", true},
		{"scheme only", "https://", true},
		{"empty", "", true},
		{"not a url", "::not a url", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateURL(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidURL) {
				t.Errorf("ValidateURL(%q) error = %v, want ErrInvalidURL", tt.in, err)
			}
		})
	}
}

func TestValidateCode(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"min length", "abcd", false},
		{"max length", "abcdefghij", false},
		{"all allowed characters", "aZ0-_", false},
		{"too short", "abc", true},
		{"too long", "abcdefghijk", true},
		{"space", "ab cd", true},
		{"slash", "ab/cd", true},
		{"non ascii", "ăbcd", true},
		{"reserved links", "links", true},
		{"reserved health", "health", true},
		{"reserved version", "version", true},
		{"reserved reset", "reset", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidCode) {
				t.Errorf("ValidateCode(%q) error = %v, want ErrInvalidCode", tt.in, err)
			}
		})
	}
}

func TestGenerateProducesValidCodes(t *testing.T) {
	seen := make(map[string]bool)
	const samples = 1000
	for range samples {
		code, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(code) != GeneratedCodeLength {
			t.Fatalf("Generate() = %q, length %d, want %d", code, len(code), GeneratedCodeLength)
		}
		if err := ValidateCode(code); err != nil {
			t.Fatalf("Generate() = %q is not a valid code: %v", code, err)
		}
		seen[code] = true
	}
	if len(seen) < samples-1 {
		t.Errorf("Generate produced %d distinct codes out of %d", len(seen), samples)
	}
}

func TestGenerateUsesOnlyAlphabet(t *testing.T) {
	code, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, c := range code {
		if !strings.ContainsRune(codeAlphabet, c) {
			t.Errorf("Generate() = %q contains %q outside the alphabet", code, c)
		}
	}
}
