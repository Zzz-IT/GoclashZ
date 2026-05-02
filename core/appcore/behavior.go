package appcore

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"sync"
)

type AppBehavior struct {
	SilentStart        bool   `json:"silentStart"` // 静默启动 (不弹窗，直接进托盘)
	CloseToTray        bool   `json:"closeToTray"` // 点击关闭时隐藏到托盘
	ColorDelay         bool   `json:"colorDelay"`  // 显色彩色延迟数字
	DelayRetention     bool   `json:"delayRetention"`
	DelayRetentionTime string `json:"delayRetentionTime"`
	LogLevel           string `json:"logLevel"` // 日志等级
	HideLogs           bool   `json:"hideLogs"`
	SubUA              string `json:"subUA"` // 订阅更新 User-Agent

	ActiveConfig string `json:"activeConfig"`
	ActiveMode   string `json:"activeMode"`

	GeoIpLink   string `json:"geoIpLink"`
	GeoSiteLink string `json:"geoSiteLink"`
	MmdbLink    string `json:"mmdbLink"`
	AsnLink     string `json:"asnLink"`

	AutoUpdate      bool   `json:"autoUpdate"`      // 是否开启自动更新
	UpdateMethod    string `json:"updateMethod"`    // 检查更新方式: "startup" (每次启动) 或 "scheduled" (定时)
	UpdateInterval  int    `json:"updateInterval"`  // 检查间隔时间 (天)
	LastUpdateCheck int64  `json:"lastUpdateCheck"` // 上次检查更新的时间戳 (Unix秒)

	// 👇 新增：自动测速控制
	AutoDelayTest         bool `json:"autoDelayTest"`
	AutoDelayTestInterval int  `json:"autoDelayTestInterval"`
}

type BehaviorStore struct {
	mu    sync.RWMutex
	cache AppBehavior
	path  string
}

func NewBehaviorStore() *BehaviorStore {
	store := &BehaviorStore{
		path: filepath.Join(utils.GetDataDir(), "behavior.json"),
	}
	store.Load()
	return store
}

func (s *BehaviorStore) Get() AppBehavior {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *BehaviorStore) SetAndSave(b AppBehavior) error {
	s.mu.Lock()
	s.cache = b
	s.mu.Unlock()
	return s.Save()
}

func (s *BehaviorStore) SetActiveConfig(id string) error {
	s.mu.Lock()
	s.cache.ActiveConfig = id
	s.mu.Unlock()
	return s.Save()
}

func (s *BehaviorStore) GetActiveConfig() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.ActiveConfig
}

func (s *BehaviorStore) SetActiveMode(mode string) error {
	s.mu.Lock()
	s.cache.ActiveMode = mode
	s.mu.Unlock()
	return s.Save()
}

func (s *BehaviorStore) GetActiveMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.ActiveMode
}

func (s *BehaviorStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		s.cache = s.Default()
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.cache)
}

func (s *BehaviorStore) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.cache, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *BehaviorStore) Default() AppBehavior {
	return AppBehavior{
		SilentStart:        false,
		CloseToTray:        true,
		ColorDelay:         false,
		DelayRetention:     true,
		DelayRetentionTime: "long",
		LogLevel:           "info",
		HideLogs:           false,
		SubUA:              "clash-verge",
		ActiveMode:         "rule",
		AutoUpdate:         true,
		UpdateMethod:       "startup",
		UpdateInterval:     1,
		GeoIpLink:          "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
		GeoSiteLink:        "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		MmdbLink:           "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		AsnLink:            "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb",
	}
}
