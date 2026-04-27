package clash

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"goclashz/core/utils"

	"gopkg.in/yaml.v3"
)

// parseSubUserInfo 解析流量 Header
func parseSubUserInfo(header string) (upload, download, total, expire int64) {
	if header == "" {
		return
	}
	parts := strings.Split(header, ";")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			// 🚀 修复：增加极其严苛的空格清洗，防止某些机场面板的不规范下发导致数据丢失
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			valStr := strings.TrimSpace(kv[1])

			val, _ := strconv.ParseInt(valStr, 10, 64)
			switch key {
			case "upload":
				upload = val
			case "download":
				download = val
			case "total":
				total = val
			case "expire":
				expire = val
			}
		}
	}
	return
}

// DownloadSub 下载订阅 (id 为空表示新增，不为空表示更新)
func DownloadSub(ctx context.Context, name, url, existingId, userAgent string) (string, error) {
	id := existingId
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	client := &http.Client{}
	// 🛡️ 修复：必须加上硬超时！防止机场服务器假死导致前端 UI 的 loading 圈永远转下去
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return id, err
	}

	if userAgent == "" {
		userAgent = "clash-verge/1.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return id, fmt.Errorf("订阅下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return id, fmt.Errorf("订阅服务器异常: HTTP %d", resp.StatusCode)
	}

	upload, download, total, expire := parseSubUserInfo(resp.Header.Get("Subscription-Userinfo"))

	// ==========================================
	// 🛡️ 终极护城河：YAML 格式与结构语义极严校验
	// ==========================================
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return id, fmt.Errorf("读取订阅内容失败: %v", err)
	}

	if err := StrictVerifyClashConfig(bodyBytes); err != nil {
		// 直接将极严校验的详细报错抛出，阻断落盘
		return id, fmt.Errorf("订阅配置校验失败: %v", err)
	}
	// ==========================================

	// 🛡️ 防御路径穿越：提取纯文件名
	safeId := filepath.Base(filepath.Clean(id))
	if safeId == "." || safeId == "/" || safeId == "\\" {
		return id, fmt.Errorf("非法的文件 ID 拒绝访问")
	}

	// 校验通过，安全落盘
	yamlPath := filepath.Join(utils.GetSubscriptionsDir(), safeId+".yaml")
	if err := os.WriteFile(yamlPath, bodyBytes, 0644); err != nil {
		return safeId, fmt.Errorf("无法写入配置文件: %v", err)
	}

	// 4. 初始化伴生规则文件 (仅在第一次添加订阅时截取原始规则)
	rulesPath := filepath.Join(utils.GetSubscriptionsDir(), safeId+"_rules.json")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		rules, err := GetOriginalRules(safeId)
		if err != nil || len(rules) == 0 {
			rules = []string{"MATCH,DIRECT"}
		}
		SaveCustomRules(safeId, rules)
	}

	// 5. 更新全局索引
	IndexLock.Lock()
	found := false
	for i, item := range SubIndex {
		if item.ID == safeId {
			SubIndex[i].Upload = upload // 更新流量和时间
			SubIndex[i].Download = download
			SubIndex[i].Total = total
			SubIndex[i].Expire = expire
			SubIndex[i].Updated = time.Now().Unix()
			found = true
			break
		}
	}
	if !found {
		SubIndex = append(SubIndex, SubIndexItem{
			ID:       safeId,
			Name:     name,
			URL:      url,
			Type:     "remote",
			Upload:   upload,
			Download: download,
			Total:    total,
			Expire:   expire,
			Updated:  time.Now().Unix(),
		})
	}
	IndexLock.Unlock()

	return safeId, SaveIndex()
}

// RenameConfig 重命名配置文件
func RenameConfig(id, newName string) error {
	IndexLock.Lock()
	for i, item := range SubIndex {
		if item.ID == id {
			SubIndex[i].Name = newName
			break
		}
	}
	IndexLock.Unlock() // 👈 核心修复：必须在这里提前释放写锁，删掉原本的 defer

	return SaveIndex() // SaveIndex 内部会自己去申请 RLock，这样就不会死锁了
}

