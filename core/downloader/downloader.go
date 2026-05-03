//go:build windows

package downloader

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	URLs            []string // 馃殌 1. 绔為€熷鐏撅細浼犲叆澶氫釜涓嬭浇鍦板潃锛堝惈闀滃儚锛夛紝绗竴涓€氬父涓轰富绔?
	DestPath        string
	UserAgent       string
	MaxBytes        int64
	Client          *http.Client
	Resume          bool
	VerifyGitHubSHA bool
	RequireGitHubSHA bool                       // 馃殌 鏂板锛歋HA 涓哄彲閫夛紝Validator 涓哄繀椤?(鏀寔 digest 缂哄け鍦烘櫙)

	ProxyURL           string                     // 馃殌 2. 鑷唬鐞嗗姞閫燂細鎸囧畾鏈湴 Clash 浠ｇ悊鍦板潃
	InsecureSkipVerify bool                       // 馃殌 3. SSL瀹藉锛氭棤瑙嗛噹楦℃満鍦虹殑杩囨湡璇佷功
	OnResponse         func(resp *http.Response)  // 鎷︽埅鍣細渚涘閮ㄦ彁鍙?subscription-userinfo 澶?
	Validator          func(tmpPath string) error // 馃殌 4. 闃叉崯灞忛殰锛氭浛鎹㈠墠鎵ц楠岃瘉閫昏緫 (鏉滅粷鍧忔枃浠?
}

type resumeMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	TotalSize    int64  `json:"totalSize"`
}

var defaultClient = &http.Client{
	Timeout: 60 * time.Second,
}

var largeClient = &http.Client{
	Timeout: 10 * time.Minute,
}

// createClients 鐢熸垚缃戠粶鐜鐭╅樀锛堢洿杩?vs 浠ｇ悊锛?
func createClients(opt Options) []*http.Client {
	if opt.Client != nil {
		return []*http.Client{opt.Client}
	}

	var clients []*http.Client
	tlsConfig := &tls.Config{InsecureSkipVerify: opt.InsecureSkipVerify}

	// 馃殌 [寮曟搸 A]锛氱洿杩炲鎴风 (System Direct)
	directTransport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment, // 璧扮郴缁熼粯璁?
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: 5 * time.Second,
		// 馃専 鏍稿績淇锛氬交搴曠鐢?Keep-Alive锛岄槻姝㈠湪鈥滈槄鍚庡嵆鐒氣€濈殑绔為€熷満鏅笅浜х敓澶ч噺鐨勫菇鐏靛崗绋嬫硠闇?
		DisableKeepAlives: true,
	}
	clients = append(clients, &http.Client{
		Timeout:   10 * time.Minute,
		Transport: directTransport,
	})

	// 馃殌 [寮曟搸 B]锛氳嚜浠ｇ悊瀹㈡埛绔?(Self-Proxy)
	if opt.ProxyURL != "" {
		if pURL, err := url.Parse(opt.ProxyURL); err == nil {
			proxyTransport := &http.Transport{
				Proxy:               http.ProxyURL(pURL), // 寮鸿璧版湰鍦?Clash 绔彛
				TLSClientConfig:     tlsConfig,
				TLSHandshakeTimeout: 5 * time.Second,
				// 馃専 鏍稿績淇锛氬悓鏍峰交搴曠鐢?Keep-Alive
				DisableKeepAlives: true,
			}
			clients = append(clients, &http.Client{
				Timeout:   10 * time.Minute,
				Transport: proxyTransport,
			})
		}
	}

	return clients
}

func metaPath(tmpPath string) string {
	return tmpPath + ".meta.json"
}

func readResumeMeta(path string) (resumeMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return resumeMeta{}, false
	}

	var meta resumeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return resumeMeta{}, false
	}
	return meta, true
}

