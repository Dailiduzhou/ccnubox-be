package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const currentCiphertextPrefix = "gcm:v1:"

type Crypto struct {
	key []byte // 用于加密解密的密钥
}

// NewCrypto 创建一个新的 Crypto 实例，key 必须是 16, 24, 或 32 字节长度（对应 AES-128, AES-192, AES-256）
func NewCrypto(key string) (*Crypto, error) {
	keyLen := len(key)
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		return nil, errors.New("密钥长度必须是 16, 24, 或 32 字节")
	}
	return &Crypto{key: []byte(key)}, nil
}

// Encrypt 加密明文并返回 Base64 编码的密文
func (c *Crypto) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)

	return currentCiphertextPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Base64 编码的密文并返回明文
func (c *Crypto) Decrypt(encodedCiphertext string) (string, error) {
	if strings.HasPrefix(encodedCiphertext, currentCiphertextPrefix) {
		return c.decryptGCM(strings.TrimPrefix(encodedCiphertext, currentCiphertextPrefix))
	}

	// 兼容历史 AES-CFB 密文，用户下次保存密码时会迁移到 GCM。
	return c.decryptLegacyCFB(encodedCiphertext)
}

func (c *Crypto) NeedsMigration(encodedCiphertext string) bool {
	return !strings.HasPrefix(encodedCiphertext, currentCiphertextPrefix)
}

func (c *Crypto) decryptGCM(encodedCiphertext string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < aead.NonceSize() {
		return "", errors.New("密文长度不足")
	}

	nonce := ciphertext[:aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, ciphertext[aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("密文认证失败")
	}
	return string(plaintext), nil
}

func (c *Crypto) decryptLegacyCFB(encodedCiphertext string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < aes.BlockSize {
		return "", errors.New("密文长度不足")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	//lint:ignore SA1019 compatibility is required until all stored CFB ciphertexts are migrated
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return string(ciphertext), nil
}
