package auth

import "crypto/subtle"

// BearerAuth validates bearer tokens against a set of known valid tokens.
type BearerAuth struct {
	tokens map[string]bool
}

// NewBearerAuth creates a BearerAuth from a list of valid tokens.
func NewBearerAuth(tokens []string) *BearerAuth {
	m := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		m[t] = true
	}
	return &BearerAuth{tokens: m}
}

// Validate checks whether the given token matches any known valid token
// using constant-time comparison.
func (b *BearerAuth) Validate(token string) bool {
	for known := range b.tokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(known)) == 1 {
			return true
		}
	}
	return false
}
