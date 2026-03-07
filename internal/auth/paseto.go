package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	pasetoV4LocalPrefix = "v4.local."
)

// Claims represents the payload of a PASETO token.
type Claims struct {
	Subject   string    `json:"sub"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// PASETOManager handles creation and validation of PASETO v4.local tokens.
type PASETOManager struct {
	key []byte // 32-byte symmetric key
}

// NewPASETOManager creates a PASETOManager from a hex-encoded 32-byte key.
func NewPASETOManager(hexKey string) (*PASETOManager, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}
	if len(key) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", chacha20poly1305.KeySize, len(key))
	}
	return &PASETOManager{key: key}, nil
}

// CreateToken creates a PASETO v4.local token for the given subject and duration.
func (p *PASETOManager) CreateToken(subject string, duration time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		Subject:   subject,
		IssuedAt:  now,
		ExpiresAt: now.Add(duration),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	aead, err := chacha20poly1305.NewX(p.key)
	if err != nil {
		return "", fmt.Errorf("create xchacha20poly1305: %w", err)
	}

	nonce := make([]byte, aead.NonceSize()) // 24 bytes
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, payload, nil)

	// Token format: v4.local.<base64url(nonce + ciphertext)>
	raw := make([]byte, len(nonce)+len(ciphertext))
	copy(raw, nonce)
	copy(raw[len(nonce):], ciphertext)

	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return pasetoV4LocalPrefix + encoded, nil
}

// ValidateToken validates a PASETO v4.local token and returns the claims.
func (p *PASETOManager) ValidateToken(token string) (*Claims, error) {
	if !strings.HasPrefix(token, pasetoV4LocalPrefix) {
		return nil, errors.New("invalid token prefix: expected v4.local.")
	}

	encoded := strings.TrimPrefix(token, pasetoV4LocalPrefix)
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	aead, err := chacha20poly1305.NewX(p.key)
	if err != nil {
		return nil, fmt.Errorf("create xchacha20poly1305: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, errors.New("token too short")
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(plaintext, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().UTC().After(claims.ExpiresAt) {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

// GenerateKey generates a random 32-byte key and returns it hex-encoded.
func GenerateKey() string {
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("failed to generate random key: %v", err))
	}
	return hex.EncodeToString(key)
}
