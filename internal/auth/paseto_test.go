package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func testKey() string {
	// 32-byte key, hex-encoded
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}

func TestNewPASETOManagerValidKey(t *testing.T) {
	pm, err := NewPASETOManager(testKey())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm == nil {
		t.Fatal("manager should not be nil")
	}
}

func TestNewPASETOManagerInvalidHex(t *testing.T) {
	_, err := NewPASETOManager("not-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestNewPASETOManagerWrongKeySize(t *testing.T) {
	shortKey := hex.EncodeToString([]byte("tooshort"))
	_, err := NewPASETOManager(shortKey)
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestCreateAndValidateToken(t *testing.T) {
	pm, _ := NewPASETOManager(testKey())

	token, err := pm.CreateToken("user123", time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	if !strings.HasPrefix(token, "v4.local.") {
		t.Fatalf("expected v4.local. prefix, got %s", token[:20])
	}

	claims, err := pm.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}

	if claims.Subject != "user123" {
		t.Fatalf("expected subject user123, got %s", claims.Subject)
	}

	if claims.ExpiresAt.Before(time.Now()) {
		t.Fatal("token should not be expired")
	}
}

func TestExpiredToken(t *testing.T) {
	pm, _ := NewPASETOManager(testKey())

	// Create a token that expires immediately
	token, err := pm.CreateToken("user", -time.Second)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	_, err = pm.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error, got: %v", err)
	}
}

func TestInvalidTokenPrefix(t *testing.T) {
	pm, _ := NewPASETOManager(testKey())

	_, err := pm.ValidateToken("v3.local.invalidprefix")
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
}

func TestTamperedToken(t *testing.T) {
	pm, _ := NewPASETOManager(testKey())

	token, _ := pm.CreateToken("user", time.Hour)

	// Tamper with the token
	tampered := token[:len(token)-5] + "XXXXX"
	_, err := pm.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestDifferentKeyCannotDecrypt(t *testing.T) {
	pm1, _ := NewPASETOManager(testKey())
	pm2, _ := NewPASETOManager("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")

	token, _ := pm1.CreateToken("user", time.Hour)

	_, err := pm2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error when decrypting with different key")
	}
}

func TestGenerateKey(t *testing.T) {
	key := GenerateKey()
	if len(key) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64 hex chars, got %d", len(key))
	}

	// Should be valid hex
	_, err := hex.DecodeString(key)
	if err != nil {
		t.Fatalf("generated key is not valid hex: %v", err)
	}

	// Two calls should produce different keys
	key2 := GenerateKey()
	if key == key2 {
		t.Fatal("two generated keys should not be identical")
	}
}
