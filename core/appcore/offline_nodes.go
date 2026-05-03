//go:build windows

package appcore

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"sync"
)

type OfflineNodeStore struct {
	mu    sync.RWMutex
	nodes map[string]string
	path  string
}

func NewOfflineNodeStore() *OfflineNodeStore {
	store := &OfflineNodeStore{
		nodes: make(map[string]string),
		path:  filepath.Join(utils.GetDataDir(), "offline_nodes.json"),
	}
	store.Load()
	return store
}

func (s *OfflineNodeStore) Load() {
	data, err := os.ReadFile(s.path)
	if err == nil {
		s.mu.Lock()
		json.Unmarshal(data, &s.nodes)
		s.mu.Unlock()
	}
}

func (s *OfflineNodeStore) Save() {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.nodes, "", "  ")
	s.mu.RUnlock()
	
	if err == nil {
		os.WriteFile(s.path, data, 0644)
	}
}

func (s *OfflineNodeStore) Mark(groupName string, nodeName string) {
	s.mu.Lock()
	if s.nodes == nil {
		s.nodes = make(map[string]string)
	}
	s.nodes[groupName] = nodeName
	s.mu.Unlock()
	s.Save()
}

func (s *OfflineNodeStore) Clear() {
	s.mu.Lock()
	s.nodes = make(map[string]string)
	s.mu.Unlock()
	s.Save()
}

func (s *OfflineNodeStore) Get(groupName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.nodes == nil {
		return "", false
	}
	node, exists := s.nodes[groupName]
	return node, exists
}

func (s *OfflineNodeStore) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[string]string)
	for k, v := range s.nodes {
		snapshot[k] = v
	}
	return snapshot
}

func MergeOfflineSelection(data map[string]interface{}, selected map[string]string) {
	if groups, ok := data["groups"].(map[string]interface{}); ok {
		for gName, groupData := range groups {
			if gMap, ok2 := groupData.(map[string]interface{}); ok2 {
				// 优先使用离线选择
				if selNode, exists := selected[gName]; exists {
					gMap["now"] = selNode
				}
				// 兜底：没有当前选中项，默认选中第一项
				if gMap["now"] == "" || gMap["now"] == nil {
					if lenRaw, has := gMap["all"]; has {
						if allArr, ok3 := lenRaw.([]interface{}); ok3 && len(allArr) > 0 {
							if firstNode, ok4 := allArr[0].(string); ok4 {
								gMap["now"] = firstNode
							}
						}
					}
				}
			}
		}
	}
}
