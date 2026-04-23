package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"goclashz/core/utils"
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
			val, _ := strconv.ParseInt(kv[1], 10, 64)
			switch strings.ToLower(kv[0]) {
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
func DownloadSub(name, url, existingId, userAgent string) (string, error) {
	id := existingId
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return id, err
	}

	if userAgent == "" {
		userAgent = "ClashforWindows/0.20.39"
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

	// 3. 绝对只读保存原始 YAML
	yamlPath := filepath.Join(utils.GetProfilesDir(), id+".yaml")
	outFile, err := os.Create(yamlPath)
	if err != nil {
		return id, fmt.Errorf("无法创建配置文件: %v", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return id, err
	}

	// 4. 初始化伴生规则文件 (如果不存在才创建，防止洗掉用户规则)
	rulesPath := filepath.Join(utils.GetProfilesDir(), id+"_rules.json")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		os.WriteFile(rulesPath, []byte(`{"customRules":[]}`), 0644)
	}

	// 5. 更新全局索引
	IndexLock.Lock()
	found := false
	for i, item := range SubIndex {
		if item.ID == id {
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
			ID:       id,
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

	return id, SaveIndex()
}

func RenameConfig(id, newName string) error {
	IndexLock.Lock()
	defer IndexLock.Unlock()
	for i, item := range SubIndex {
		if item.ID == id {
			SubIndex[i].Name = newName
			break
		}
	}
	return SaveIndex() // 只改 json，底层 yaml 名字不动！
}

func DeleteConfig(id string) error {
	IndexLock.Lock()
	for i, item := range SubIndex {
		if item.ID == id {
			SubIndex = append(SubIndex[:i], SubIndex[i+1:]...)
			break
		}
	}
	IndexLock.Unlock()
	SaveIndex()

	// 删除物理文件
	dir := utils.GetProfilesDir()
	os.Remove(filepath.Join(dir, id+".yaml"))
	os.Remove(filepath.Join(dir, id+"_rules.json"))
	return nil
}

// ReloadConfig 调用内核 API 热重载
func ReloadConfig() error {
	req, _ := http.NewRequest("PUT", "http://127.0.0.1:9090/configs?force=true", nil)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("内核配置重载失败")
	}
	return nil
}