func writeResumeMeta(path string, meta resumeMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

var destLocks sync.Map // map[string]*sync.Mutex

func lockDest(path string) func() {
	clean := filepath.Clean(path)

	abs, err := filepath.Abs(clean)
	if err != nil {
		abs = clean
	}

	// 🚀 核心修复：Windows 路径大小写不敏感，统一转小写，彻底防止同一路径绕过锁
	abs = strings.ToLower(abs)

	v, _ := destLocks.LoadOrStore(abs, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()

	return func() {
		mu.Unlock()
	}
}

func probeSingleRemote(ctx context.Context, client *http.Client, testURL string, opt Options) (resumeMeta, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, testURL, nil)
	if err != nil {
		return resumeMeta{}, false, err
	}

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return resumeMeta{}, false, err // 鉁?淇锛氬悜涓婃姏鍑虹湡瀹炵綉缁滄姤閿?
	}

	// 鉁?淇锛氬緢澶氭満鍦洪潰鏉垮睆钄?HEAD 璇锋眰杩斿洖 405/403锛岃Е鍙?GET 闄嶇骇鎺㈡祴
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		req, _ = http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		req.Header.Set("User-Agent", ua)
		// 浣跨敤 Range 闄愬埗鍙姹備竴鐐圭偣鏁版嵁锛岄伩鍏嶆祴閫熸椂灏变笅杞戒簡鏁翠釜璁㈤槄
		req.Header.Set("Range", "bytes=0-0")
		resp, err = client.Do(req)
		if err != nil {
			return resumeMeta{}, false, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resumeMeta{}, false, fmt.Errorf("HTTP璇锋眰澶辫触锛岀姸鎬佺爜: %d", resp.StatusCode) // 鉁?淇锛氭姏鍑哄叿浣撻敊璇?
	}

	total := resp.ContentLength
	acceptRanges := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")

	return resumeMeta{
		URL:          resp.Request.URL.String(), // 璁板綍瀹為檯璧锋晥鐨勯噸瀹氬悜鐪熷疄鍦板潃
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		TotalSize:    total,
	}, acceptRanges && total > 0, nil
}

type envProbeResult struct {
	meta       resumeMeta
	canPartial bool
	client     *http.Client // 馃専 鏍稿績锛氳褰曟槸鍝釜瀹㈡埛绔紙鐜锛夎耽寰椾簡绔為€?
	err        error
}

// probeFastestEnvironment 骞跺彂绔為€燂細瀵绘壘鏈€蹇殑閾炬帴涓庢渶閫氱晠鐨勭綉缁滅幆澧?
func probeFastestEnvironment(ctx context.Context, clients []*http.Client, opt Options) (resumeMeta, bool, *http.Client, error) {
	urls := opt.URLs

	totalRaces := len(clients) * len(urls)
	resultCh := make(chan envProbeResult, totalRaces)

	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	// 寮€鍚?N x M 鍗忕▼鐭╅樀 (鐜 x 闀滃儚)
	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				meta, partial, err := probeSingleRemote(raceCtx, c, target, opt)
				if err == nil { // 鉁?绉婚櫎 TotalSize > 0 鐨勫己鍒朵緷璧?
					resultCh <- envProbeResult{meta, partial, c, nil}
				} else {
					resultCh <- envProbeResult{resumeMeta{}, false, nil, err}
				}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil { // 鉁?绉婚櫎 TotalSize > 0 鐨勫己鍒朵緷璧?
			cancelRace() // 馃専 鎵惧埌鏈€蹇殑閫氶亾锛岀珛鍒诲垏鏂叾浠栬惤鍚庡崗绋?
			return res.meta, res.canPartial, res.client, nil
		}
		lastErr = res.err
	}

	return resumeMeta{}, false, nil, fmt.Errorf("鎵€鏈夌綉缁滅幆澧冧笌闀滃儚鍧囨帰娴嬪け璐? %v", lastErr)
}

func parseContentRangeTotal(v string) (int64, bool) {
	idx := strings.LastIndex(v, "/")
	if idx < 0 || idx+1 >= len(v) {
		return 0, false
	}

	totalStr := strings.TrimSpace(v[idx+1:])
	if totalStr == "*" {
		return 0, false
	}

	total, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil || total <= 0 {
		return 0, false
	}
	return total, true
}

