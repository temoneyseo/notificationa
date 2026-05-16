package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const SignatureHeader = "X-Notification-Signature"

func SignHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func VerifyHMACSHA256(secret string, body []byte, signature string) bool {
	expected := SignHMACSHA256(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}
