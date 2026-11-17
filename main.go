//go:build windows

package main

import (
	"embed"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"goclashz/core/version"
	"os/signal"
	"syscall"
)

//go:embed all:frontend/dist
var assets embed.FS

func hasFlag(flag string) bool {
	for _, a := range os.Args {
		if a == flag {
			return true
		}
	}
	return false
}

func main() {
	// 🗂 0) 日志落盘最先初始化：此后任何一步（迁移/提权/Panic）都会留下痕迹。
	// 初始化失败不阻断启动，只是退化为「仅内存环」。
	if err := logger.InitFileSink(filepath.Join(utils.GetDataDir(), "logs")); err != nil {
		fmt.Fprintf(os.Stderr, "logger: init file sink failed: %v\n", err)
	}
	// 收编标准库 log：core/runtimeassets 的自愈日志走的是 log.Printf，
	// GUI 进程没有控制台，不接这里就全部丢失（审计 A3 F-A3-03）。
	// 注意顺序：StdWriter 在前。GUI 进程的 os.Stderr 句柄无效，io.MultiWriter 遇到
	// 写失败会立即返回、不再写给后续 writer —— 把文件 sink 放前面才能保证不丢。
	log.SetOutput(io.MultiWriter(logger.StdWriter(), os.Stderr))

	// 🚀 新增：恐慌恢复逻辑，确保程序因未知 Bug 崩溃时，能最后尝试清理一次代理
	defer func() {
		if r := recover(); r != nil {
			// 先落栈再 Flush：GUI 进程没有 stderr，栈只能靠文件 sink 带出去。
			logger.Errorf("检测到程序 Panic: %v\n%s\n正在紧急清理系统代理...", r, debug.Stack())
			logger.Flush()
			sys.ClearOwnedSystemProxy()
			panic(r) // 继续抛出 Panic 以供日志记录
		}
	}()

	// 启动上下文第一行：版本/路径/管理员/seed 目录/关键参数（boot.start）。
	writeBootStart()

	// 1. 判断是否为 Wails 开发模式 (放行 Debug)
	// 在 Wails Dev 模式下，通常可执行文件路径包含临时目录或 wails-dev
	exePath, _ := os.Executable()
	isDebugMode := false
	if filepath.Base(exePath) == "GoclashZ-dev.exe" || len(os.Getenv("WAILS_DEV_SERVER")) > 0 {
		isDebugMode = true
		logger.Infof("👉 Wails 开发模式，跳过单实例检查")
	}

	// 🚨 核心逻辑：数据目录稳定化及迁移
	// 必须在加载任何行为/期望状态/核心之前执行。
	if err := utils.MigrateLegacyAppDataToInstallData(); err != nil {
		logger.Errorf("旧数据迁移失败: %v", err)
	}

	if hasFlag("--shutdown-existing") {
		sys.RequestExistingInstanceQuit()
		exitProcess(0)
	}

	if hasFlag("--repair-permissions") {
		if !sys.CheckAdmin() {
			err := sys.RequestAdmin()
			if err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}

		err := utils.RepairDataDirPermission()
		if err != nil {
			logger.Errorf("修复数据目录权限失败: %v", err)
			exitProcess(1)
		}
		logger.Infof("✅ 成功修复数据目录权限")
		exitProcess(0)
	}

	if hasFlag("--repair-core-layout") {
		if !sys.CheckAdmin() {
			err := sys.RequestAdmin()
			if err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}

		runtimeassets.MigrateLegacyAssets()
		err := utils.RepairCoreBinPermission()
		if err != nil {
			logger.Errorf("修复内核目录权限失败: %v", err)
			exitProcess(1)
		}
		logger.Infof("✅ 成功修复内核布局及权限")
		exitProcess(0)
	}

	if hasFlag("--save-uwp-exemptions") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}

		filePath := ""
		for i, arg := range os.Args {
			if arg == "--save-uwp-exemptions" && i+1 < len(os.Args) {
				filePath = os.Args[i+1]
				break
			}
		}

		if filePath == "" {
			err := fmt.Errorf("未指定 UWP 规则临时文件路径")
			logger.Errorf("%v", err)
			sys.WriteAdminTaskResult("save-uwp-exemptions", err)
			exitProcess(1)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Errorf("读取 UWP 规则临时文件失败: %v", err)
			sys.WriteAdminTaskResult("save-uwp-exemptions", err)
			exitProcess(1)
		}

		var sids []string
		if err := jsonv2.Unmarshal(data, &sids); err != nil {
			logger.Errorf("解析 UWP 规则失败: %v", err)
			sys.WriteAdminTaskResult("save-uwp-exemptions", err)
			exitProcess(1)
		}

		if err := sys.SaveUwpExemptions(sids); err != nil {
			logger.Errorf("保存 UWP 规则失败: %v", err)
			sys.WriteAdminTaskResult("save-uwp-exemptions", err)
			exitProcess(1)
		}

		logger.Infof("成功保存 UWP 环回免除规则")
		sys.WriteAdminTaskResult("save-uwp-exemptions", nil)
		exitProcess(0)
	}

	// Helper 服务管理命令（由提权子进程执行）
	if hasFlag("--install-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}

		// 解析 --allowed-sid 参数
		allowedSid := ""
		for i, arg := range os.Args {
			if arg == "--allowed-sid" && i+1 < len(os.Args) {
				allowedSid = os.Args[i+1]
				break
			}
		}

		helperPath := filepath.Join(filepath.Dir(exePath), "GoclashZHelper.exe")
		if err := sys.RecoverHelperServiceForUser(helperPath, allowedSid); err != nil {
			logger.Errorf("修复 Helper 服务失败: %v", err)
			sys.WriteAdminTaskResult("install-helper", err)
			exitProcess(1)
		}

		logger.Infof("✅ 成功安装/修复并启动 Helper 服务")
		exitProcess(0)
	}

	if hasFlag("--uninstall-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}
		if err := sys.UninstallHelperService(); err != nil {
			logger.Errorf("卸载 Helper 服务失败: %v", err)
			exitProcess(1)
		}
		logger.Infof("✅ 成功卸载 Helper 服务")
		exitProcess(0)
	}

	if hasFlag("--restart-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			exitProcess(0)
		}
		if err := sys.StopHelperService(); err != nil {
			logger.Warnf("停止 Helper 服务失败: %v", err)
		}
		if err := sys.StartHelperService(); err != nil {
			logger.Errorf("启动 Helper 服务失败: %v", err)
			exitProcess(1)
		}
		logger.Infof("✅ 成功重启 Helper 服务")
		exitProcess(0)
	}

	// 2. 单实例：v2 的自研 Global\GoclashZ_Single_Instance_Mutex 互斥锁已删除，
	// 改由 v3 原生 application.SingleInstanceOptions 实现（见下方 application.Options）。
	// 第二实例会在 application.New 内部 notify 第一实例后直接 os.Exit(ExitCode)，
	// 不会进入 NewApp / 窗口 / startup 流程。

	app := NewApp()

	// 👇 修复：将默认兜底颜色改为夜间模式，对齐 app.go 的默认行为
	var r, g, b uint8 = 17, 17, 17 // 默认夜间底色 (#111111)

	// ✅ 使用统一的智能数据目录读取主题
	themeFile := filepath.Join(utils.GetDataDir(), "theme_setting.txt")
	// 🎯 修复：使用匿名函数建立独立的局部作用域，让 defer 立即执行
	func() {
		if f, err := os.Open(themeFile); err == nil {
			defer f.Close() // 现在它会在大括号结束时立刻执行，而不是等待 main() 结束

			buf := make([]byte, 16)
			n, _ := f.Read(buf)

			if n > 0 && strings.TrimSpace(string(buf[:n])) == "light" {
				r, g, b = 242, 242, 242
			}
		}
	}()

	// 👇 Wails v3 迁移：ServiceStartup 早于窗口实际创建执行，因此窗口可见性必须
	// 在创建窗口之前先算出来，再通过 WebviewWindowOptions.Hidden 一次性决定，
	// 避免在 ServiceStartup 里操作尚未创建的窗口。
	// 口径与 v2 完全一致：isSilent = --silent || behavior.SilentStart。
	// --startup 只影响 BootstrapOptions.IsStartupLaunch，不参与窗口可见性判定
	// （v2 中 --startup 且 SilentStart=false 时窗口照常显示）。
	// 同包可直接访问 a.core，与 app.go 启动逻辑读取同一份行为配置。
	isSilent := hasFlag("--silent") || app.core.Behavior.Get().SilentStart
	app.silentLaunch = isSilent

	// 👇 #5：WebView2 用户数据目录。v2 的默认值就是 %AppData%\GoclashZ.exe
	// （v3 缺省时按 %APPDATA%\[可执行文件名] 推导，两者本就一致），这里显式写死，
	// 避免未来 v3 默认值调整导致用户数据目录漂移。注意这是「应用级」Options.Windows
	// 字段，不是窗口级 WindowsWindow。
	// 极端情况下（AppData 环境变量缺失）回退为空串，交回 v3 默认逻辑，绝不传相对路径。
	webviewUserDataPath := ""
	if appData := os.Getenv("AppData"); appData != "" {
		webviewUserDataPath = filepath.Join(appData, "GoclashZ.exe")
	}

	// 👇 #7：v3 原生单实例。debug 模式保持 v2 语义（不启用单实例检查）。
	// 第二实例在 application.New 内部 notify 第一实例后 os.Exit(ExitCode)，不会进入本进程后续流程；
	// 回调则在「第一实例」进程内、由 framework goroutine 执行：
	//   - 第二实例带 --silent / --startup（开机自启或静默拉起）→ 直接返回，绝不打扰已有窗口；
	//   - 其余情况 → 还原/显示/激活主窗口并闪烁任务栏，等价 v2 的唤醒逻辑。
	var singleInstanceOptions *application.SingleInstanceOptions
	if !isDebugMode {
		singleInstanceOptions = &application.SingleInstanceOptions{
			UniqueID: "GoclashZ_Single_Instance_Mutex",
			ExitCode: 0,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				for _, arg := range data.Args {
					if arg == "--silent" || arg == "--startup" {
						logger.Infof("⚠️ GoclashZ 已有实例在运行，当前为静默/自启模式，直接退出，不唤醒已有窗口")
						return
					}
				}

				logger.Infof("⚠️ GoclashZ 已经在后台运行，正在唤醒已有窗口...")

				// #2：不再 GetByName，直接用 App 持有的窗口引用（nil 安全）。
				if w := app.mainWindow(); w != nil {
					w.Restore()
					w.Show()
					w.Focus()
					w.UnMaximise()
				}

				// v2 的纯 Win32 唤醒 + 任务栏闪烁兜底（core/sys 原实现保留）
				sys.FocusMainWindowAndFlashTwiceWin32Only()
			},
		}
	}

	// 👇 Wails v3 应用实例：108 个绑定方法按领域拆到 3 个转发服务（#8），
	// 每个方法体只转发到 *App，方法名/签名/行为与拆分前完全一致。
	// 注册顺序 App → Core → Update：ServiceStartup/ServiceShutdown 由 AppService 承接，
	// 启动/关闭通知顺序与拆分前单服务时一致。退出清理仍由 App.ServiceShutdown 承担，
	// 不再使用 application.Options.OnShutdown。
	wailsApp := application.New(application.Options{
		Name:        "GoclashZ",
		Description: "GoclashZ",
		// #6：v3.1.0 在 Windows 上实际由 exe 资源（build/windows/*.syso）决定图标，
		// 本字段在 Windows 基本无效，保留只为 API 完整性。
		Icon: iconData,
		// #8：三个转发服务，方法体只调用 *App 上同名方法；归属表见 services_v3_mapping.go。
		Services: []application.Service{
			application.NewService(&AppService{app: app}),
			application.NewService(&CoreService{app: app}),
			application.NewService(&UpdateService{app: app}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: webviewUserDataPath,
		},
		// 🗂 F-A3-06：把 v3 框架层的日志接进 logger（→ 内存环 + JSONL 文件）。
		// 不设置时，生产构建的默认 logger 是 slog.NewTextHandler(io.Discard)，
		// WebView2/绑定/事件校验等框架错误会全部静默消失。
		Logger: slog.New(logger.SlogHandler()),
		// 🗂 F-A3-06：ErrorHandler 接手框架错误。v3 的事件类型校验失败
		// （"does not match registered data type"）会走到这里，落一条 warn，
		// 避免「事件静默不来」只能靠猜。
		ErrorHandler: func(err error) {
			logger.LogFrameworkError(err)
		},
		// #3：v3.1.0 仅 macOS 的 shouldQuitApplication 消费 ShouldQuit，Windows 没有任何
		// 消费点。恒返回 true 表示不拦截系统退出请求，Windows 行为与现状零变化。
		ShouldQuit: func() bool { return true },
		// #7：v3 原生单实例（debug 模式为 nil，即不启用）
		SingleInstance: singleInstanceOptions,
	})

	// 只有真正拿到单实例锁、进入 GUI 主流程的进程才写运行标记：
	// 第二实例会在上面的 application.New 内直接 os.Exit，若提前写标记会被误判成
	// 「上次异常退出」，而一次性提权/修复命令则在更早的分支就已退出。
	beginRunMarker()

	// 🚀 v2 语义保持：旧计划任务清理与「安装器请求退出」信号监听必须只由真正持有
	// 单实例锁的进程执行。v3 的第二实例会在上面的 application.New 内直接 os.Exit，
	// 因此这两项副作用从 v2 的「互斥锁成功之后」平移到「application.New 成功之后」。
	if !isDebugMode {
		// 🚀 核心自愈：清理可能残留的旧计划任务（防止开机时同时触发注册表Run与计划任务导致双启碰撞）
		go sys.DeleteLegacyTaskSchedulerTask()

		// 🚀 核心保护：注册操作系统信号监听
		installEmergencyProxyCleanup()
	}

	// 👇 Wails v3 主窗口：字段逐一对应 v2 的 options.App 窗口配置。
	mainWin := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "GoclashZ",
		Width:     1024,
		Height:    768,
		MinWidth:  900,  // 👈 核心修复：限制最小宽度，防止 UI 布局挤压
		MinHeight: 600,  // 👈 核心修复：限制最小高度
		Frameless: true, // 保持无边框，自己渲染 UI

		// 👈 v2 的 StartHidden:true + startup 内 Show/Hide 改为 Hidden: isSilent
		Hidden: isSilent,
		// 👈 使用动态读取的颜色
		BackgroundColour: application.NewRGB(r, g, b),
		// 原生 Acrylic 需要 translucent 窗口 + 透明 WebView；
		// 关闭玻璃效果时由前端不透明背景盖住，不影响正常观感。
		BackgroundType: application.BackgroundTypeTranslucent,

		Windows: application.WindowsWindow{
			Theme:        application.SystemDefault, // 对齐 v2 windows.Options.Theme
			DisableIcon:  false,                     // 对齐 v2 DisableWindowIcon:false
			BackdropType: application.None,          // 启动时先关闭，运行时按设置开启
		},
	})

	// 👈 #2：把主窗口引用交给 App，后续所有窗口操作都不再做 GetByName 查找。
	app.win = mainWin

	// 👈 v3 没有 HideWindowOnClose：拦截关闭事件，取消关闭并隐藏窗口，等价 v2 行为。
	// 必须用 RegisterHook 而不是 OnWindowEvent：v3.1.0 的 hooks 同步先执行、cancel 能阻断
	// 框架内部 WindowClosing listener 的销毁；listener 是并发 goroutine，会与销毁竞态。
	mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		mainWin.Hide()
	})

	// 👈 #4：runtime 加载完成后再兜底隐藏一次。Hidden:isSilent 已在创建期去掉
	// WS_VISIBLE（杜绝首帧白闪），这里防止 WebView2 初始化/系统消息让静默启动的
	// 窗口露头；原来 ServiceStartup 里的 sys.SafeHideMainWindowWin32 /
	// sys.EnsureMainWindowHiddenSilently 兜底已删除（core/sys 内函数保留不动）。
	mainWin.OnWindowEvent(events.Common.WindowRuntimeReady, func(e *application.WindowEvent) {
		// 必须在 Wails 完成原生窗口创建（setBackdropType）之后再应用；
		// 提前调用会被框架初始化时的 BackdropType=None 覆盖。
		behavior := app.core.Behavior.Get()
		// strength 统一用「背景着色」(GlassTintStrength)：它与 SaveAppBehavior 里的调用保持一致，
		// 否则启动瞬间与保存后两次 tint alpha 不同，用户会看到一次观感跳变。
		sys.ApplyWindowGlass(uintptr(mainWin.NativeWindow()), behavior.GlassBlurEnabled, behavior.GlassTintStrength)
		if isSilent {
			mainWin.Hide()
		}
	})

	if err := wailsApp.Run(); err != nil {
		logger.Errorf("Error: %v", err)
	}

	// 正常退出路径：boot.exit + 清运行标记 + Flush（Flush 失败也不阻断退出）。
	finishRunMarker()
}