// FetchSmallFileAtomic 馃殌 楂樼骇骞跺彂涓嬭浇锛氶拡瀵瑰皬鏂囦欢锛堝璁㈤槄銆侀厤缃級鎵ц鈥滃叏閫熶綋鎰熺珵閫熲€?
// 瀹冧笉浠呭苟鍙戞祴璇曡繛鎺ワ紝杩樼洿鎺ュ苟鍙戜笅杞藉唴瀹广€傜涓€涓€氳繃 Validator 鏍￠獙鐨勮繛鎺ュ皢璧㈠緱姣旇禌銆?
func FetchSmallFileAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 涓虹┖")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	clients := createClients(opt)
	urls := opt.URLs
	totalRaces := len(clients) * len(urls)

	type result struct {
		body   []byte
		header http.Header
		err    error
	}
	resultCh := make(chan result, totalRaces)

	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				req, err := http.NewRequestWithContext(raceCtx, http.MethodGet, target, nil)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				req.Header.Set("User-Agent", ua)

				resp, err := c.Do(req)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					resultCh <- result{err: fmt.Errorf("HTTP %d", resp.StatusCode)}
					return
				}

				var reader io.Reader = resp.Body
				if opt.MaxBytes > 0 {
					reader = io.LimitReader(resp.Body, opt.MaxBytes+1)
				}

				body, err := io.ReadAll(reader)
				if err != nil {
					resultCh <- result{err: err}
					return
				}

				if opt.MaxBytes > 0 && int64(len(body)) > opt.MaxBytes {
					resultCh <- result{err: fmt.Errorf("鏂囦欢瓒呰繃澶у皬闄愬埗")}
					return
				}

				resultCh <- result{body: body, header: resp.Header, err: nil}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil && len(res.body) > 0 {
			// 1. 鍐欏叆涓存椂鏂囦欢锛屽噯澶団€滈獙姣掆€?
			tmpPath := opt.DestPath + ".tmp"
			if err := os.WriteFile(tmpPath, res.body, 0644); err != nil {
				lastErr = err
				continue
			}

			// 馃専 2. 鏍稿績淇锛氬厛鍏堥獙姣掞紝鍚庨濂栵紒
			// 濡傛灉鏍￠獙涓嶉€氳繃锛岃鏄庤閫氶亾鍙兘琚?WAF 鎷︽埅杩斿洖浜?HTML锛岀洿鎺ヤ涪寮冨苟缁х画绛夊緟鍏朵粬鎱㈤€熶絾姝ｇ‘鐨勯€氶亾
			if opt.Validator != nil {
				if err := opt.Validator(tmpPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = fmt.Errorf("鎺㈡祴鍒版棤鏁堟暟鎹?鍙兘琚嫤鎴?: %v", err)
					continue
				}
			}

			// 馃専 3. 鏍￠獙閫氳繃锛屽畠鏄湡鐨勶紒
			cancelRace() // 姝ゆ椂鎵嶆柀鏂叾浠栬繕鍦ㄨ窇鐨勫崗绋?

			if opt.OnResponse != nil {
				opt.OnResponse(&http.Response{Header: res.header})
			}

			return ReplaceFile(tmpPath, opt.DestPath)
		}
		if res.err != nil {
			lastErr = res.err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("鎵€鏈夌珵閫熼€氶亾鍧囧凡澶辨晥")
	}
	return fmt.Errorf("绔為€熶笅杞藉け璐? %v", lastErr)
}

func DownloadAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 涓虹┖")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	// 1. 鐢熸垚绔為€熺煩闃?(鐩磋繛 & 浠ｇ悊)
	clients := createClients(opt)

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

	var oldMeta resumeMeta
	var canResume bool
	var startOffset int64

	if opt.Resume {
		if info, err := os.Stat(tmpPath); err == nil && !info.IsDir() && info.Size() > 0 {
			if meta, ok := readResumeMeta(metaFile); ok {
				oldMeta = meta
				startOffset = info.Size()
				canResume = true
			}
		}
	} else {
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
	}

	// 2. 鍙戣捣鏋侀檺绔為€?(鐜 x 闀滃儚) 馃殌
	remoteMeta, serverSupportsRange, winningClient, err := probeFastestEnvironment(ctx, clients, opt)
	if err != nil {
		return err
	}

	if canResume {
		if oldMeta.ETag != "" && remoteMeta.ETag != "" && oldMeta.ETag != remoteMeta.ETag {
			canResume = false
		}
		if oldMeta.LastModified != "" && remoteMeta.LastModified != "" && oldMeta.LastModified != remoteMeta.LastModified {
			canResume = false
		}
		if oldMeta.TotalSize > 0 && remoteMeta.TotalSize > 0 && oldMeta.TotalSize != remoteMeta.TotalSize {
			canResume = false
		}
	}

	if !serverSupportsRange || !canResume || startOffset <= 0 {
		startOffset = 0
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
	}

	// 灏濊瘯涓嬭浇鎴栫画浼?
	if startOffset > 0 && remoteMeta.TotalSize > 0 && startOffset >= remoteMeta.TotalSize {
		// 宸蹭笅杞藉畬锛岃烦杩囦笅杞介樁娈?
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteMeta.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", ua)

		if startOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
			if remoteMeta.ETag != "" {
				req.Header.Set("If-Range", remoteMeta.ETag)
			} else if remoteMeta.LastModified != "" {
				req.Header.Set("If-Range", remoteMeta.LastModified)
			}
		}

		// 馃専 3. 浣跨敤璧㈠緱绔為€熺殑 client 鍙戣捣姝ｅ紡鐨勫ぇ鏂囦欢涓嬭浇
		resp, err := winningClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// 馃殌 鍦?resp 鑾峰彇鎴愬姛鍚庯紝绔嬪埢鏆撮湶鍑?Header 渚涘閮ㄨВ鏋?
		if opt.OnResponse != nil {
			opt.OnResponse(resp)
		}

		appendMode := false
		switch resp.StatusCode {
		case http.StatusOK:
			startOffset = 0
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
		case http.StatusPartialContent:
			appendMode = true
		default:
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("涓嬭浇澶辫触: HTTP %d", resp.StatusCode)
			}
		}

		if remoteMeta.ETag == "" {
			remoteMeta.ETag = resp.Header.Get("ETag")
		}
		if remoteMeta.LastModified == "" {
			remoteMeta.LastModified = resp.Header.Get("Last-Modified")
		}
		if remoteMeta.TotalSize <= 0 {
			if resp.StatusCode == http.StatusPartialContent {
				if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
					remoteMeta.TotalSize = total
				}
			} else if resp.ContentLength > 0 {
				remoteMeta.TotalSize = resp.ContentLength
			}
		}

		_ = writeResumeMeta(metaFile, remoteMeta)

		flag := os.O_CREATE | os.O_WRONLY
		if appendMode {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}

		out, err := os.OpenFile(tmpPath, flag, 0644)
		if err != nil {
			return err
		}

		var reader io.Reader = resp.Body
		if opt.MaxBytes > 0 {
			remain := opt.MaxBytes - startOffset
			if remain <= 0 {
				out.Close()
				return fmt.Errorf("涓嬭浇鏂囦欢瓒呰繃澶у皬闄愬埗")
			}
			reader = io.LimitReader(resp.Body, remain+1)
		}

		n, copyErr := io.Copy(out, reader)
		closeErr := out.Close()

		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}

		if opt.MaxBytes > 0 && startOffset+n > opt.MaxBytes {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
			return fmt.Errorf("涓嬭浇鏂囦欢瓒呰繃澶у皬闄愬埗")
		}
	}

	if remoteMeta.TotalSize > 0 {
		info, err := os.Stat(tmpPath)
		if err != nil {
			return err
		}
		if info.Size() < remoteMeta.TotalSize {
			return fmt.Errorf("涓嬭浇鏈畬鎴? %d/%d", info.Size(), remoteMeta.TotalSize)
		}
		if info.Size() > remoteMeta.TotalSize {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
			return fmt.Errorf("涓嬭浇鏂囦欢澶у皬寮傚父: %d/%d", info.Size(), remoteMeta.TotalSize)
		}
	}

	// 鏍￠獙閫昏緫
	if opt.VerifyGitHubSHA {
		// 鏍￠獙涔熼渶瑕佷娇鐢ㄨ幏鑳滅殑 client锛岀‘淇濈綉缁滈€氱晠
		expectedSHA, err := ResolveGitHubAssetSHA256(ctx, winningClient, remoteMeta.URL, ua)
		if err != nil {
			if opt.RequireGitHubSHA {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("鑾峰彇 GitHub 鏂囦欢鍝堝笇澶辫触: %v", err)
			}
			// digest 缂哄け鏃跺厑璁哥户缁紝浣嗗悗缁繀椤讳緷璧?Validator 鍋氭牸寮忔牎楠?
			expectedSHA = ""
		}

		if expectedSHA != "" {
			if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("鏂囦欢 SHA256 鏍￠獙澶辫触: %v", err)
			}
		}
	}

	// 馃殌 鍦?tmp 鏂囦欢鍐欏叆瀹屾瘯锛岄┈涓婅鎵ц ReplaceFile 涔嬪墠锛?
	if opt.Validator != nil {
		if err := opt.Validator(tmpPath); err != nil {
			_ = os.Remove(tmpPath) // 鏍￠獙澶辫触姣佸案鐏抗锛屼繚鎶ょ郴缁熼厤缃畨鍏?
			return fmt.Errorf("瀹夊叏鏍￠獙鎷︽埅, 鏂囦欢瀛樺湪鎹熷潖鎴栨牸寮忛敊璇? %v", err)
		}
	}

	if err := ReplaceFile(tmpPath, opt.DestPath); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
		return err
	}

	_ = os.Remove(metaFile)
	return nil
}

