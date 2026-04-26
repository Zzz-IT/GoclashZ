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
	indexIOMu sync.Mutex // 专属 IO 锁，仅用于保护磁盘写入
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
	// 1. 内存极速序列化阶段（仅锁内存）
	IndexLock.RLock()
	data, err := json.MarshalIndent(SubIndex, "", "  ")
	IndexLock.RUnlock()

	if err != nil {
		return err
	}

	// 2. 磁盘写入阶段（严格串行化）
	indexIOMu.Lock()
	defer indexIOMu.Unlock()

	indexPath := getIndexPath()
	tempPath := indexPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tempPath, indexPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

