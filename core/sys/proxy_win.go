//go:build windows

package sys

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	// 注册表中存储代理设置的路径
	internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	// wininet.dll 中用于通知系统刷新网络设置的常量
	INTERNET_OPTION_SETTINGS_CHANGED = 39
	INTERNET_OPTION_REFRESH          = 37
)

var (
	// 加载 Windows 底层网络库
	wininet           = syscall.NewLazyDLL("wininet.dll")
	internetSetOption = wininet.NewProc("InternetSetOptionW")
)

// notifyWinInet 发送系统广播，强制 Windows 重新读取注册表中的代理设置
func notifyWinInet() {
	internetSetOption.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	internetSetOption.Call(0, INTERNET_OPTION_REFRESH, 0, 0)
}

// SetSystemProxy 开启系统代理
func SetSystemProxy(server string, port int) error {
	// 打开注册表键 (允许写入)
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("无法打开注册表: %v", err)
	}
	defer k.Close()

	proxyAddress := fmt.Sprintf("%s:%d", server, port)
	// 绕过局域网和本地回环地址，防止本地应用（如 Wails 自身的前端）走代理死循环
	proxyOverride := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"

	// 1. 开启代理开关 (设为 1)
	err = k.SetDWordValue("ProxyEnable", 1)
	if err != nil {
		return err
	}

	// 2. 写入代理服务器地址 (127.0.0.1:7890)
	err = k.SetStringValue("ProxyServer", proxyAddress)
	if err != nil {
		return err
	}

	// 3. 写入绕过列表
	err = k.SetStringValue("ProxyOverride", proxyOverride)
	if err != nil {
		return err
	}

	// 4. ✨ 核心魔法：通知系统刷新，让浏览器立刻生效
	notifyWinInet()

	fmt.Printf("✅ Windows 系统代理已强行接管 -> %s\n", proxyAddress)
	return nil
}

// ClearSystemProxy 清理并关闭系统代理
func ClearSystemProxy() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("无法打开注册表: %v", err)
	}
	defer k.Close()

	// 将代理开关设为 0 (关闭)
	err = k.SetDWordValue("ProxyEnable", 0)
	if err != nil {
		return err
	}

	// 通知系统刷新
	notifyWinInet()

	fmt.Println("🛑 Windows 系统代理已恢复正常")
	return nil
}