// installEmergencyProxyCleanup 注册系统信号监听
func installEmergencyProxyCleanup() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		logger.Infof("检测到退出信号，正在清理 GoclashZ 设置的系统代理...")
		sys.ClearOwnedSystemProxy()
		exitProcess(0)
	}()

	go func() {
		sys.ListenForShutdownEvent()
		logger.Infof("检测到安装器请求退出信号，正在紧急清理并退出...")
		sys.ClearOwnedSystemProxy()
		exitProcess(0)
	}()
}

// ---- 启动上下文 / 运行标记（审计 A3 F-A3-02） ----

// runMarker 是判断「上次是否异常退出」的落盘标记。
type runMarker struct {
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
}

// runMarkerActive 表示本进程已写入运行标记：受控退出路径负责删除它。
var runMarkerActive bool

func runMarkerPath() string {
	return filepath.Join(utils.GetRuntimeDir(), "run.marker")
}

// bootLog 写一条带 event 名的启动日志；同时进入内存环与 JSONL 文件。
func bootLog(level, event, format string, args ...any) {
	logger.AppLogs.Add(logger.LogEntry{
		Type:    level,
		Source:  logger.LogSourceApp,
		Payload: fmt.Sprintf(format, args...),
		Event:   event,
	})
}

// writeBootStart 在 main 最早期记录一次启动上下文（版本/路径/权限/seed/参数）。
func writeBootStart() {
	bootLog("info", "boot.start",
		"启动 pid=%d version=%s appDir=%s dataDir=%s seedCoreBinDir=%s isAdmin=%v silent=%v startup=%v args=[%s]",
		os.Getpid(), version.AppVersion, utils.GetAppDir(), utils.GetDataDir(), utils.GetSeedCoreBinDir(),
		sys.CheckAdmin(), hasFlag("--silent"), hasFlag("--startup"), strings.Join(os.Args[1:], " "))
}

