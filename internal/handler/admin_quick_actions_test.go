package handler

import "testing"

func TestAdminQuickActionCommandMapping(t *testing.T) {
	tests := []struct {
		label   string
		wantCmd string
	}{
		{label: adminQuickRevenue, wantCmd: "/revenue"},
		{label: adminQuickTransactions, wantCmd: "/transactions"},
		{label: adminQuickProviders, wantCmd: "/phones"},
		{label: adminQuickSyncUsers, wantCmd: "/sync"},
		{label: adminQuickBackupStatus, wantCmd: "/backup status"},
		{label: adminQuickBackupList, wantCmd: "/backup list"},
		{label: adminQuickEnableTest, wantCmd: "/test enable"},
		{label: adminQuickDisableTest, wantCmd: "/test disable"},
		{label: adminQuickOpenDashboard, wantCmd: adminQuickActionDashboard},
		{label: adminQuickHelp, wantCmd: "/help"},
		{label: adminQuickHideKeyboard, wantCmd: adminQuickActionHide},
	}

	for _, tc := range tests {
		got, ok := adminQuickActionCommand(tc.label)
		if !ok {
			t.Fatalf("adminQuickActionCommand(%q) not recognized", tc.label)
		}
		if got != tc.wantCmd {
			t.Fatalf("adminQuickActionCommand(%q) = %q, want %q", tc.label, got, tc.wantCmd)
		}
	}
}

func TestIsAdminQuickAction(t *testing.T) {
	h := Handler{}

	if !h.IsAdminQuickAction(adminQuickRevenue) {
		t.Fatal("IsAdminQuickAction(adminQuickRevenue) = false, want true")
	}

	if h.IsAdminQuickAction("not a known action") {
		t.Fatal("IsAdminQuickAction(unknown) = true, want false")
	}
}
