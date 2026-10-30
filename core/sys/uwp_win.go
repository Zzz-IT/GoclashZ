//go:build windows

package sys

import (
	"fmt"
	"goclashz/core/utils"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
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

// 预编译正则，匹配 UWP 的标准 SID 格式
var uwpSidRegex = regexp.MustCompile(`S-1-15-[-0-9]+`)

// getExemptedSids 解析 CheckNetIsolation.exe LoopbackExempt -s 的结果
func getExemptedSids() (map[string]bool, error) {
	exemptMap := make(map[string]bool)
	// 枚举同样走固定系统路径，避免 PATH 劫持伪造「已豁免」结果
	cmd := exec.Command(checkNetIsolationPath(), "LoopbackExempt", "-s")
	utils.HideCommandWindow(cmd, 0)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// 🔒 枚举失败必须报错：返回空表会让调用方误以为「系统里没有任何豁免」，
		// 进而在保存时把真实存在的豁免项当成「用户关掉了」全部删掉。
		message := strings.TrimSpace(string(output))
		if message != "" {
			return nil, fmt.Errorf("读取现有 UWP 豁免列表失败: %s", message)
		}
		return nil, fmt.Errorf("读取现有 UWP 豁免列表失败: %w", err)
	}

	// ✅ 直接使用正则从所有输出内容中暴力提取符合 UWP SID 规范的字符串
	// 这样可以完全无视 CheckNetIsolation 的输出语言（中/英/俄等）
	matches := uwpSidRegex.FindAllString(string(output), -1)
	for _, sid := range matches {
		exemptMap[sid] = true
	}

	return exemptMap, nil
}

// uwpExemptionConcurrency CheckNetIsolation 子进程的并发上限。
// 原来是 10：并发过高时多个 CheckNetIsolation 会互相抢占（表现为大量假失败），这里降到 3。
const uwpExemptionConcurrency = 3

// uwpExemptionRetries 单个 SID 失败后的重试次数（1 = 失败后再试一次）
const uwpExemptionRetries = 1

// uwpLoopbackChange 一次 CheckNetIsolation 增删动作
type uwpLoopbackChange struct {
	add bool   // true=添加豁免，false=取消豁免
	sid string // UWP AppContainer SID
}

// planUwpExemptionChanges 计算本次需要增删的 SID。
// 🔒 安全约束：只有「本次枚举到的应用」才允许进入 removed。
// 系统里已经豁免、但没出现在枚举结果里的 SID（其它工具/其它账号添加的、注册表映射里没有的）
// 一律不碰，避免一次保存把它们全部删掉。
func planUwpExemptionChanges(targetSids []string, apps []UwpApp) (added []string, removed []string) {
	targetSet := make(map[string]bool, len(targetSids))
	for _, sid := range targetSids {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		targetSet[sid] = true
	}

	enumerated := make(map[string]bool, len(apps))
	exempted := make(map[string]bool, len(apps))
	for _, app := range apps {
		enumerated[app.SID] = true
		if app.IsEnabled {
			exempted[app.SID] = true
		}
	}

	for sid := range exempted {
		if enumerated[sid] && !targetSet[sid] {
			removed = append(removed, sid)
		}
	}
	for sid := range targetSet {
		if !exempted[sid] {
			added = append(added, sid)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// SaveUwpExemptions 批量保存豁免（差异应用版，前端仍在调用这个入口）。
// 语义收紧：先完整枚举一次应用列表，只删除「既不在 targetSids、又确实出现在本次枚举里」的 SID；
// 枚举失败直接返回错误，绝不返回空表后继续执行删除。
func SaveUwpExemptions(targetSids []string) error {
	if !CheckAdmin() {
		return fmt.Errorf("修改 UWP 环回免除规则需要管理员权限")
	}

	apps, err := GetUwpAppList()
	if err != nil {
		return fmt.Errorf("枚举 UWP 应用失败: %w", err)
	}

	added, removed := planUwpExemptionChanges(targetSids, apps)
	return ApplyUwpExemptionChanges(added, removed)
}

// ApplyUwpExemptionChanges 只按传入的差异执行增删，绝不枚举、也绝不推断系统里还有哪些豁免。
// 调用方没点名的 SID 一个都不会动；有失败时聚合返回（带第一个失败原因）。
func ApplyUwpExemptionChanges(added []string, removed []string) error {
	changes, err := buildUwpLoopbackChanges(added, removed)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}

	if !CheckAdmin() {
		return fmt.Errorf("修改 UWP 环回免除规则需要管理员权限")
	}

	return runUwpLoopbackChanges(changes)
}

// buildUwpLoopbackChanges 校验并展开成待执行动作。非法 SID 直接报错，
// 不把不可信的字符串拼进系统命令参数。
func buildUwpLoopbackChanges(added []string, removed []string) ([]uwpLoopbackChange, error) {
	addedSet := make(map[string]bool, len(added))
	removedSet := make(map[string]bool, len(removed))

	for _, sid := range added {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		if !isValidUwpSid(sid) {
			return nil, fmt.Errorf("非法的 UWP SID: %s", sid)
		}
		addedSet[sid] = true
	}
	for _, sid := range removed {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		if !isValidUwpSid(sid) {
			return nil, fmt.Errorf("非法的 UWP SID: %s", sid)
		}
		removedSet[sid] = true
	}

	changes := make([]uwpLoopbackChange, 0, len(addedSet)+len(removedSet))
	for sid := range addedSet {
		if removedSet[sid] {
			// 同一个 SID 既加又删属于调用方出错：直接拒绝，避免「先加后删」的竞态结果
			return nil, fmt.Errorf("SID 同时出现在新增与删除列表: %s", sid)
		}
		changes = append(changes, uwpLoopbackChange{add: true, sid: sid})
	}
	for sid := range removedSet {
		changes = append(changes, uwpLoopbackChange{add: false, sid: sid})
	}

	// map 遍历顺序随机，排序后测试与日志可复现（添加在前、删除在后）
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].add != changes[j].add {
			return changes[i].add
		}
		return changes[i].sid < changes[j].sid
	})

	return changes, nil
}

