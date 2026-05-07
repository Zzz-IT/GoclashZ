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
	nodes map[string]map[string]string // profileID -> groupName -> nodeName
	path  string
}

func NewOfflineNodeStore() *OfflineNodeStore {
	store := &OfflineNodeStore{
		nodes: make(map[string]map[string]string),
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

func (s *OfflineNodeStore) Mark(profileID, groupName, nodeName string) {
	if profileID == "" {
		profileID = "default"
	}
	s.mu.Lock()
	if s.nodes == nil {
		s.nodes = make(map[string]map[string]string)
	}
	if s.nodes[profileID] == nil {
		s.nodes[profileID] = make(map[string]string)
	}
	s.nodes[profileID][groupName] = nodeName
	s.mu.Unlock()
	s.Save()
}

func (s *OfflineNodeStore) Clear() {
	s.mu.Lock()
	s.nodes = make(map[string]map[string]string)
	s.mu.Unlock()
	s.Save()
}

func (s *OfflineNodeStore) Get(profileID, groupName string) (string, bool) {
	if profileID == "" {
		profileID = "default"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.nodes == nil {
		return "", false
	}
	profileNodes, exists := s.nodes[profileID]
	if !exists {
		return "", false
	}
	node, exists := profileNodes[groupName]
	return node, exists
}

func (s *OfflineNodeStore) Snapshot(profileID string) map[string]string {
	if profileID == "" {
		profileID = "default"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[string]string)
	if s.nodes == nil {
		return snapshot
	}
	profileNodes, exists := s.nodes[profileID]
	if !exists {
		return snapshot
	}
	for k, v := range profileNodes {
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
