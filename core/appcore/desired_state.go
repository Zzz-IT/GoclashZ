//go:build windows

package appcore

import (
	"goclashz/core/utils"
	"sync"
	"time"
)

type DesiredState struct {
	CoreRunning  bool   `json:"coreRunning"`
	SystemProxy  bool   `json:"systemProxy"`
	Tun          bool   `json:"tun"`
	ActiveConfig string `json:"activeConfig"`
	Mode         string `json:"mode"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type DesiredStateStore struct {
	mu    sync.RWMutex
	cache DesiredState
}

func NewDesiredStateStore() *DesiredStateStore {
	store := &DesiredStateStore{}
	_ = store.Load()
	return store
}

func (s *DesiredStateStore) Get() DesiredState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *DesiredStateStore) SetAndSave(d DesiredState) error {
	d.UpdatedAt = time.Now().Unix()

	s.mu.Lock()
	s.cache = d
	snapshot := s.cache
	s.mu.Unlock()

	return utils.SaveSetting("user_desired_state", &snapshot)
}

func (s *DesiredStateStore) Load() error {
	defaults := s.Default()

	cfg, err := utils.LoadSetting("user_desired_state", defaults)
	if err != nil {
		s.mu.Lock()
		s.cache = defaults
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.cache = *cfg
	s.mu.Unlock()
	return nil
}

func (s *DesiredStateStore) Save() error {
	s.mu.RLock()
	cfg := s.cache
	s.mu.RUnlock()

	cfg.UpdatedAt = time.Now().Unix()

	return utils.SaveSetting("user_desired_state", &cfg)
}

func (s *DesiredStateStore) Default() DesiredState {
	return DesiredState{
		CoreRunning:  false,
		SystemProxy:  false,
		Tun:          false,
		ActiveConfig: "",
		Mode:         "rule",
		UpdatedAt:    time.Now().Unix(),
	}
}
