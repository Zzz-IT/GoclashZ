package clash

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"sync"
)

// SubIndexItem 定义前端列表显示所需的轻量级数据
type SubIndexItem struct {
	ID       string `json:"id"`       // 唯一标识，如 "1713840000000"
	Name     string `json:"name"`     // UI显示名称
	URL      string `json:"url"`      // 订阅链接，本地文件则为空
	Type     string `json:"type"`     // "remote" 或 "local"
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
	Expire   int64  `json:"expire"`
	Updated  int64  `json:"updated"`
}

var (
	IndexLock sync.RWMutex
	SubIndex  []SubIndexItem
)

func getIndexPath() string {
	return filepath.Join(utils.GetProfilesDir(), "index.json")
}

// LoadIndex 启动时加载
func LoadIndex() error {
	IndexLock.Lock()
	defer IndexLock.Unlock()
	data, err := os.ReadFile(getIndexPath())
	if err != nil {
		SubIndex = []SubIndexItem{}
		return nil
	}
	return json.Unmarshal(data, &SubIndex)
}

// SaveIndex 保存到本地 (原子写入版)
func SaveIndex() error {
	// 1. 注意这里从 RLock() 提升为了 Lock()，因为底层文件写入需要绝对的串行保护
	IndexLock.Lock()
	defer IndexLock.Unlock()

	data, err := json.MarshalIndent(SubIndex, "", "  ")
	if err != nil {
		return err
	}

	indexPath := getIndexPath()
	tempPath := indexPath + ".tmp"

	// 2. 先将数据安全地写入临时文件
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	// 3. 原子重命名覆盖（Windows 底层原生支持 MoveFileEx 进行原子覆盖）
	if err := os.Rename(tempPath, indexPath); err != nil {
		// 如果重命名失败，删除无用的临时文件
		os.Remove(tempPath)
		return err
	}

	return nil
}
