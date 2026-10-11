package clash

import (
	"testing"
)

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		input   string
		want    [3]int
		wantErr bool
	}{
		{"v1.19.31", [3]int{1, 19, 31}, false},
		{"V1.19.27", [3]int{1, 19, 27}, false},
		{"1.19.0", [3]int{1, 19, 0}, false},
		{"v1.19.31-alpha", [3]int{1, 19, 31}, false},
		{"v1.19.31+meta", [3]int{1, 19, 31}, false},
		{"", [3]int{0, 0, 0}, true},
		{"invalid", [3]int{0, 0, 0}, true},
		{"已安装，版本未知", [3]int{0, 0, 0}, true},
	}

	for _, tc := range tests {
		got, err := ParseVersionParts(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseVersionParts(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseVersionParts(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCompareCoreVersion(t *testing.T) {
	tests := []struct {
		seed    string
		current string
		wantCmp int
	}{
		{"v1.19.31", "v1.19.27", 1},  // seed > current: current is lower
		{"v1.19.27", "v1.19.31", -1}, // seed < current: current is higher
		{"v1.19.31", "v1.19.31", 0},  // seed == current: equal
	}

	for _, tc := range tests {
		got, err := CompareCoreVersion(tc.seed, tc.current)
		if err != nil {
			t.Errorf("CompareCoreVersion(%q, %q) unexpected error: %v", tc.seed, tc.current, err)
			continue
		}
		if got != tc.wantCmp {
			t.Errorf("CompareCoreVersion(%q, %q) = %d, want %d", tc.seed, tc.current, got, tc.wantCmp)
		}
	}
}

func TestAdjustTunConfigForCore(t *testing.T) {
	// 1. nil config
	if AdjustTunConfigForCore(nil, false) {
		t.Errorf("AdjustTunConfigForCore(nil, false) should be false")
	}

	// 2. mips when core does not support mips -> should downgrade
	cfg1 := &TunConfig{
		Stack:                "mips",
		CongestionController: "bbr3",
	}
	if !AdjustTunConfigForCore(cfg1, false) {
		t.Errorf("AdjustTunConfigForCore with mips on unsupported core should return true")
	}
	if cfg1.Stack != "mixed" {
		t.Errorf("expected stack to be 'mixed', got %q", cfg1.Stack)
	}
	if cfg1.CongestionController != "" {
		t.Errorf("expected congestion controller to be cleared, got %q", cfg1.CongestionController)
	}

	// 3. case insensitive and whitespace
	cfg2 := &TunConfig{
		Stack:                " MIPS ",
		CongestionController: "bbr3",
	}
	if !AdjustTunConfigForCore(cfg2, false) {
		t.Errorf("AdjustTunConfigForCore with ' MIPS ' on unsupported core should return true")
	}
	if cfg2.Stack != "mixed" {
		t.Errorf("expected stack to be 'mixed', got %q", cfg2.Stack)
	}

	// 4. mips when core DOES support mips -> should NOT downgrade
	cfg3 := &TunConfig{
		Stack:                "mips",
		CongestionController: "bbr3",
	}
	if AdjustTunConfigForCore(cfg3, true) {
		t.Errorf("AdjustTunConfigForCore with mips on supported core should return false")
	}
	if cfg3.Stack != "mips" {
		t.Errorf("expected stack to remain 'mips', got %q", cfg3.Stack)
	}
	if cfg3.CongestionController != "bbr3" {
		t.Errorf("expected congestion controller to remain 'bbr3', got %q", cfg3.CongestionController)
	}

	// 5. non-mips stacks -> should NOT downgrade
	cfg4 := &TunConfig{
		Stack: "gvisor",
	}
	if AdjustTunConfigForCore(cfg4, false) {
		t.Errorf("AdjustTunConfigForCore with gvisor should return false")
	}
	if cfg4.Stack != "gvisor" {
		t.Errorf("expected stack to remain 'gvisor', got %q", cfg4.Stack)
	}
}
