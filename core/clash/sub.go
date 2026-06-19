//go:build windows

package clash

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"goclashz/core/downloader"
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

	dir := utils.GetSubscriptionsDir()
	os.MkdirAll(dir, 0755)

	// 🛡️ 防御路径穿越：利用统一工具函数提取安全文件名
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return id, err
	}

	originPath := filepath.Join(OriginDir(), safeId+".yaml")
	workingPath := filepath.Join(SubscriptionsDir(), safeId+".yaml")

	os.MkdirAll(filepath.Dir(originPath), 0755)

	var upload, download, total, expire int64

	err = downloader.FetchSmallFileAtomic(ctx, downloader.Options{
		URLs:               []string{url},
		DestPath:           originPath,
		UserAgent:          userAgent,
		MaxBytes:           50 * 1024 * 1024,
		Strategy: func() downloader.DownloadStrategy {
			var pUrl string
			if IsRunning() {
				if netCfg, err := GetNetworkConfig(); err == nil && netCfg.MixedPort != 0 {
					pUrl = fmt.Sprintf("http://127.0.0.1:%d", netCfg.MixedPort)
				}
			}
			return downloader.DownloadStrategy{
				ProxyURL:    pUrl,
				PreferProxy: pUrl != "",
			}
		},
		InsecureSkipVerify: true,     // 🛡️ [SSL宽容] 机场证书烂也能下载
		OnResponse: func(resp *http.Response) {
			// 🛡️ [流量提取] 解析 Subscription-Userinfo
			if info := resp.Header.Get("Subscription-Userinfo"); info != "" {
				upload, download, total, expire = parseSubUserInfo(info)
			}
		},
		Validator: func(tmpPath string) error {
			// 🛡️ [原子防损] 只有通过极严 YAML 结构校验的文件才会被最终替换
			data, err := os.ReadFile(tmpPath)
			if err != nil {
				return err
			}
			if err := StrictVerifyClashConfig(data); err != nil {
				return fmt.Errorf("订阅配置校验失败: %v (可能下载到了网页、HTML 或乱码)", err)
			}
			if err := ValidateClashReferencesBytes(data); err != nil {
				return fmt.Errorf("订阅配置引用校验失败: %v", err)
			}
			
			// 在更新之前，先备份原有的 origin 文件（如果存在）
			if _, statErr := os.Stat(originPath); statErr == nil {
				originData, _ := os.ReadFile(originPath)
				_ = utils.WriteFileAtomic(originPath+".bak", originData, 0644)
			}

			// 原子复制到 working (如果这是一个全新的订阅，我们直接用 origin 覆盖 working)
			// 如果是更新订阅，这里也是用新内容覆盖 working
			if err := utils.WriteFileAtomic(workingPath, data, 0644); err != nil {
				return err
			}
			return nil
		},
	})

	if err != nil {
		// 如果过程中失败，尝试从 bak 恢复 origin
		if _, statErr := os.Stat(originPath + ".bak"); statErr == nil {
			_ = os.Rename(originPath+".bak", originPath)
		}
		return safeId, err
	}

	// 确认更新成功后删除 bak 文件
	_ = os.Remove(originPath + ".bak")

	// 4. 确保 overlay 存在 (如果是新订阅，创建空 overlay；如果是更新，保留用户配置)
	if err := EnsureEmptyOverlay(safeId); err != nil {
		return safeId, fmt.Errorf("初始化规则配置失败: %w", err)
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
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return err
	}

	dir := utils.GetSubscriptionsDir()
	yamlPath := filepath.Join(dir, safeId+".yaml")
	rulesPath := filepath.Join(dir, safeId+"_rules.json")
	originPath := filepath.Join(dir, "origin", safeId+".yaml")
	overlayPath := filepath.Join(dir, safeId+"_overlay.json")

	// 1. 🚀 核心修复：先尝试删除物理文件（或者校验文件锁）
	if err := os.Remove(yamlPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法删除配置文件，可能正被内核占用，请停止代理后重试: %v", err)
	}
	// 清理伴生文件
	_ = os.Remove(rulesPath)
	_ = os.Remove(originPath)
	_ = os.Remove(overlayPath)

	// 2. 物理文件删除成功后，再安全地更新内存与磁盘索引（事务提交）
	IndexLock.Lock()
	for i, item := range SubIndex {
		if item.ID == safeId {
			SubIndex = append(SubIndex[:i], SubIndex[i+1:]...)
			break
		}
	}
	IndexLock.Unlock()

	return SaveIndex()
}

// ReloadConfig 调用内核 API 热重载
func ReloadConfig() error {
	return doKernelRequest(
		context.Background(),
		http.MethodPut,
		"/configs?force=true",
		nil,
		http.StatusOK,
		http.StatusNoContent,
	)
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
// ImportLocalConfig 导入本地配置文件
func ImportLocalConfig(srcPath, name string) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}

	if err := StrictVerifyClashConfig(data); err != nil {
		return "", err
	}

	if err := ValidateClashReferencesBytes(data); err != nil {
		return "", err
	}

	id := fmt.Sprintf("%d", time.Now().UnixMilli())
	safeId, _ := utils.SanitizeFilename(id)

	originPath := filepath.Join(OriginDir(), safeId+".yaml")
	workingPath := filepath.Join(SubscriptionsDir(), safeId+".yaml")

	os.MkdirAll(filepath.Dir(originPath), 0755)

	if err := utils.WriteFileAtomic(originPath, data, 0644); err != nil {
		return "", err
	}

	if err := utils.WriteFileAtomic(workingPath, data, 0644); err != nil {
		return "", err
	}

	// 更新索引
	IndexLock.Lock()
	SubIndex = append(SubIndex, SubIndexItem{
		ID:      safeId,
		Name:    name,
		URL:     "",
		Type:    "local",
		Updated: time.Now().Unix(),
	})
	IndexLock.Unlock()

	return safeId, SaveIndex()
}
