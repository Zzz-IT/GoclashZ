//go:build windows

package sys

import (
	"fmt"
	"unsafe"
	"golang.org/x/sys/windows"
)

// 定义 Windows API 需要的结构体
type INET_FIREWALL_APP_CONTAINER struct {
	AppContainerSid     *windows.SID
	UserSid             *windows.SID
	AppContainerName    *uint16
	DisplayName         *uint16
	Description         *uint16
	Capabilities        uintptr
	Binaries            uintptr
	WorkingDir          *uint16
	PackageFullName     *uint16
}

type UwpApp struct {
	DisplayName       string `json:"displayName"`
	PackageFamilyName string `json:"packageFamilyName"`
	SID               string `json:"sid"`
	IsEnabled         bool   `json:"isEnabled"`
}

var (
	firewallAPI = windows.NewLazySystemDLL("FirewallAPI.dll")
	
	// API 声明
	procEnumAppContainers      = firewallAPI.NewProc("NetworkIsolationEnumAppContainers")
	procFreeAppContainers      = firewallAPI.NewProc("NetworkIsolationFreeAppContainers")
	procGetAppContainerConfig  = firewallAPI.NewProc("NetworkIsolationGetAppContainerConfig")
	procSetAppContainerConfig  = firewallAPI.NewProc("NetworkIsolationSetAppContainerConfig")
)

// GetUwpAppList 获取所有 UWP 应用及其当前的环回豁免状态
func GetUwpAppList() ([]UwpApp, error) {
	var count uint32
	var pAppContainers unsafe.Pointer

	// 1. 枚举所有 UWP 容器
	ret, _, _ := procEnumAppContainers.Call(
		0,
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&pAppContainers)),
	)
	if ret != 0 {
		return nil, fmt.Errorf("枚举 UWP 容器失败: %v", ret)
	}
	defer procFreeAppContainers.Call(uintptr(pAppContainers))

	// 2. 获取当前已开启豁免的应用 SID 列表
	var exemptCount uint32
	var pExemptSids unsafe.Pointer
	procGetAppContainerConfig.Call(
		uintptr(unsafe.Pointer(&exemptCount)),
		uintptr(unsafe.Pointer(&pExemptSids)),
	)

	// 将豁免 SID 存入 Map 方便快速查询
	exemptMap := make(map[string]bool)
	if pExemptSids != nil {
		type SID_AND_ATTRIBUTES struct {
			Sid        *windows.SID
			Attributes uint32
		}
		sids := unsafe.Slice((*SID_AND_ATTRIBUTES)(pExemptSids), exemptCount)
		for _, sa := range sids {
			exemptMap[sa.Sid.String()] = true
		}
	}

	// 3. 解析枚举结果并组装数据
	var apps []UwpApp
	containerPtrs := unsafe.Slice((**INET_FIREWALL_APP_CONTAINER)(pAppContainers), count)

	for _, container := range containerPtrs {
		sidStr := container.AppContainerSid.String()
		apps = append(apps, UwpApp{
			DisplayName:       windows.UTF16PtrToString(container.DisplayName),
			PackageFamilyName: windows.UTF16PtrToString(container.AppContainerName),
			SID:               sidStr,
			IsEnabled:         exemptMap[sidStr],
		})
	}

	return apps, nil
}

// SaveUwpExemptions 批量保存豁免配置
func SaveUwpExemptions(sids []string) error {
	type SID_AND_ATTRIBUTES struct {
		Sid        *windows.SID
		Attributes uint32
	}
	
	items := make([]SID_AND_ATTRIBUTES, 0)
	for _, s := range sids {
		sid, err := windows.StringToSid(s)
		if err != nil {
			continue
		}
		items = append(items, SID_AND_ATTRIBUTES{Sid: sid, Attributes: 0})
	}

	var ptr uintptr
	if len(items) > 0 {
		ptr = uintptr(unsafe.Pointer(&items[0]))
	}

	ret, _, _ := procSetAppContainerConfig.Call(
		uintptr(len(items)),
		ptr,
	)
	if ret != 0 {
		return fmt.Errorf("保存 UWP 豁免配置失败: %v", ret)
	}
	return nil
}

// ExemptAllUWP 兼容旧接口：一键豁免所有应用
func ExemptAllUWP() error {
	apps, err := GetUwpAppList()
	if err != nil {
		return err
	}
	
	var sids []string
	for _, app := range apps {
		sids = append(sids, app.SID)
	}
	
	return SaveUwpExemptions(sids)
}
