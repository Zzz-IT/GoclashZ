package runtimeassets

type AssetKey string

const (
	AssetCore    AssetKey = "core"
	AssetWintun  AssetKey = "wintun"
	AssetGeoIP   AssetKey = "geoip"
	AssetGeoSite AssetKey = "geosite"
	AssetMMDB    AssetKey = "mmdb"
	AssetASN     AssetKey = "asn"
)

type AssetErrorCode string

const (
	ErrNone       AssetErrorCode = ""
	ErrMissing    AssetErrorCode = "missing"
	ErrIsDir      AssetErrorCode = "is_dir"
	ErrInvalidPE  AssetErrorCode = "invalid_pe"
	ErrExecFailed AssetErrorCode = "exec_failed"
	// ErrCorrupt 表示文件存在但内容明显不完整（例如 0 字节 / 被截断的规则库）。
	ErrCorrupt AssetErrorCode = "corrupt"
)

type AssetHealth struct {
	Key      AssetKey `json:"key"`
	Label    string   `json:"label"`
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Valid    bool     `json:"valid"`
	Ready    bool     `json:"ready"`
	Required bool     `json:"required"`

	Size           int64  `json:"size"`
	ModTime        int64  `json:"modTime"`
	SHA256         string `json:"sha256,omitempty"`
	Version        string `json:"version,omitempty"`
	VersionProbeOK bool   `json:"versionProbeOK,omitzero"`

	ErrorCode AssetErrorCode `json:"errorCode,omitempty"`
	Error     string         `json:"error,omitempty"`
	Hint      string         `json:"hint,omitempty"`
}

type RuntimeAssetStatus struct {
	AppDir         string                   `json:"appDir"`
	DataDir        string                   `json:"dataDir"`
	CoreBinDir     string                   `json:"coreBinDir"`
	SeedCoreBinDir string                   `json:"seedCoreBinDir"`
	Assets         map[AssetKey]AssetHealth `json:"assets"`

	CoreReady   bool `json:"coreReady"`
	WintunReady bool `json:"wintunReady"`
	GeoReady    bool `json:"geoReady"`

	// Ready 是历史兼容字段，语义固定为「内核 + 驱动就绪」：规则库不参与，
	// 避免把"能启动"与"规则完整"混成一个布尔（F-A1-20）。
	// 需要"三层全就绪"请用 ReadyForAll。
	Ready bool `json:"ready"`
	// ReadyForAll = CoreReady && WintunReady && GeoReady。
	ReadyForAll bool `json:"readyForAll"`

	// Required 记录本次评估场景真正需要的资产键（由 EnsureReady 按 Requirement 填）。
	// AssetHealth.Required 是"静态必需性"（内核/驱动恒 true，规则库恒 false），
	// 单看它会误判规则库；场景相关的必需性以本字段为准。
	Required []AssetKey `json:"required,omitempty"`
}

// Requirement 定义不同场景对资产的需求
type Requirement struct {
	NeedCore   bool
	NeedWintun bool
	NeedGeo    bool
}

var (
	RequireCoreOnly = Requirement{NeedCore: true}
	RequireTun      = Requirement{NeedCore: true, NeedWintun: true}
	RequireAll      = Requirement{NeedCore: true, NeedWintun: true, NeedGeo: true}
	RequireGeoOnly  = Requirement{NeedGeo: true}
)

func baseHealth(key AssetKey, label string, path string, required bool) AssetHealth {
	return AssetHealth{
		Key:      key,
		Label:    label,
		Path:     path,
		Required: required,
	}
}
