//go:build windows

package clash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

// 🔎 配置偏差检测
//
// 只报「用户没有主动关闭、但设置仍然被丢弃或改写」的分支：每条规则都必须在代码里真实存在
// （见注释里的 file:line），最多 3 条，宁少勿错。用户主动关掉的功能（例如关闭 DNS、关闭外部控制）
// 属于用户意图本身，不算偏差，不在这里出现。
// 文案不在 Go 里拼：Field / Reason 只返回 LocalizedText{Code}，由前端 i18n 渲染。

// LocalizedText 本地化文案契约（配置偏差与 DNS 解释共用）：
// Go 侧只给标识，前端按 code 到 deviation.field.* / deviation.reason.* / diag.note.* 取文案，
// Arg 是唯一的插值参数（前端占位符固定为 {arg}）。这样加语言不用改后端。
type LocalizedText struct {
	Code string `json:"code"`
	Arg  string `json:"arg,omitempty"`
}

// 配置偏差的文案 code：deviation.field.<code> / deviation.reason.<code>
const (
	deviationCodeHTTPPort   = "httpPort"
	deviationCodeLanAuth    = "lanAuth"
	deviationCodeCongestion = "congestion"
)

// ConfigDeviation 一条「界面上能改，但最终没有按设置生效」的说明
type ConfigDeviation struct {
	Key      string        `json:"key"`      // 稳定语义 id，前端按它去重/定位：network.httpPort / lan.auth / tun.congestion
	Severity string        `json:"severity"` // ignored（未生效）/ forced（被强制改值）
	Module   string        `json:"module"`   // tun / network / lan
	Field    LocalizedText `json:"field"`    // 字段名文案：deviation.field.<code>
	Reason   LocalizedText `json:"reason"`   // 原因文案：deviation.reason.<code>
}

const (
	// deviationSeverityIgnored 设置写进了配置文件，但没有被应用到内核
	deviationSeverityIgnored = "ignored"
	// deviationSeverityForced 设置被程序强制改写成了别的值
	deviationSeverityForced = "forced"

	deviationModuleTun     = "tun"
	deviationModuleNetwork = "network"
	deviationModuleLan     = "lan"
)

// GetConfigDeviations 返回当前设置下「不会按字面生效」的配置项。
// module 为空表示返回全部模块，否则只返回指定模块（tun / network / lan）。
func GetConfigDeviations(module string) ([]ConfigDeviation, error) {
	module = strings.ToLower(strings.TrimSpace(module))

	all := collectConfigDeviations()

	deviations := make([]ConfigDeviation, 0, len(all))
	for _, deviation := range all {
		if module != "" && deviation.Module != module {
			continue
		}
		deviations = append(deviations, deviation)
	}

	return deviations, nil
}

// collectConfigDeviations 逐条检查会产生偏差的条件分支，最多 3 条。
// 每条的注释都标出对应实现所在的 file:line，便于维护时对照。
func collectConfigDeviations() []ConfigDeviation {
	deviations := make([]ConfigDeviation, 0, 3)

	// 网络设置被下面两条规则共用，只读一次
	var networkSetting struct {
		Port        int    `json:"port"`
		MixedPort   int    `json:"mixedPort"`
		LanAuthUser string `json:"lanAuthUser"`
		LanAuthPass string `json:"lanAuthPass"`
	}
	networkOK := readRawUserSetting("network", &networkSetting)

	// ① 同时填写混合端口与 HTTP 端口时，HTTP 端口会被删掉。
	//    依据 core/clash/config.go:548-561 —— MixedPort != 0 时写 root["mixed-port"] 并 delete(root, "port")
	if networkOK && networkSetting.MixedPort != 0 && networkSetting.Port != 0 {
		deviations = append(deviations, ConfigDeviation{
			Key:      "network.httpPort",
			Severity: deviationSeverityIgnored,
			Module:   deviationModuleNetwork,
			Field:    LocalizedText{Code: deviationCodeHTTPPort},
			Reason:   LocalizedText{Code: deviationCodeHTTPPort},
		})
	}

	// ② 局域网认证只填密码不填用户名时，整条认证配置被丢弃。
	//    依据 core/clash/config.go:575-580 —— LanAuthUser 为空时 delete(root, "authentication")
	if networkOK && strings.TrimSpace(networkSetting.LanAuthUser) == "" && strings.TrimSpace(networkSetting.LanAuthPass) != "" {
		deviations = append(deviations, ConfigDeviation{
			Key:      "lan.auth",
			Severity: deviationSeverityIgnored,
			Module:   deviationModuleLan,
			Field:    LocalizedText{Code: deviationCodeLanAuth},
			Reason:   LocalizedText{Code: deviationCodeLanAuth},
		})
	}

	var tunSetting struct {
		Stack                string `json:"stack"`
		CongestionController string `json:"congestionController"`
	}
	if readRawUserSetting("tun", &tunSetting) {
		stack := strings.ToLower(strings.TrimSpace(tunSetting.Stack))
		congestion := strings.TrimSpace(tunSetting.CongestionController)

		// ③ 非 MIPS 协议栈下，拥塞控制算法会被清空。
		//    依据 core/clash/config.go:491-494 —— `if tunRuntime.Stack != "mips" { tunRuntime.CongestionController = "" }`
		//    选择「跟随配置文件」(follow) 时实际栈由订阅决定，无法判断，故不报。
		if stack != "" && stack != "mips" && stack != "follow" && congestion != "" {
			deviations = append(deviations, ConfigDeviation{
				Key:      "tun.congestion",
				Severity: deviationSeverityForced,
				Module:   deviationModuleTun,
				Field:    LocalizedText{Code: deviationCodeCongestion},
				Reason:   LocalizedText{Code: deviationCodeCongestion},
			})
		}

		// 说明：这里不再报「MIPS 被降级成 Mixed」。
		// clash 包内唯一的合并读取入口 GetTunConfig() 在读取时就会调用 AdjustTunConfigForCore
		// （core/clash/config.go:234-237）把不受支持的 mips 改成 mixed 并落盘，
		// 因此从「合并后的 tun 配置」里永远看不到本应降级的现场；为不误报，该规则按「宁少勿错」删除。
	}

	return deviations
}

// readRawUserSetting 原样读取 Settings/user_<name>.json，不做默认值合并、也不写盘。
// 之所以不用 utils.LoadSetting：诊断接口绝不能产生副作用（LoadSetting 会顺手回写补全后的文件）。
func readRawUserSetting(name string, out any) bool {
	data, err := os.ReadFile(filepath.Join(utils.GetSettingsDir(), "user_"+name+".json"))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, out) == nil
}