// runUwpLoopbackChanges 用固定大小工作池执行动作：并发上限 uwpExemptionConcurrency，
// 单个失败重试 uwpExemptionRetries 次，最后聚合返回第一个失败原因。
func runUwpLoopbackChanges(changes []uwpLoopbackChange) error {
	exe := checkNetIsolationPath()

	workers := uwpExemptionConcurrency
	if len(changes) < workers {
		workers = len(changes)
	}

	jobs := make(chan uwpLoopbackChange)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	var firstErr error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for change := range jobs {
				if err := runUwpLoopbackChange(exe, change); err != nil {
					mu.Lock()
					failed++
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}

	for _, change := range changes {
		jobs <- change
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return fmt.Errorf("UWP 豁免有 %d 个操作失败（首个失败: %v）", failed, firstErr)
	}
	return nil
}

// runUwpLoopbackChange 执行单个 SID 的增删；失败后等一下再试一次
func runUwpLoopbackChange(exe string, change uwpLoopbackChange) error {
	action := "-d"
	label := "取消豁免"
	if change.add {
		action = "-a"
		label = "添加豁免"
	}

	var lastErr error
	for attempt := 0; attempt <= uwpExemptionRetries; attempt++ {
		cmd := exec.Command(exe, "LoopbackExempt", action, "-p="+change.sid)
		utils.HideCommandWindow(cmd, 0)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}

		message := strings.TrimSpace(string(output))
		if message != "" {
			lastErr = fmt.Errorf("%s %s 失败: %s", label, change.sid, message)
		} else {
			lastErr = fmt.Errorf("%s %s 失败: %w", label, change.sid, err)
		}

		if attempt < uwpExemptionRetries {
			time.Sleep(150 * time.Millisecond)
		}
	}

	return lastErr
}

// checkNetIsolationPath 固定使用系统目录下的可执行文件，防路径劫持；找不到时退回 PATH 查找
func checkNetIsolationPath() string {
	if root := strings.TrimSpace(os.Getenv("SystemRoot")); root != "" {
		candidate := filepath.Join(root, "System32", "CheckNetIsolation.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "CheckNetIsolation.exe"
}

// isValidUwpSid 判断是否是标准 UWP AppContainer SID（S-1-15-...）
func isValidUwpSid(sid string) bool {
	return sid != "" && uwpSidRegex.FindString(sid) == sid
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
