//go:build windows

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

// ListSubIndex 返回订阅索引的副本
func ListSubIndex() []SubIndexItem {
	IndexLock.RLock()
	defer IndexLock.RUnlock()

	out := make([]SubIndexItem, len(SubIndex))
	copy(out, SubIndex)
	return out
}

// FindSubIndexByID 根据 ID 查找订阅索引项
func FindSubIndexByID(id string) (SubIndexItem, bool) {
	IndexLock.RLock()
	defer IndexLock.RUnlock()

	for _, item := range SubIndex {
		if item.ID == id {
			return item, true
		}
	}
	return SubIndexItem{}, false
}

// ReplaceSubIndex 替换整个订阅索引并保存
func ReplaceSubIndex(next []SubIndexItem) error {
	IndexLock.Lock()
	SubIndex = append([]SubIndexItem(nil), next...)
	IndexLock.Unlock()

	return SaveIndex()
}

// UpdateSubIndex 使用 mutator 函数更新订阅索引并保存
func UpdateSubIndex(mutator func([]SubIndexItem) ([]SubIndexItem, error)) error {
	IndexLock.Lock()
	next, err := mutator(append([]SubIndexItem(nil), SubIndex...))
	if err == nil {
		SubIndex = next
	}
	IndexLock.Unlock()

	if err != nil {
		return err
	}
	return SaveIndex()
}

