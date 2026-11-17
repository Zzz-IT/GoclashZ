//go:build windows

package appcore

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	jsonutil "goclashz/core/jsonutil"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"goclashz/core/version"
)

type DiagnosticAssets struct {
	ClashExe    bool `json:"clash.exe"`
	WintunDll   bool `json:"wintun.dll"`
	Geoip       bool `json:"geoip.metadb"`
	Geosite     bool `json:"geosite.dat"`
	CountryMmdb bool `json:"country.mmdb"`
	AsnDat      bool `json:"asn.dat"`
}

type DiagnosticInfo struct {
	AppDir            string `json:"appDir"`
	DataDir           string `json:"dataDir"`
	SeedCoreBinDir    string `json:"seedCoreBinDir"`
	RuntimeCoreBinDir string `json:"runtimeCoreBinDir"`
	RuntimeConfigPath string `json:"runtimeConfigPath"`
	IsAdmin           bool   `json:"isAdmin"`

	HelperServiceStatus string `json:"helperServiceStatus"`
	HelperBinaryPath    string `json:"helperBinaryPath"`

	Assets DiagnosticAssets `json:"assets"`

	SeedManifest any `json:"seedManifest"`
	AssetState   any `json:"assetState"`

	// ---- F-A3-14 增补（均为廉价字段，不引入额外 RPC） ----
	AppVersion string           `json:"appVersion"`
	Goos       string           `json:"goos"`
	Goarch     string           `json:"goarch"`
	LogDir     string           `json:"logDir"`
	LogFiles   []DiagnosticFile `json:"logFiles,omitempty"`
	// LogStats 是日志子系统自身的健康计数（丢弃/落盘/环容量，F-A3-20）。
	LogStats logger.Stats `json:"logStats"`
}

// DiagnosticFile 是诊断包里附带的文件清单条目。
type DiagnosticFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mod  string `json:"modTime,omitempty"`
}

// DiagnosticBundle 是 diagnostic.zip 里 diagnostics.json 的根结构。
type DiagnosticBundle struct {
	GeneratedAt string `json:"generatedAt"`
	// Redaction 说明本包做了哪些脱敏，避免接收方误读。
	Redaction []string       `json:"redaction"`
	Info      DiagnosticInfo `json:"info"`

	// 以下为导出时额外采集的运行时状态（会多花一次 RPC，只在导出时做）。
	CoreStatus  any              `json:"coreStatus,omitempty"`
	HelperError string           `json:"helperError,omitempty"`
	LogFiles    []DiagnosticFile `json:"logFiles,omitempty"`
	AssetFiles  []DiagnosticFile `json:"assetFiles,omitempty"`
	ConfigFile  string           `json:"configFile,omitempty"`
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// logDir 返回日志目录（与 logger.InitFileSink 使用同一路径）。
func logDir() string {
	return filepath.Join(utils.GetDataDir(), "logs")
}

func GetDiagnosticInfo() DiagnosticInfo {
	info := DiagnosticInfo{
		AppDir:            utils.GetAppDir(),
		DataDir:           utils.GetDataDir(),
		SeedCoreBinDir:    utils.GetSeedCoreBinDir(),
		RuntimeCoreBinDir: utils.GetCoreBinDir(),
		RuntimeConfigPath: utils.GetRuntimeConfigPath(),
		IsAdmin:           sys.CheckAdmin(),
		AppVersion:        version.AppVersion,
		Goos:              runtime.GOOS,
		Goarch:            runtime.GOARCH,
		LogDir:            logDir(),
	}

	// Helper Service Status
	status := sys.CheckHelperService()
	info.HelperServiceStatus = status.Error
	if status.Reachable {
		info.HelperServiceStatus = "Reachable"
	} else if status.Running {
		info.HelperServiceStatus = "Running but not reachable"
	} else if status.Installed {
		info.HelperServiceStatus = "Installed but not running"
	} else if status.Error == "" {
		info.HelperServiceStatus = "Not installed"
	}

	info.HelperBinaryPath = filepath.Join(info.AppDir, "GoclashZHelper.exe")

	// Assets
	info.Assets.ClashExe = fileExists(filepath.Join(info.RuntimeCoreBinDir, "clash.exe"))
	info.Assets.WintunDll = fileExists(filepath.Join(info.RuntimeCoreBinDir, "wintun.dll"))
	info.Assets.Geoip = fileExists(filepath.Join(info.RuntimeCoreBinDir, "geoip.metadb"))
	info.Assets.Geosite = fileExists(filepath.Join(info.RuntimeCoreBinDir, "geosite.dat"))
	info.Assets.CountryMmdb = fileExists(filepath.Join(info.RuntimeCoreBinDir, "country.mmdb"))
	info.Assets.AsnDat = fileExists(filepath.Join(info.RuntimeCoreBinDir, "asn.dat"))

	manifest, _ := runtimeassets.LoadSeedManifest()
	info.SeedManifest = manifest

	info.AssetState = runtimeassets.LoadAssetState()

	// 日志文件清单（F-A3-13/F-A3-14：helper 侧的内核输出日志也在这里）
	info.LogFiles = listDiagnosticFiles(info.LogDir, logFileMaxAge, 0)
	info.LogStats = logger.GetStats()

	return info
}

const (
	// logFileMaxAge 是诊断包携带日志的时间窗。
	logFileMaxAge = 3 * 24 * time.Hour
	// diagMaxTotalBytes 限制诊断包里日志部分的总大小，避免拖垮导出。
	diagMaxTotalBytes int64 = 32 * 1024 * 1024
)

// listDiagnosticFiles 列出目录里最近 maxAge 内的普通文件（按修改时间倒序）。
// maxTotal>0 时按顺序累加，超过预算即停止。
func listDiagnosticFiles(dir string, maxAge time.Duration, maxTotal int64) []DiagnosticFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	files := make([]DiagnosticFile, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if maxTotal > 0 && total+info.Size() > maxTotal {
			continue
		}
		total += info.Size()
		files = append(files, DiagnosticFile{
			Name: e.Name(),
			Size: info.Size(),
			Mod:  info.ModTime().Format(time.RFC3339),
		})
	}
	return files
}

