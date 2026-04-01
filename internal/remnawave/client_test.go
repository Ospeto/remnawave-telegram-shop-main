package remnawave

import "testing"

func TestGenerateUsernameUsesMoreThanLastFourTxnChars(t *testing.T) {
	// These transaction IDs share the same last 4 chars.
	// Username generation must still avoid collisions.
	u1 := generateUsername("", 1, 123456789, 0, "AAAA0001")
	u2 := generateUsername("", 1, 123456789, 0, "BBBB0001")

	if u1 == u2 {
		t.Fatalf("generateUsername() collision: %q == %q", u1, u2)
	}
}