func DeleteConfig(id string) error {
	// 🛡️ 防御路径穿越：强行提取纯文件名
	safeId := filepath.Base(filepath.Clean(id))
	if safeId == "." || safeId == "/" || safeId == "\\" {
		return fmt.Errorf("非法的文件 ID 拒绝访问")
	}

	dir := utils.GetSubscriptionsDir()
	yamlPath := filepath.Join(dir, safeId+".yaml")
	rulesPath := filepath.Join(dir, safeId+"_rules.json")

	// 1. 🚀 核心修复：先尝试删除物理文件（或者校验文件锁）
	if err := os.Remove(yamlPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法删除配置文件，可能正被内核占用，请停止代理后重试: %v", err)
	}
	// 伴生规则文件一并清理
	_ = os.Remove(rulesPath)

	// 2. 物理文件删除成功后，再安全地更新内存与磁盘索引（事务提交）
	IndexLock.Lock()
	for i, item := range SubIndex {
		if item.ID == id {
			SubIndex = append(SubIndex[:i], SubIndex[i+1:]...)
			break
		}
	}
	IndexLock.Unlock()

	return SaveIndex()
}

// ReloadConfig 调用内核 API 热重载
func ReloadConfig() error {
	req, err := http.NewRequest("PUT", APIURL("/configs?force=true"), nil)
	if err != nil {
		return fmt.Errorf("构建重载请求失败: %v", err)
	}

	resp, err := localAPIClient.Do(req)
	if err != nil {
		return fmt.Errorf("内核配置重载请求失败: %v", err)
	}
	defer resp.Body.Close() // 规范：即使不需要读取响应内容，也必须释放底层的 TCP 连接

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("内核配置重载失败，状态码: %d", resp.StatusCode)
	}
	return nil
}

// StrictVerifyClashConfig 进行极度严格的 Clash 配置文件语义与结构级校验
func StrictVerifyClashConfig(data []byte) error {
	var root map[string]interface{}
	// 1. 基础语法校验：必须是合法的 YAML
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("文件解析失败，非合法 YAML 格式 (可能下载到了网页、HTML 或乱码)")
	}

	if len(root) == 0 {
		return fmt.Errorf("文件格式拒绝：配置文件为空")
	}

	// 2. 宏观特征校验：必须包含 Clash 的核心特征字段 (兼容首字母大写)
	hasProxies := root["proxies"] != nil || root["Proxy"] != nil
	hasProxyGroups := root["proxy-groups"] != nil || root["Proxy Group"] != nil
	hasProxyProviders := root["proxy-providers"] != nil

	if !hasProxies && !hasProxyGroups && !hasProxyProviders {
		return fmt.Errorf("格式拒绝：未检测到 proxies 或 proxy-groups。这不是一个标准的 Clash 订阅文件")
	}

	// 3. 刚性结构与语义抽样校验：防止披着 proxies 外衣的假数据
	if proxiesNode := root["proxies"]; proxiesNode != nil {
		proxiesList, ok := proxiesNode.([]interface{})
		if !ok {
			return fmt.Errorf("语法结构致命错误：[proxies] 必须是一个节点列表 (Array)")
		}

		// 抽样检查第一个代理节点的内部结构
		if len(proxiesList) > 0 {
			firstProxy, isMap := proxiesList[0].(map[string]interface{})
			if !isMap {
				return fmt.Errorf("语法结构致命错误：[proxies] 列表内的元素必须是节点对象 (Object)")
			}

			// Clash 节点的刚性必备属性，缺一不可
			requiredKeys := []string{"name", "type", "server", "port"}
			for _, key := range requiredKeys {
				if _, exists := firstProxy[key]; !exists {
					return fmt.Errorf("语义合规拒绝：代理节点缺失 Clash 必备底层属性 [%s]", key)
				}
			}
		}
	}

	// 校验 proxy-groups (策略组) 结构
	if groupsNode := root["proxy-groups"]; groupsNode != nil {
		groupsList, ok := groupsNode.([]interface{})
		if !ok {
			return fmt.Errorf("语法结构致命错误：[proxy-groups] 必须是一个组列表 (Array)")
		}

		if len(groupsList) > 0 {
			firstGroup, isMap := groupsList[0].(map[string]interface{})
			if !isMap {
				return fmt.Errorf("语法结构致命错误：[proxy-groups] 内的元素必须是对象 (Object)")
			}
			// 策略组必备属性
			if _, ok := firstGroup["name"]; !ok {
				return fmt.Errorf("语义合规拒绝：策略组缺失必备属性 [name]")
			}
			if _, ok := firstGroup["type"]; !ok {
				return fmt.Errorf("语义合规拒绝：策略组缺失必备属性 [type]")
			}
		}
	}

	// 校验通过，确认为高纯度合规的 Clash 配置
	return nil
}
