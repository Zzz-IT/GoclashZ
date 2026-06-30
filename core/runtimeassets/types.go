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
)

type AssetHealth struct {
	Key      AssetKey       `json:"key"`
	Label    string         `json:"label"`
	Path     string         `json:"path"`
	Exists   bool           `json:"exists"`
	Valid    bool           `json:"valid"`
	Ready    bool           `json:"ready"`
	Required bool           `json:"required"`

	Size          int64  `json:"size"`
	ModTime       int64  `json:"modTime"`
	SHA256        string `json:"sha256,omitempty"`
	Version       string `json:"version,omitempty"`
	VersionProbeOK bool  `json:"versionProbeOK,omitempty"`

	ErrorCode AssetErrorCode `json:"errorCode,omitempty"`
	Error     string         `json:"error,omitempty"`
	Hint      string         `json:"hint,omitempty"`
}

type RuntimeAssetStatus struct {
	AppDir         string                 `json:"appDir"`
	DataDir        string                 `json:"dataDir"`
	CoreBinDir     string                 `json:"coreBinDir"`
	SeedCoreBinDir string                 `json:"seedCoreBinDir"`
	Assets         map[AssetKey]AssetHealth `json:"assets"`

	CoreReady   bool `json:"coreReady"`
	WintunReady bool `json:"wintunReady"`
	Ready       bool `json:"ready"`
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
)

func baseHealth(key AssetKey, label string, path string, required bool) AssetHealth {
	return AssetHealth{
		Key:      key,
		Label:    label,
		Path:     path,
		Required: required,
	}
}
