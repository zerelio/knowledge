package auth

import (
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple", "hunter2hunter2"},
		{"complex", "Tr0ub4dor&3-horse-battery"},
		{"unicode", "пароль密码パスワード"},
		{"empty", ""},
		{"long", "a very long input that exceeds typical length limits and goes on and on and on and on"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.input)
			if err != nil {
				t.Fatalf("HashPassword(%q) error: %v", tt.input, err)
			}

			if hash == "" {
				t.Fatal("hash should not be empty")
			}

			ok, err := VerifyPassword(tt.input, hash)
			if err != nil {
				t.Fatalf("VerifyPassword() error: %v", err)
			}
			if !ok {
				t.Error("VerifyPassword() should return true for correct input")
			}

			ok, err = VerifyPassword(tt.input+"wrong", hash)
			if err != nil {
				t.Fatalf("VerifyPassword() error: %v", err)
			}
			if ok {
				t.Error("VerifyPassword() should return false for wrong input")
			}
		})
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	hash1, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}

	hash2, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}

	if hash1 == hash2 {
		t.Error("two hashes of the same password should differ due to random salts")
	}
}
