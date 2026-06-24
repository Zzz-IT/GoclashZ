//go:build windows

package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"goclashz/core/utils"
	syswin "golang.org/x/sys/windows"
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
	// 🚀 新增：恐慌恢复逻辑，确保程序因未知 Bug 崩溃时，能最后尝试清理一次代理
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("检测到程序 Panic: %v\n正在紧急清理系统代理...", r)
			sys.ClearOwnedSystemProxy()
			panic(r) // 继续抛出 Panic 以供日志记录
		}
	}()

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

	if hasFlag("--repair-permissions") {
		if !sys.CheckAdmin() {
			err := sys.RequestAdmin()
			if err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			os.Exit(0)
		}

		err := utils.RepairDataDirPermission()
		if err != nil {
			logger.Errorf("修复数据目录权限失败: %v", err)
			os.Exit(1)
		}
		logger.Infof("✅ 成功修复数据目录权限")
		os.Exit(0)
	}

	if hasFlag("--repair-core-layout") {
		if !sys.CheckAdmin() {
			err := sys.RequestAdmin()
			if err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			os.Exit(0)
		}

		clash.MigrateCoreAssetsToBin()
		err := utils.RepairCoreBinPermission()
		if err != nil {
			logger.Errorf("修复内核目录权限失败: %v", err)
			os.Exit(1)
		}
		logger.Infof("✅ 成功修复内核布局及权限")
		os.Exit(0)
	}

	// Helper 服务管理命令（由提权子进程执行）
	if hasFlag("--install-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			os.Exit(0)
		}

		// 解析 --allowed-sid 参数
		allowedSid := ""
		for i, arg := range os.Args {
			if arg == "--allowed-sid" && i+1 < len(os.Args) {
				allowedSid = os.Args[i+1]
				break
			}
		}

		exePath := filepath.Join(filepath.Dir(exePath), "GoclashZHelper.exe")
		if err := sys.InstallHelperServiceForUser(exePath, allowedSid); err != nil {
			logger.Errorf("安装 Helper 服务失败: %v", err)
			os.Exit(1)
		}
		if err := sys.StartHelperService(); err != nil {
			logger.Errorf("启动 Helper 服务失败: %v", err)
			os.Exit(1)
		}
		logger.Infof("✅ 成功安装并启动 Helper 服务")
		os.Exit(0)
	}

	if hasFlag("--uninstall-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			os.Exit(0)
		}
		if err := sys.UninstallHelperService(); err != nil {
			logger.Errorf("卸载 Helper 服务失败: %v", err)
			os.Exit(1)
		}
		logger.Infof("✅ 成功卸载 Helper 服务")
		os.Exit(0)
	}

	if hasFlag("--restart-helper") {
		if !sys.CheckAdmin() {
			if err := sys.RequestAdmin(); err != nil {
				logger.Errorf("请求管理员权限失败: %v", err)
			}
			os.Exit(0)
		}
		if err := sys.StopHelperService(); err != nil {
			logger.Warnf("停止 Helper 服务失败: %v", err)
		}
		if err := sys.StartHelperService(); err != nil {
			logger.Errorf("启动 Helper 服务失败: %v", err)
			os.Exit(1)
		}
		logger.Infof("✅ 成功重启 Helper 服务")
		os.Exit(0)
	}

	// 2. 单实例锁逻辑
	if !isDebugMode {
		mutexName, _ := syswin.UTF16PtrFromString("Global\\GoclashZ_Single_Instance_Mutex")
		mutexHandle, err := syswin.CreateMutex(nil, false, mutexName)
		
		// ✅ 核心修复：直接通过系统调用返回的 err 判断，切勿使用 GetLastError()
		if err != nil {
			if err == syswin.ERROR_ALREADY_EXISTS {
				logger.Infof("⚠️ GoclashZ 已经在后台运行，正在唤醒已有窗口...")
				
				// 🚀 核心重构：调用统一的唤醒与闪烁函数 (由 core/sys 提供)
				sys.FocusMainWindowAndFlashTwiceWin32Only()
				
				// 显式释放内核互斥锁句柄
				if mutexHandle != 0 {
					syswin.CloseHandle(mutexHandle)
				}
				os.Exit(0)
			} else {
				logger.Errorf("创建互斥锁发生异常: %v", err)
			}
		}

		// 确保当前程序真的退出时（而不是假死）再释放锁
		if mutexHandle != 0 {
			defer syswin.CloseHandle(mutexHandle)
		}

		// 🚀 核心自愈：我们不再无条件清理代理，而是交由 Supervisor 恢复机制
		// sys.ClearOwnedSystemProxy() // 移除此行

		// 🚀 核心保护：注册操作系统信号监听
		installEmergencyProxyCleanup()
	}

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

	err := wails.Run(&options.App{
		Title:  "GoclashZ",
		Width:  1024,
		Height: 768,
		MinWidth:  900, // 👈 核心修复：限制最小宽度，防止 UI 布局挤压
		MinHeight: 600, // 👈 核心修复：限制最小高度
		Frameless: true, // 保持无边框，自己渲染 UI
		
		HideWindowOnClose: true, // 👈 1. 新增：点击关闭按钮时，隐藏窗口而不是退出进程
		StartHidden:       true, // 👈 核心：启动时默认不弹窗，为“静默启动”铺垫
		
		// 2. 👈 使用动态读取的颜色
		BackgroundColour: &options.RGBA{R: r, G: g, B: b, A: 255}, 
		
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:             windows.SystemDefault, 
			DisableWindowIcon: false,
		},
	})

	if err != nil {
		logger.Errorf("Error: %v", err)
	}
}

// installEmergencyProxyCleanup 注册系统信号监听
func installEmergencyProxyCleanup() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		logger.Infof("检测到退出信号，正在清理 GoclashZ 设置的系统代理...")
		sys.ClearOwnedSystemProxy()
		os.Exit(0)
	}()
}
