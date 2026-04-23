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

// SaveIndex 保存到本地
func SaveIndex() error {
	IndexLock.RLock()
	defer IndexLock.RUnlock()
	data, _ := json.MarshalIndent(SubIndex, "", "  ")
	return os.WriteFile(getIndexPath(), data, 0644)
}
