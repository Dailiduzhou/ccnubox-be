package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

const testKey = "muxiStudioSecret"

func TestEncryptDecrypt(t *testing.T) {
	cryptoClient, err := NewCrypto(testKey)
	if err != nil {
		t.Fatal(err)
	}

	first, err := cryptoClient.Encrypt("password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cryptoClient.Encrypt("password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("Encrypt() reused a nonce")
	}
	if cryptoClient.NeedsMigration(first) {
		t.Fatal("current ciphertext unexpectedly needs migration")
	}

	plaintext, err := cryptoClient.Decrypt(first)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "password" {
		t.Fatalf("Decrypt() = %q, want password", plaintext)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	cryptoClient, err := NewCrypto(testKey)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cryptoClient.Encrypt("password")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := base64.StdEncoding.DecodeString(encrypted[len(currentCiphertextPrefix):])
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	tampered := currentCiphertextPrefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := cryptoClient.Decrypt(tampered); err == nil {
		t.Fatal("Decrypt() accepted tampered ciphertext")
	}
}

func TestDecryptLegacyCFB(t *testing.T) {
	cryptoClient, err := NewCrypto(testKey)
	if err != nil {
		t.Fatal(err)
	}
	legacy := encryptLegacyCFB(t, []byte(testKey), "password")
	if !cryptoClient.NeedsMigration(legacy) {
		t.Fatal("legacy ciphertext should need migration")
	}

	plaintext, err := cryptoClient.Decrypt(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "password" {
		t.Fatalf("Decrypt() = %q, want password", plaintext)
	}
}

func encryptLegacyCFB(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		t.Fatal(err)
	}
	//lint:ignore SA1019 this helper creates a legacy compatibility fixture
	cipher.NewCFBEncrypter(block, iv).XORKeyStream(ciphertext[aes.BlockSize:], []byte(plaintext))
	return base64.StdEncoding.EncodeToString(ciphertext)
}
