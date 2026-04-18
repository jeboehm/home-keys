package main

import "testing"

func TestGenerateAndValidate(t *testing.T) {
	secret := []byte("test-secret-at-least-16-chars")
	code := "1234"

	token, err := GenerateSessionToken(secret, code)
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if !ValidateSessionToken(secret, token, code) {
		t.Error("valid token rejected")
	}
}

func TestValidate_WrongCode(t *testing.T) {
	secret := []byte("test-secret-at-least-16-chars")
	token, _ := GenerateSessionToken(secret, "1234")
	if ValidateSessionToken(secret, token, "9999") {
		t.Error("token with wrong code should be invalid")
	}
}

func TestValidate_MalformedToken(t *testing.T) {
	secret := []byte("test-secret-at-least-16-chars")
	if ValidateSessionToken(secret, "nodothere", "1234") {
		t.Error("malformed token should be invalid")
	}
}
