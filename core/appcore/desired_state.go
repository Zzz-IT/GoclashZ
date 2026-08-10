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
	// PortProxy 独立跟踪端口代理激活状态，与 CoreRunning/SystemProxy/Tun 完全解耦
	// TUN 开关永远不会修改此字段，系统代理开关也不会修改此字段
	PortProxy    bool   `json:"portProxy"`
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

	// Hold the write lock for the entire disk-write + cache-update so that
	// concurrent callers cannot produce a state where disk and cache diverge
	// (e.g. write-A finishes disk, write-B finishes disk and cache, write-A
	// then overwrites cache with stale value).
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := utils.SaveSetting(desiredStateSettingName, &d); err != nil {
		return err
	}
	s.cache = d
	return nil
}

// SetAndSaveIf is a compare-and-swap variant of SetAndSave. It only commits
// next if the current in-memory cache matches expected (UpdatedAt is excluded
// from the comparison so callers need not track the timestamp). Returns
// (false, nil) when the swap is skipped due to a mismatch, (true, err) on
// attempt.
func (s *DesiredStateStore) SetAndSaveIf(expected, next DesiredState) (bool, error) {
	next.UpdatedAt = time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Compare every field except the bookkeeping timestamp.
	cur := s.cache
	cur.UpdatedAt = 0
	exp := expected
	exp.UpdatedAt = 0
	if cur != exp {
		return false, nil
	}

	if err := utils.SaveSetting(desiredStateSettingName, &next); err != nil {
		return false, err
	}
	s.cache = next
	return true, nil
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
		PortProxy:    false,
		SystemProxy:  false,
		Tun:          false,
		ActiveConfig: "",
		Mode:         "rule",
		UpdatedAt:    time.Now().Unix(),
	}
}
