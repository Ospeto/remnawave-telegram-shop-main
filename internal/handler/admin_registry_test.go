package handler

import "testing"

func TestPublicBotCommandsDefaultToEnglish(t *testing.T) {
	commands := PublicBotCommands("my-MM")
	if len(commands) != 3 {
		t.Fatalf("PublicBotCommands() len = %d, want 3", len(commands))
	}
	if commands[0].Command != "start" || commands[0].Description != "Open the main menu" {
		t.Fatalf("PublicBotCommands() first command = %#v", commands[0])
	}
}

func TestAdminBotCommandsIncludeDashboard(t *testing.T) {
	commands := AdminBotCommands("ru")
	if len(commands) != 3 {
		t.Fatalf("AdminBotCommands() len = %d, want 3", len(commands))
	}
	if commands[0].Command != "admin" {
		t.Fatalf("AdminBotCommands() first command = %q, want admin", commands[0].Command)
	}
	if commands[0].Description == "" {
		t.Fatal("AdminBotCommands() admin description is empty")
	}
	if commands[1].Command != "healthbot" || commands[2].Command != "help" {
		t.Fatalf("AdminBotCommands() = %#v, want admin/healthbot/help", commands)
	}
}

func TestBuildAdminInputCommand(t *testing.T) {
	tests := []struct {
		name    string
		flow    adminFlowState
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "referral bonus",
			flow:  adminFlowState{Kind: adminFlowReferralBonus},
			input: "2500",
			want:  "/setreferralbonus 2500",
		},
		{
			name:  "set phone",
			flow:  adminFlowState{Kind: adminFlowSetPhone, Provider: "kpay"},
			input: "09123456789",
			want:  "/setphone kpay 09123456789",
		},
		{
			name:  "set name",
			flow:  adminFlowState{Kind: adminFlowSetName, Provider: "wavepay"},
			input: "Aung Aung",
			want:  "/setname wavepay Aung Aung",
		},
		{
			name:  "backup schedule",
			flow:  adminFlowState{Kind: adminFlowBackupSchedule},
			input: "00:15",
			want:  "/backup schedule 00:15",
		},
		{
			name:  "notify",
			flow:  adminFlowState{Kind: adminFlowNotify},
			input: "12345678",
			want:  "/notify 12345678",
		},
		{
			name:  "add promo",
			flow:  adminFlowState{Kind: adminFlowAddPromo},
			input: "sale50 50% 10days 100code",
			want:  "/addpromo sale50 50% 10days 100code",
		},
		{
			name:  "delete promo",
			flow:  adminFlowState{Kind: adminFlowDeletePromo},
			input: "SALE50",
			want:  "/deletepromo SALE50",
		},
		{
			name:    "invalid schedule",
			flow:    adminFlowState{Kind: adminFlowBackupSchedule},
			input:   "25:00",
			wantErr: true,
		},
		{
			name:    "invalid notify",
			flow:    adminFlowState{Kind: adminFlowNotify},
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "invalid add promo",
			flow:    adminFlowState{Kind: adminFlowAddPromo},
			input:   "sale50 50%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAdminInputCommand(tt.flow, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildAdminInputCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("buildAdminInputCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
