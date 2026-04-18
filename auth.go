package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

const sessionCookieName = "session"

// GenerateSessionToken returns a hex(random_32_bytes).hex(HMAC-SHA256) token.
// The signing key is derived from secret+code so changing code invalidates all sessions.
func GenerateSessionToken(secret []byte, code string) (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	payloadHex := hex.EncodeToString(payload)
	sig := computeHMAC(deriveKey(secret, code), payloadHex)
	return payloadHex + "." + sig, nil
}

func ValidateSessionToken(secret []byte, cookieValue string, code string) bool {
	parts := strings.SplitN(cookieValue, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expected := computeHMAC(deriveKey(secret, code), parts[0])
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// deriveKey binds the session signing key to the current code value.
// When code changes, all previously issued tokens become invalid.
func deriveKey(secret []byte, code string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func computeHMAC(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, sessionCookie(token, 86400*2))
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, sessionCookie("", -1))
}

func sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	}
}
