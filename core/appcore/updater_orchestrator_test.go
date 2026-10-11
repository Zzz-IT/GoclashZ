package appcore

import (
	"testing"
)

func TestIsVersionUnknown(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"未安装", true},
		{"已安装，版本未知", true},
		{"版本未知", true},
		{"unknown", true},
		{"123.abc", true},
		{"v1.19.31", false},
		{"1.19.27", false},
		{"v1.19.0", false},
	}

	for _, tc := range tests {
		got := isVersionUnknown(tc.input)
		if got != tc.want {
			t.Errorf("isVersionUnknown(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEvaluateCoreComparison(t *testing.T) {
	tests := []struct {
		name                 string
		currentVer           string
		seedVer              string
		promptEnabled        bool
		wantShouldPrompt     bool
		wantIsCurrentLower   bool
		wantIsUnknown        bool
		wantNewPromptEnabled bool
	}{
		{
			name:                 "当前内核版本不明，不提示，自动关闭提示",
			currentVer:           "已安装，版本未知",
			seedVer:              "v1.19.31",
			promptEnabled:        true,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   false,
			wantIsUnknown:        true,
			wantNewPromptEnabled: false,
		},
		{
			name:                 "当前未安装内核，不提示，自动关闭提示",
			currentVer:           "未安装",
			seedVer:              "v1.19.31",
			promptEnabled:        true,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   false,
			wantIsUnknown:        true,
			wantNewPromptEnabled: false,
		},
		{
			name:                 "种子版本不明，无法对比，不提示",
			currentVer:           "v1.19.25",
			seedVer:              "",
			promptEnabled:        true,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   false,
			wantIsUnknown:        false,
			wantNewPromptEnabled: true,
		},
		{
			name:                 "当前版本大于种子版本，不提示",
			currentVer:           "v1.19.31",
			seedVer:              "v1.19.27",
			promptEnabled:        true,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   false,
			wantIsUnknown:        false,
			wantNewPromptEnabled: true,
		},
		{
			name:                 "当前版本等于种子版本，不提示",
			currentVer:           "v1.19.31",
			seedVer:              "v1.19.31",
			promptEnabled:        true,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   false,
			wantIsUnknown:        false,
			wantNewPromptEnabled: true,
		},
		{
			name:                 "当前版本低于种子版本，提示已开启 -> 弹出提示",
			currentVer:           "v1.19.25",
			seedVer:              "v1.19.31",
			promptEnabled:        true,
			wantShouldPrompt:     true,
			wantIsCurrentLower:   true,
			wantIsUnknown:        false,
			wantNewPromptEnabled: true,
		},
		{
			name:                 "当前版本低于种子版本，但用户已选择不再提示 -> 不提示",
			currentVer:           "v1.19.25",
			seedVer:              "v1.19.31",
			promptEnabled:        false,
			wantShouldPrompt:     false,
			wantIsCurrentLower:   true,
			wantIsUnknown:        false,
			wantNewPromptEnabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, newPromptEnabled := EvaluateCoreComparison(tc.currentVer, tc.seedVer, tc.promptEnabled)
			if res.ShouldPrompt != tc.wantShouldPrompt {
				t.Errorf("ShouldPrompt = %v, want %v", res.ShouldPrompt, tc.wantShouldPrompt)
			}
			if res.IsCurrentLower != tc.wantIsCurrentLower {
				t.Errorf("IsCurrentLower = %v, want %v", res.IsCurrentLower, tc.wantIsCurrentLower)
			}
			if res.IsUnknown != tc.wantIsUnknown {
				t.Errorf("IsUnknown = %v, want %v", res.IsUnknown, tc.wantIsUnknown)
			}
			if newPromptEnabled != tc.wantNewPromptEnabled {
				t.Errorf("newPromptEnabled = %v, want %v", newPromptEnabled, tc.wantNewPromptEnabled)
			}
		})
	}
}
