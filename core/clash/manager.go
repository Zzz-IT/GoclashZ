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
	// 1. 🚀 优化：使用 RLock，并在极短的内存序列化后立刻释放
	IndexLock.RLock()
	data, err := json.MarshalIndent(SubIndex, "", "  ")
	IndexLock.RUnlock() // 尽早释放内存锁，不再阻塞列表读取

	if err != nil {
		return err
	}

	indexPath := getIndexPath()
	tempPath := indexPath + ".tmp"

	// 2. 耗时的磁盘 I/O 移到无锁环境执行
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, indexPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}