// beginRunMarker 判断上一次是否异常退出，并写入本次运行标记。
func beginRunMarker() {
	path := runMarkerPath()

	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		detail := strings.TrimSpace(string(data))
		var prev runMarker
		if jsonv2.Unmarshal(data, &prev) == nil {
			detail = fmt.Sprintf("pid=%d version=%s startedAt=%s", prev.PID, prev.Version, prev.StartedAt)
		}
		bootLog("warn", "boot.previousRunUnclean", "上次运行未正常退出（残留标记 %s）：%s", path, detail)
	}

	marker := runMarker{PID: os.Getpid(), Version: version.AppVersion, StartedAt: time.Now().Format(time.RFC3339)}
	data, err := jsonv2.Marshal(marker)
	if err != nil {
		logger.Warnf("编码运行标记失败: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warnf("创建 runtime 目录失败: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Warnf("写入运行标记失败: %v", err)
		return
	}
	runMarkerActive = true
}

// finishRunMarker 正常退出：记录 boot.exit、删除标记、Flush。
func finishRunMarker() {
	if n := logger.Dropped(); n > 0 {
		bootLog("warn", "boot.logDropped", "本次运行有 %d 条日志未能落盘（队列满或写盘失败）", n)
	}
	bootLog("info", "boot.exit", "正常退出 pid=%d", os.Getpid())
	if runMarkerActive {
		if err := os.Remove(runMarkerPath()); err != nil && !os.IsNotExist(err) {
			logger.Warnf("清理运行标记失败: %v", err)
		}
		runMarkerActive = false
	}
	logger.Flush()
}

// exitProcess 是所有受控退出路径的统一出口：清标记 + Flush 后再退出，
// 避免「日志只在内存里」——审计 A3 F-A3-02 的核心修复点之一。
func exitProcess(code int) {
	if runMarkerActive {
		_ = os.Remove(runMarkerPath())
		runMarkerActive = false
	}
	// 最后一道保险（F-A5-13）：退出链路上任何一步被硬超时截断时，
	// 至少在这里把我们自己写入的系统代理摘掉，避免用户断网。
	// ClearOwnedSystemProxy 自带归属校验（所有权文件 + 注册表匹配），不会误伤其它工具。
	sys.ClearOwnedSystemProxy()
	logger.Flush()
	os.Exit(code)
}