func ReplaceFile(tmpPath, destPath string) error {
	backupPath := destPath + ".bak"
	_ = os.Remove(backupPath)

	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, backupPath); err != nil {
			return fmt.Errorf("澶囦唤鏃ф枃浠跺け璐? %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Rename(backupPath, destPath)
		return fmt.Errorf("鏇挎崲鐩爣鏂囦欢澶辫触: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
}

func VerifySHA256(path string, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
type AppUpdateInfo struct {
	HasUpdate   bool     `json:"hasUpdate"`
	Version     string   `json:"version"`
	Body        string   `json:"body"`
	ReleaseURL  string   `json:"releaseUrl"`
	DownloadURL string   `json:"downloadUrl"`
	AssetName   string   `json:"assetName"`
	Assets      []string `json:"assets"`
}

var versionRe = regexp.MustCompile(`\d+(?:\.\d+){0,3}`)

var strictVersionRe = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+\.\d+(?:\.\d+)?(?:\.\d+)?)`)

func CheckAppUpdate(ctx context.Context, currentVersion string) (*AppUpdateInfo, error) {
	// 璇锋眰 GitHub API 鑾峰彇鏈€鏂?Release
	apiURL := "https://api.github.com/repos/Zzz-IT/GoclashZ/releases/latest"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "GoclashZ-Updater")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 杩斿洖 HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	// 馃殌 浣跨敤澧炲己鐨勬瘮瀵归€昏緫
	cmp, err := CompareAppVersion(release.TagName, currentVersion)
	if err != nil {
		return nil, err
	}
	hasUpdate := cmp > 0

	assetName, downloadURL := selectWindowsAsset(release.Assets)

	assetNames := make([]string, 0, len(release.Assets))
	for _, asset := range release.Assets {
		assetNames = append(assetNames, asset.Name)
	}

	return &AppUpdateInfo{
		HasUpdate:   hasUpdate,
		Version:     release.TagName,
		Body:        release.Body,
		ReleaseURL:  release.HTMLURL,
		DownloadURL: downloadURL,
		AssetName:   assetName,
		Assets:      assetNames,
	}, nil
}

func selectWindowsAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}) (string, string) {
	var fallbackName, fallbackURL string

	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)

		if !strings.HasSuffix(lower, ".exe") {
			continue
		}

		// 馃洝锔?榛戝悕鍗曪細杩欎簺涓嶆槸杞欢鏈綋锛屽摢鎬曞畠浠篃鏄?.exe
		if strings.Contains(lower, "mihomo") ||
			strings.Contains(lower, "clash") ||
			strings.Contains(lower, "wintun") ||
			strings.Contains(lower, "geoip") ||
			strings.Contains(lower, "geosite") ||
			strings.Contains(lower, "mmdb") ||
			strings.Contains(lower, "asn") {
			continue
		}

		// 馃洝锔?鐧藉悕鍗曪細蹇呴』鍖呭惈 goclashz锛屼笖鍊惧悜浜庡寘鍚?setup/installer/windows 绛夊叧閿瓧
		if strings.Contains(lower, "goclashz") {
			if strings.Contains(lower, "setup") ||
				strings.Contains(lower, "installer") ||
				strings.Contains(lower, "windows") ||
				strings.Contains(lower, "win") ||
				strings.Contains(lower, "x64") ||
				strings.Contains(lower, "amd64") {
				return asset.Name, asset.BrowserDownloadURL
			}

			if fallbackURL == "" {
				fallbackName = asset.Name
				fallbackURL = asset.BrowserDownloadURL
			}
		}
	}

	return fallbackName, fallbackURL
}

func CompareAppVersion(remote, current string) (int, error) {
	aa := parseVersionParts(remote)
	bb := parseVersionParts(current)

	if len(aa) == 0 {
		return 0, fmt.Errorf("鏃犳硶瑙ｆ瀽杩滅鐗堟湰: %s", remote)
	}
	if len(bb) == 0 {
		return 0, fmt.Errorf("鏃犳硶瑙ｆ瀽褰撳墠鐗堟湰: %s", current)
	}

	maxLen := len(aa)
	if len(bb) > maxLen {
		maxLen = len(bb)
	}

	// 琛ラ綈闀垮害
	for len(aa) < maxLen {
		aa = append(aa, 0)
	}
	for len(bb) < maxLen {
		bb = append(bb, 0)
	}

	for i := 0; i < maxLen; i++ {
		if aa[i] > bb[i] {
			return 1, nil
		}
		if aa[i] < bb[i] {
			return -1, nil
		}
	}
	return 0, nil
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(v)

	// 馃殌 鏍稿績鏀硅繘锛氫娇鐢ㄦ洿涓ユ牸鐨勬鍒欐彁鍙栫増鏈儴鍒嗭紝瑕佹眰鑷冲皯 x.y
	m := strictVersionRe.FindStringSubmatch(v)
	if len(m) < 2 {
		return nil
	}

	parts := strings.Split(m[1], ".")
	out := make([]int, 0, len(parts))

	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}

	return out
}

