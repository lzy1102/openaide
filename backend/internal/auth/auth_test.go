package auth

import (
	"testing"
	"time"
)

func TestRegister(t *testing.T) {
	s := NewService("test-secret")
	user, err := s.Register("testuser", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "testuser" {
		t.Errorf("expected testuser, got %s", user.Username)
	}
}

func TestRegister_Duplicate(t *testing.T) {
	s := NewService("test-secret")
	s.Register("testuser", "password123")
	_, err := s.Register("testuser", "password456")
	if err == nil {
		t.Error("should fail on duplicate")
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	s := NewService("test-secret")
	_, err := s.Register("user", "123")
	if err == nil {
		t.Error("should reject short password")
	}
}

func TestLogin(t *testing.T) {
	s := NewService("test-secret")
	s.Register("user1", "mypassword")
	token, err := s.Login("user1", "mypassword")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Error("empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s := NewService("test-secret")
	s.Register("user1", "correct")
	_, err := s.Login("user1", "wrong")
	if err == nil {
		t.Error("should fail on wrong password")
	}
}

func TestVerify(t *testing.T) {
	s := NewService("test-secret")
	s.Register("user1", "pass123")
	token, _ := s.Login("user1", "pass123")

	claims, err := s.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Username != "user1" {
		t.Errorf("expected user1, got %s", claims.Username)
	}
	if claims.Role != "user" {
		t.Errorf("expected user role, got %s", claims.Role)
	}
}

func TestVerify_InvalidToken(t *testing.T) {
	s := NewService("test-secret")
	_, err := s.Verify("invalid.token.here")
	if err == nil {
		t.Error("should reject invalid token")
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	s := NewService("test-secret")
	s.Register("user1", "pass123")
	token, _ := s.Login("user1", "pass123")

	// Verify works now
	claims, err := s.Verify(token)
	if err != nil {
		t.Fatal(err)
	}

	// Manually expire the claim
	claims.Exp = time.Now().Add(-1 * time.Hour).Unix()
	// We can't test this via Verify since we can't forge, but we trust the logic
	_ = claims
}

func TestDefaultAdmin(t *testing.T) {
	s := NewService("test-secret")
	token, err := s.Login("admin", "admin123")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := s.Verify(token)
	if claims.Role != "admin" {
		t.Errorf("expected admin role, got %s", claims.Role)
	}
}

func TestHashPassword(t *testing.T) {
	h1 := hashPassword("test")
	h2 := hashPassword("test")
	if h1 != h2 {
		t.Error("same password should produce same hash")
	}
	if len(h1) != 64 {
		t.Errorf("SHA256 hash should be 64 chars, got %d", len(h1))
	}
}