// ExportDiagnosticsToFile 导出诊断包（zip：diagnostics.json + 日志 + 资产清单 + 脱敏配置 + system.txt）。
//
// 兼容性：调用方（controller.ExportDiagnostics 的保存对话框）目前仍以 .json 命名，
// 这里在扩展名不是 .zip 时自动补 .zip，避免把 zip 内容写成 .json 造成误判。
func ExportDiagnosticsToFile(path string) error {
	start := time.Now()
	corr := fmt.Sprintf("diag-%d-%d", os.Getpid(), start.UnixNano())

	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".zip") {
		path += ".zip"
	}
	// 固定文件名会被下一次导出覆盖；这里补时间戳（F-A3-21）。
	path = ensureTimestampSuffix(path)

	logger.Record(logger.NewEvent("diag", "diag.export.start").
		WithCorr(corr).
		WithPayload("开始导出诊断包: %s", redactPath(path)))

	if err := writeDiagnosticZip(path); err != nil {
		logger.Record(logger.NewEvent("diag", "diag.export.failed").
			WithLevel("error").
			WithCode("DIAG_EXPORT_FAILED").
			WithCorr(corr).
			WithDur(time.Since(start)).
			WithPayload("导出诊断包失败: %v", err))
		return err
	}

	logger.Record(logger.NewEvent("diag", "diag.export.end").
		WithCorr(corr).
		WithDur(time.Since(start)).
		WithPayload("诊断包已生成: %s", redactPath(path)))
	return nil
}

