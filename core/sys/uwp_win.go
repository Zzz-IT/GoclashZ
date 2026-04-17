//go:build windows

package sys

import (
	"fmt"
	"os/exec"
	"syscall"
)

// ExemptAllUWP 解除所有 UWP 应用的本地回环网络限制
func ExemptAllUWP() error {
	// 核心原理：通过 PowerShell 遍历所有已安装的 UWP 容器，并使用 CheckNetIsolation 注入豁免规则
	psScript := `
		$apps = Get-AppxPackage
		foreach ($app in $apps) {
			$family = $app.PackageFamilyName
			if ($family) {
				CheckNetIsolation.exe LoopbackExempt -a -n="$family" | Out-Null
			}
		}
	`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)

	// 隐藏执行窗口，防止弹黑框闪烁
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("UWP 环回免除执行失败: %v", err)
	}

	fmt.Println("✅ 已成功为所有 UWP 应用添加本地回环免除规则")
	return nil
}
