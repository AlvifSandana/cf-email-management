package auth

import (
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	secret := "cloudflare_api_token_very_secret_12345"

	encrypted, err := Encrypt([]byte(secret), key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == secret {
		t.Fatalf("ciphertext equals plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != secret {
		t.Fatalf("expected '%s', got '%s'", secret, string(decrypted))
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	invalidKey := []byte("short-key")
	_, err := Encrypt([]byte("data"), invalidKey)
	if err == nil {
		t.Fatalf("expected error with short key, got nil")
	}
}

func TestDecryptCorrupted(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	_, err := Decrypt("not-valid-base64!!!", key)
	if err == nil {
		t.Fatalf("expected error on invalid base64, got nil")
	}

	_, err = Decrypt("YQ==", key) // short payload
	if err == nil {
		t.Fatalf("expected error on short ciphertext, got nil")
	}
}
