package identity

import (
	"strings"
	"testing"
)

func TestPasswordHashIsSaltedAndBounded(t *testing.T) {
	first, err := HashPassword("a long synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("a long synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !VerifyPassword(first, "a long synthetic password") || VerifyPassword(first, "another password") {
		t.Fatal("Argon2id-Salt oder Verifikation fehlerhaft")
	}
	if VerifyPassword(strings.Replace(first, "m=19456", "m=999999", 1), "a long synthetic password") {
		t.Fatal("Unbegrenzte Argon2-Parameter wurden akzeptiert")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("Kurzes Passwort wurde akzeptiert")
	}
}
