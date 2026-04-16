package utils

import "testing"

func TestMaskTail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		visible int
		want    string
	}{
		{name: "empty", input: "", visible: 4, want: ""},
		{name: "short", input: "123", visible: 4, want: "***"},
		{name: "phone", input: "09123456789", visible: 4, want: "*******6789"},
		{name: "transaction", input: "01004063070995016447", visible: 6, want: "**************016447"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskTail(tt.input, tt.visible); got != tt.want {
				t.Fatalf("MaskTail(%q, %d) = %q, want %q", tt.input, tt.visible, got, tt.want)
			}
		})
	}
}

func TestFirstToken(t *testing.T) {
	if got := FirstToken("/setreferralbonus 2000"); got != "/setreferralbonus" {
		t.Fatalf("FirstToken() = %q, want /setreferralbonus", got)
	}
	if got := FirstToken("   "); got != "" {
		t.Fatalf("FirstToken() empty input = %q, want empty string", got)
	}
}
