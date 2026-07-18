//go:build windows

package backup

import "testing"

func TestNormalizeBackupEntryDesiredStateCompatibility(t *testing.T) {
	tests := []string{
		"desired_state.json",
		"user_desired_state.json",
		"user_user_desired_state.json",
		"Settings/desired_state.json",
		"Settings/user_desired_state.json",
		"Settings/user_user_desired_state.json",
	}

	for _, input := range tests {
		dest, kind, ok := normalizeBackupEntry(input)
		if !ok {
			t.Fatalf("%q was not accepted", input)
		}
		if dest != "Settings/user_desired_state.json" || kind != "settings" {
			t.Fatalf("%q normalized to dest=%q kind=%q", input, dest, kind)
		}
	}
}
