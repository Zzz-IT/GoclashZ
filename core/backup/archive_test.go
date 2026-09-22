//go:build windows

package backup

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestNormalizeBackupEntryExtendedCompatibility(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		kind     string
	}{
		{"offline_nodes.json", "offline_nodes.json", "nodes"},
		{"index.json", "profiles/index.json", "profiles"},
		{"subscriptions/index.json", "profiles/index.json", "profiles"},
		{"behavior.json", "Settings/user_behavior.json", "settings"},
		{"user_behavior.json", "Settings/user_behavior.json", "settings"},
		{"dns.json", "Settings/user_dns.json", "settings"},
		{"user_dns.json", "Settings/user_dns.json", "settings"},
		{"network.json", "Settings/user_network.json", "settings"},
		{"user_network.json", "Settings/user_network.json", "settings"},
		{"tun.json", "Settings/user_tun.json", "settings"},
		{"user_tun.json", "Settings/user_tun.json", "settings"},
		{"config.yaml", "config.yaml", "config"},
		{"theme_setting.txt", "theme_setting.txt", "theme"},
		{"manifest.json", "manifest.json", "manifest"},
		{"Subscriptions/sub1.yaml", "Subscriptions/sub1.yaml", "subs"},
		{"profiles/index.json", "profiles/index.json", "profiles"},
	}

	for _, tc := range cases {
		dest, kind, ok := normalizeBackupEntry(tc.input)
		if !ok {
			t.Fatalf("expected %q to be accepted", tc.input)
		}
		if dest != tc.expected || kind != tc.kind {
			t.Fatalf("normalizeBackupEntry(%q) = dest:%q, kind:%q; want dest:%q, kind:%q", tc.input, dest, kind, tc.expected, tc.kind)
		}
	}
}

func TestValidateRestorePlanInputsOptionalFiles(t *testing.T) {
	tempDir := t.TempDir()

	// In "all" mode, required dirs: Settings, Subscriptions, profiles
	_ = os.MkdirAll(filepath.Join(tempDir, "Settings"), 0755)
	_ = os.MkdirAll(filepath.Join(tempDir, "Subscriptions"), 0755)
	_ = os.MkdirAll(filepath.Join(tempDir, "profiles"), 0755)

	plan := buildRestorePlan("all")

	// Notice: config.yaml, theme_setting.txt, and offline_nodes.json do NOT exist in tempDir
	err := validateRestorePlanInputs(tempDir, plan, "all")
	if err != nil {
		t.Fatalf("validateRestorePlanInputs should pass when optional files are missing, got: %v", err)
	}
}

func TestRebuildIndexFromSubscriptionsSkipsSubdirsAndWritesIndex(t *testing.T) {
	stagingDir := t.TempDir()
	subDir := filepath.Join(stagingDir, "Subscriptions")
	_ = os.MkdirAll(subDir, 0755)

	// Create root subscription yaml
	_ = os.WriteFile(filepath.Join(subDir, "my_sub.yaml"), []byte("proxies: []"), 0644)

	// Create origin subdir with duplicate yaml name
	originDir := filepath.Join(subDir, "origin")
	_ = os.MkdirAll(originDir, 0755)
	_ = os.WriteFile(filepath.Join(originDir, "my_sub.yaml"), []byte("proxies: []"), 0644)

	items, err := rebuildIndexFromSubscriptions(stagingDir)
	if err != nil {
		t.Fatalf("rebuildIndexFromSubscriptions failed: %v", err)
	}

	// Should only have 1 item from root, NOT origin
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(items), items)
	}
	if items[0].ID != "my_sub" {
		t.Fatalf("expected ID 'my_sub', got %q", items[0].ID)
	}

	// Should have written stagingDir/profiles/index.json
	profilesIndex := filepath.Join(stagingDir, "profiles", "index.json")
	if _, err := os.Stat(profilesIndex); err != nil {
		t.Fatalf("expected profiles/index.json to be created on disk, err: %v", err)
	}
}
