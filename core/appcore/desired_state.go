//go:build windows

package appcore

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const desiredStateSettingName = "desired_state"

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

	if err := utils.SaveSetting(desiredStateSettingName, &d); err != nil {
		return err
	}

	s.mu.Lock()
	s.cache = d
	s.mu.Unlock()

	return nil
}

func (s *DesiredStateStore) Update(fn func(d *DesiredState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	newCache := s.cache
	fn(&newCache)
	newCache.UpdatedAt = time.Now().Unix()

	if err := utils.SaveSetting(desiredStateSettingName, &newCache); err != nil {
		return err
	}

	s.cache = newCache
	return nil
}

func (s *DesiredStateStore) Load() error {
	defaults := s.Default()
	if migrated, ok := loadLegacyDesiredState(defaults); ok {
		s.mu.Lock()
		s.cache = migrated
		s.mu.Unlock()
		return utils.SaveSetting(desiredStateSettingName, &migrated)
	}

	cfg, err := utils.LoadSetting(desiredStateSettingName, defaults)
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

	return utils.SaveSetting(desiredStateSettingName, &cfg)
}

// loadLegacyDesiredState accepts historical file names but always rewrites the
// normalized state to Settings/user_desired_state.json.
func loadLegacyDesiredState(defaults DesiredState) (DesiredState, bool) {
	legacyPaths := []string{
		filepath.Join(utils.GetSettingsDir(), "user_user_desired_state.json"),
		filepath.Join(utils.GetSettingsDir(), "desired_state.json"),
		filepath.Join(utils.GetDataDir(), "desired_state.json"),
	}

	canonicalPath := filepath.Join(utils.GetSettingsDir(), "user_desired_state.json")
	if _, err := os.Stat(canonicalPath); err == nil {
		return DesiredState{}, false
	}

	for _, path := range legacyPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		state := defaults
		if json.Unmarshal(data, &state) == nil {
			return state, true
		}
	}

	return DesiredState{}, false
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
