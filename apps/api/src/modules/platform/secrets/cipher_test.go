package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCipherRoundTripAndTamperDetection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, bytes.Repeat([]byte{7}, 32), 0o600); err != nil {
		t.Fatalf("write test key: %v", err)
	}
	cipher, err := NewCipherFromFile(path)
	if err != nil {
		t.Fatalf("NewCipherFromFile() error = %v", err)
	}

	encrypted, err := cipher.Encrypt("access-sandbox-secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if bytes.Contains(encrypted, []byte("access-sandbox-secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil || decrypted != "access-sandbox-secret" {
		t.Fatalf("Decrypt() = %q, %v", decrypted, err)
	}

	encrypted[len(encrypted)-1] ^= 1
	if _, err := cipher.Decrypt(encrypted); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}