func writeDiagnosticZip(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	zw := zip.NewWriter(f)

	bundle := collectDiagnosticBundle()
	if data, err := jsonutil.MarshalIndent(bundle, "", "  "); err == nil {
		if err := zipWriteBytes(zw, "diagnostics.json", data); err != nil {
			_ = zw.Close()
			return err
		}
	}

	if err := zipAddLogs(zw, bundle); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zipAddAssets(zw, bundle); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zipAddConfig(zw); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zipWriteBytes(zw, "system.txt", []byte(buildSystemSummary(bundle))); err != nil {
		_ = zw.Close()
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return f.Sync()
}

// collectDiagnosticBundle 汇总导出所需的一切（含 helper/core 状态，属于「导出时才花」的成本）。
func collectDiagnosticBundle() DiagnosticBundle {
	bundle := DiagnosticBundle{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Info:        GetDiagnosticInfo(),
		Redaction: []string{
			"用户目录替换为 %USERPROFILE%",
			"配置中 password/secret/token/uuid/cipher/psk/auth/url/server 字段值替换为 ***REDACTED***",
		},
	}

	client := sys.NewHelperClient()
	if detail, err := client.CoreStatusDetail(); err == nil {
		bundle.CoreStatus = detail
	} else {
		bundle.HelperError = err.Error()
	}

	bundle.LogFiles = listDiagnosticFiles(logDir(), logFileMaxAge, 0)
	bundle.AssetFiles = listDiagnosticFiles(utils.GetCoreBinDir(), logFileMaxAge, 0)
	bundle.ConfigFile = redactPath(utils.GetRuntimeConfigPath())
	return bundle
}

func zipWriteBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// zipAddLogs 把主进程 JSONL 与 helper 侧内核输出日志一起打进包里（F-A3-13）。
func zipAddLogs(zw *zip.Writer, bundle DiagnosticBundle) error {
	var total int64
	for _, item := range bundle.LogFiles {
		if total >= diagMaxTotalBytes {
			break
		}
		src := filepath.Join(logDir(), item.Name)
		n, err := zipAddFile(zw, "logs/"+item.Name, src)
		if err != nil {
			continue // 单个文件被占用/删除不影响整体导出
		}
		total += n
	}
	return nil
}

func zipAddAssets(zw *zip.Writer, bundle DiagnosticBundle) error {
	candidates := map[string]string{
		"assets/asset-state.json":     filepath.Join(utils.GetCoreBinDir(), "asset-state.json"),
		"assets/seed-manifest.json":   utils.GetSeedAssetManifestPath(),
		"assets/install-profile.json": filepath.Join(utils.GetAppDir(), "install-profile.json"),
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	// 固定顺序，便于复现 diff
	sort.Strings(names)
	for _, name := range names {
		if _, err := zipAddFile(zw, name, candidates[name]); err != nil {
			continue
		}
	}
	return nil
}

// zipAddConfig 打入脱敏后的运行期配置（订阅 URL / 凭据 / 节点地址都会被打码）。
func zipAddConfig(zw *zip.Writer) error {
	src := utils.GetRuntimeConfigPath()
	data, err := os.ReadFile(src)
	if err != nil {
		return nil // 配置不存在不算导出失败
	}
	return zipWriteBytes(zw, "config/runtime-config.redacted.yaml", redactYAML(data))
}

func zipAddFile(zw *zip.Writer, name, src string) (int64, error) {
	f, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if st.IsDir() {
		return 0, fmt.Errorf("is dir")
	}

	w, err := zw.Create(name)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, f)
	if err != nil {
		return n, err
	}
	return n, nil
}

// buildSystemSummary 生成人类可读的 system.txt（同样做路径脱敏）。
func buildSystemSummary(bundle DiagnosticBundle) string {
	var sb strings.Builder
	sb.WriteString("GoclashZ diagnostics\n")
	sb.WriteString("generatedAt: " + bundle.GeneratedAt + "\n")
	sb.WriteString("version: " + bundle.Info.AppVersion + "\n")
	sb.WriteString("go: " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\n")
	sb.WriteString(fmt.Sprintf("pid: %d\n", os.Getpid()))
	sb.WriteString("appDir: " + redactPath(bundle.Info.AppDir) + "\n")
	sb.WriteString("dataDir: " + redactPath(bundle.Info.DataDir) + "\n")
	sb.WriteString("logDir: " + redactPath(bundle.Info.LogDir) + "\n")
	sb.WriteString(fmt.Sprintf("isAdmin: %v\n", bundle.Info.IsAdmin))
	sb.WriteString("helperService: " + bundle.Info.HelperServiceStatus + "\n")
	if bundle.HelperError != "" {
		sb.WriteString("helperError: " + bundle.HelperError + "\n")
	}
	if detail, ok := bundle.CoreStatus.(sys.CoreStatusDetail); ok {
		sb.WriteString(fmt.Sprintf("core: running=%v state=%s pid=%d consoleError=%q\n",
			detail.Running, detail.State, detail.PID, detail.ConsoleError))
		sb.WriteString(fmt.Sprintf("core.lastExit: code=%d at=%s pid=%d\n",
			detail.ExitCode, detail.ExitAt, detail.LastExitPID))
	}
	sb.WriteString("logs: " + fmt.Sprint(len(bundle.LogFiles)) + " file(s) included\n")
	for _, lf := range bundle.LogFiles {
		sb.WriteString(fmt.Sprintf("  - %s (%d bytes, %s)\n", lf.Name, lf.Size, lf.Mod))
	}
	sb.WriteString("redaction:\n")
	for _, r := range bundle.Redaction {
		sb.WriteString("  - " + r + "\n")
	}
	return sb.String()
}

// ensureTimestampSuffix 在文件名主干后补 "-YYYYMMDD-HHMMSS"，
// 避免多次导出互相覆盖（调用方沿用固定名也不会丢历史包）。
func ensureTimestampSuffix(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	if isTimestampTail(base) {
		return path
	}
	return filepath.Join(filepath.Dir(path), base+"-"+time.Now().Format("20060102-150405")+ext)
}

// isTimestampTail 判断字符串末尾是否已有 YYYYMMDD-HHMMSS。
func isTimestampTail(s string) bool {
	const layoutLen = 15 // 8 位日期 + '-' + 6 位时间
	if len(s) < layoutLen {
		return false
	}
	tail := s[len(s)-layoutLen:]
	for i := 0; i < layoutLen; i++ {
		if i == 8 {
			if tail[i] != '-' {
				return false
			}
			continue
		}
		if tail[i] < '0' || tail[i] > '9' {
			return false
		}
	}
	return true
}

// ---- 脱敏（F-A3-14） ----

// credentialKeys 命中即把 value 打码（包含匹配，覆盖 password/passwd/psk 等变体）。
var credentialKeys = []string{
	"password", "passwd", "secret", "token", "uuid", "cipher", "psk",
	"auth", "private-key", "api-key", "username", "url", "server",
}

// redactYAML 做行级脱敏：只处理 "key: value" 形式，保持缩进与注释，不解析 YAML。
// 刻意保持简单：宁可多打码，也不引入 YAML 解析依赖/失败路径。
func redactYAML(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		key = strings.TrimPrefix(key, "- ")
		for _, k := range credentialKeys {
			if strings.Contains(key, k) {
				lines[i] = line[:idx+1] + " \"***REDACTED***\""
				break
			}
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// redactPath 把用户目录替换成 %USERPROFILE%（Windows 路径大小写不敏感，故用 EqualFold）。
func redactPath(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if len(p) >= len(home) && strings.EqualFold(p[:len(home)], home) {
		return "%USERPROFILE%" + p[len(home):]
	}
	return p
}
