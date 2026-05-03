//go:build windows

package appcore

import (
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
}

func NewBehaviorStore() *BehaviorStore {
	// 1. 旧版本配置文件平滑迁移逻辑
	oldPath := filepath.Join(utils.GetDataDir(), "behavior.json")
	if _, err := os.Stat(oldPath); err == nil {
		newPath := filepath.Join(utils.GetSettingsDir(), "user_behavior.json")
		// 如果旧文件存在，且新文件不存在，则将其移动过去
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			_ = os.MkdirAll(utils.GetSettingsDir(), 0755)
			_ = os.Rename(oldPath, newPath)
		} else {
			// 如果新文件已存在，直接删掉废弃的旧文件
			_ = os.Remove(oldPath)
		}
	}

	store := &BehaviorStore{}
	_ = store.Load()
	return store
}

func (s *BehaviorStore) Get() AppBehavior {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *BehaviorStore) SetAndSave(b AppBehavior) error {
	b = normalizeBehavior(b)

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
	defaults := s.Default()

	// 使用统一的 LoadSetting 机制 (注意这里传入指针)
	cfg, err := utils.LoadSetting("behavior", defaults)
	if err != nil {
		s.mu.Lock()
		s.cache = defaults
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.cache = normalizeBehavior(*cfg)
	s.mu.Unlock()
	return nil
}

func (s *BehaviorStore) Save() error {
	s.mu.RLock()
	cfg := s.cache
	s.mu.RUnlock()

	// 使用统一的 SaveSetting 机制
	return utils.SaveSetting("behavior", &cfg)
}

func normalizeBehavior(b AppBehavior) AppBehavior {
	if b.LogLevel == "" {
		b.LogLevel = "info"
	}

	if b.DelayRetentionTime == "" {
		b.DelayRetentionTime = "long"
	}

	if b.ActiveMode == "" {
		b.ActiveMode = "rule"
	}

	if b.SubUA == "" {
		b.SubUA = "clash-verge"
	}

	if b.UpdateMethod == "" {
		b.UpdateMethod = "startup"
	}

	if b.UpdateInterval <= 0 {
		b.UpdateInterval = 1
	}

	if b.AutoDelayTest && b.AutoDelayTestInterval <= 0 {
		b.AutoDelayTestInterval = 60
	}

	if b.GeoIpLink == "" {
		b.GeoIpLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"
	}
	if b.GeoSiteLink == "" {
		b.GeoSiteLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"
	}
	if b.MmdbLink == "" {
		b.MmdbLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb"
	}
	if b.AsnLink == "" {
		b.AsnLink = "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb"
	}

	return b
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

		AutoUpdate:     true,
		UpdateMethod:   "startup",
		UpdateInterval: 1,

		AutoDelayTest:         false,
		AutoDelayTestInterval: 60,

		GeoIpLink:   "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
		GeoSiteLink: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		MmdbLink:    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		AsnLink:     "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb",
	}
}
