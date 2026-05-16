package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

var sensitiveConfigFields = map[string]struct{}{
	"bot_token":      {},
	"token":          {},
	"webhook_secret": {},
	"secret":         {},
	"client_secret":  {},
}

type Cipher struct {
	aead cipher.AEAD
}

func NewCipherFromEnv() (*Cipher, error) {
	return NewCipher(os.Getenv("ENCRYPTION_KEY"))
}

func NewCipher(key string) (*Cipher, error) {
	raw := []byte(key)
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) > 0 {
		raw = decoded
	}
	switch len(raw) {
	case 16, 24, 32:
	default:
		return nil, errors.New("ENCRYPTION_KEY must be 16, 24, or 32 bytes, optionally base64 encoded")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) EncryptString(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, "enc:") {
		return plain, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return "enc:" + base64.StdEncoding.EncodeToString(payload), nil
}

func (c *Cipher) DecryptString(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	if !strings.HasPrefix(encrypted, "enc:") {
		return encrypted, nil
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, "enc:"))
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("encrypted payload too short")
	}
	plain, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (c *Cipher) EncryptConfig(config map[string]any) (map[string]any, error) {
	return c.transformConfig(config, c.EncryptString)
}

func (c *Cipher) DecryptConfig(config map[string]any) (map[string]any, error) {
	return c.transformConfig(config, c.DecryptString)
}

func MaskConfig(config map[string]any) map[string]any {
	masked := copyMap(config)
	for key := range masked {
		if isSensitiveField(key) && masked[key] != "" {
			masked[key] = "********"
		}
	}
	return masked
}

func (c *Cipher) transformConfig(config map[string]any, transform func(string) (string, error)) (map[string]any, error) {
	out := copyMap(config)
	for key, value := range out {
		if !isSensitiveField(key) {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		next, err := transform(text)
		if err != nil {
			return nil, err
		}
		out[key] = next
	}
	return out, nil
}

func copyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func isSensitiveField(key string) bool {
	_, ok := sensitiveConfigFields[strings.ToLower(key)]
	return ok
}
