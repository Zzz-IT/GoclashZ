//go:build windows

package sys

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

type UwpApp struct {
	DisplayName       string `json:"displayName"`
	PackageFamilyName string `json:"packageFamilyName"`
	SID               string `json:"sid"`
	IsEnabled         bool   `json:"isEnabled"`
}

// GetUwpAppList 利用注册表极速获取 UWP 列表，并结合 CheckNetIsolation 获取状态
func GetUwpAppList() ([]UwpApp, error) {
	// 1. 获取当前已豁免的 SID 列表 (官方命令输出)
	exemptedSids, err := getExemptedSids()
	if err != nil {
		return nil, err
	}

	// 2. 从注册表枚举所有 UWP 映射
	// 路径: HKEY_CLASSES_ROOT\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings
	const mappingKey = `Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings`
	k, err := registry.OpenKey(registry.CLASSES_ROOT, mappingKey, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("无法读取注册表映射: %v", err)
	}
	defer k.Close()

	sids, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}

	var apps []UwpApp
	for _, sid := range sids {
		subKey, err := registry.OpenKey(registry.CLASSES_ROOT, mappingKey+`\`+sid, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		
		moniker, _, _ := subKey.GetStringValue("Moniker")
		displayName, _, _ := subKey.GetStringValue("DisplayName")
		subKey.Close()

		if moniker == "" {
			continue
		}

		// 某些 DisplayName 是资源字符串 (@{...})，如果为空或格式不对则使用 Moniker
		finalName := displayName
		if finalName == "" || strings.HasPrefix(finalName, "@") {
			finalName = moniker
		}

		apps = append(apps, UwpApp{
			DisplayName:       finalName,
			PackageFamilyName: moniker,
			SID:               sid,
			IsEnabled:         exemptedSids[sid],
		})
	}

	return apps, nil
}

// getExemptedSids 解析 CheckNetIsolation.exe LoopbackExempt -s 的结果
func getExemptedSids() (map[string]bool, error) {
	exemptMap := make(map[string]bool)
	cmd := exec.Command("CheckNetIsolation.exe", "LoopbackExempt", "-s")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	
	output, err := cmd.Output()
	if err != nil {
		return exemptMap, nil // 即使失败也返回空表，不阻塞主流程
	}

	// 遍历输出，寻找类似 SID: S-1-15-... 的行
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "SID:") || strings.Contains(line, "S-1-15-") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "S-1-15-") {
					exemptMap[p] = true
				}
			}
		}
	}
	return exemptMap, nil
}

// SaveUwpExemptions 批量保存豁免（增量更新版）
func SaveUwpExemptions(targetSids []string) error {
	// 1. 获取当前系统已有的豁免列表
	currentExempted, err := getExemptedSids()
	if err != nil {
		return err
	}

	// 2. 将目标列表转为 Map 方便查询
	targetMap := make(map[string]bool)
	for _, sid := range targetSids {
		targetMap[sid] = true
	}

	// 3. 增量删除：存在于系统但不在目标列表中的
	for sid := range currentExempted {
		if !targetMap[sid] {
			cmd := exec.Command("CheckNetIsolation.exe", "LoopbackExempt", "-d", "-p="+sid)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = cmd.Run()
		}
	}

	// 4. 增量添加：存在于目标列表但不在系统中的
	for sid := range targetMap {
		if !currentExempted[sid] {
			cmd := exec.Command("CheckNetIsolation.exe", "LoopbackExempt", "-a", "-p="+sid)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = cmd.Run()
		}
	}

	return nil
}


// ExemptAllUWP 兼容旧接口：一键豁免所有应用
func ExemptAllUWP() error {
	apps, err := GetUwpAppList()
	if err != nil {
		return err
	}
	
	var sids []string
	for _, app := range apps {
		sids = append(sids, app.SID)
	}
	
	return SaveUwpExemptions(sids)
}
