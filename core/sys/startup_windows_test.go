//go:build windows

package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCommandLine(t *testing.T) {
	cmd := `"C:\Program Files\GoclashZ\GoclashZ.exe" --startup --silent --data-dir "C:\Users\Test\AppData\Local\GoclashZ"`
	exe, args, argList := parseCommandLine(cmd)

	if exe != `C:\Program Files\GoclashZ\GoclashZ.exe` {
		t.Errorf("expected exe 'C:\\Program Files\\GoclashZ\\GoclashZ.exe', got '%s'", exe)
	}

	if len(argList) != 4 {
		t.Errorf("expected 4 arguments in argList, got %d: %v", len(argList), argList)
	}

	dataDir := extractArgValue(argList, "--data-dir")
	if dataDir != `C:\Users\Test\AppData\Local\GoclashZ` {
		t.Errorf("expected dataDir 'C:\\Users\\Test\\AppData\\Local\\GoclashZ', got '%s'", dataDir)
	}

	if args == "" {
		t.Errorf("expected args string to be populated")
	}
}

func TestRegistryStartupLifecycle(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get executable: %v", err)
	}
	absExe, _ := filepath.Abs(exe)

	// 1. 测试创建自启动项
	err = CreateStartupTask(absExe)
	if err != nil {
		t.Fatalf("CreateStartupTask failed: %v", err)
	}

	// 2. 测试检查自启动项状态
	info, err := CheckStartupTask()
	if err != nil {
		t.Fatalf("CheckStartupTask failed: %v", err)
	}
	if !info.Exists {
		t.Errorf("expected info.Exists to be true")
	}
	if !info.Enabled {
		t.Errorf("expected info.Enabled to be true")
	}
	if !info.IsHealthy {
		t.Errorf("expected info.IsHealthy to be true, lastError: %s", info.LastError)
	}

	// 3. 测试删除自启动项
	err = DeleteStartupTask()
	if err != nil {
		t.Fatalf("DeleteStartupTask failed: %v", err)
	}

	// 4. 测试删除后再检查
	infoAfter, err := CheckStartupTask()
	if err != nil {
		t.Fatalf("CheckStartupTask after delete failed: %v", err)
	}
	if infoAfter.Exists {
		t.Errorf("expected infoAfter.Exists to be false after deletion")
	}
}
