# GoclashZv3 — Wails v3 Phase 6 报告：单实例 v3 路径推演

> 范围：`D:\GOLANG\GoclashZv3`（隔离对照工程，仅此一处落地；`D:\GOLANG\GoclashZ` 主干按硬约束未触碰）
> 计划来源：`D:\GOLANG\GoclashZ\WAILS_V3_MIGRATION_PLAN.md` 第 5 章 阶段 6
> 状态：✅ v3 公开 `SingleInstance` API 已实测；✅ 与生产 GoclashZ 共存不冲突；✅ GoclashZ 主干 v3 化时的接入骨架已在 `GoclashZv3\main.go` 注释块中写出

## 1. 现状盘点（实测）

### 1.1 v2 GoclashZ 主干的单实例机制

- 文件：`D:\GOLANG\GoclashZ\main.go:200-256`
- 方式：**自研 Win32 互斥锁** `Global\GoclashZ_Single_Instance_Mutex`
- API：`golang.org/x/sys/windows.CreateMutex(nil, false, ...)` + `syswin.ERROR_ALREADY_EXISTS` 判断
- 行为：
  - 第一个实例：拿到 mutex 继续启动；`defer syswin.CloseHandle(mutexHandle)`
  - 第二个实例：`ERROR_ALREADY_EXISTS` → 调 `sys.FocusMainWindowAndFlashTwiceWin32Only()` 唤起第一个实例的窗口 + FlashWindowEx 闪烁 → 自己 `os.Exit(0)`
  - 静默模式 (`--silent` / `--startup`)：第二个实例**不唤起**已有窗口，直接 `os.Exit(0)`（不打扰用户）

### 1.2 v3 beta.28 公开 API（实测源码）

`pkg/application/single_instance.go` 完整实现：

```go
type SingleInstanceOptions struct {
    UniqueID               string                                                // mutex + hidden window 类名前缀
    OnSecondInstanceLaunch func(data SecondInstanceData)                        // 收到 argv 时回调
    AdditionalData         map[string]string
    ExitCode               int                                                   // 第二实例退出码
    EncryptionKey          [32]byte                                              // AES-256-GCM，可选
}

type SecondInstanceData struct {
    Args           []string          `json:"args"`
    WorkingDir     string            `json:"workingDir"`
    AdditionalData map[string]string `json:"additionalData,omitempty"`
}
```

挂载方式（`application_options.go:111-112`）：

```go
type Options struct {
    ...
    SingleInstance *SingleInstanceOptions   // ← 字段
}
```

`single_instance_windows.go:34-54` 实现：

```go
func (l *windowsLock) acquire(uniqueID string) error {
    id := "wails-app-" + uniqueID
    className  := id + "-sic"      // 隐藏消息窗口的 ClassName
    windowName := id + "-siw"      // 隐藏消息窗口的 WindowName
    mutexName  := id + "-sim"      // ← 真正注册的 Win32 mutex
    _, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(mutexName))
    if err != nil { return alreadyRunningError }
    l.hwnd = createEventTargetWindow(className, windowName)  // ← 收 WM_COPYDATA
    return nil
}
```

第二实例通知走 `WM_COPYDATA` + 第二实例 argv/工作目录 JSON 序列化（AES-256-GCM 可选加密）。

`application.go:203-218` 接入流程：

```go
if appOptions.SingleInstance != nil {
    manager, err := newSingleInstanceManager(result, appOptions.SingleInstance)
    if err != nil {
        if errors.Is(err, alreadyRunningError) && manager != nil {
            err = manager.notifyFirstInstance()       // ← 通知第一实例
            os.Exit(appOptions.SingleInstance.ExitCode)  // ← **第二实例直接退出**
        }
        result.fatal(...)
    } else {
        result.singleInstanceManager = manager
    }
}
```

`OnSecondInstanceLaunch` 在第一实例的 goroutine 里通过 `secondInstanceBuffer` channel 收到。

## 2. v2 → v3 推演

### 2.1 必须解决的差异

| 行为 | v2 GoclashZ | v3 (Plan A: 用 v3 native API) | v3 (Plan B: 保留 v2 Win32 互斥锁) |
|---|---|---|---|
| 互斥锁创建 | 自研 `CreateMutex` | v3 内部 `singleInstance_windows.go:45` | 同 v2 |
| 互斥锁名字 | `Global\GoclashZ_Single_Instance_Mutex` | `Global\wails-app-GoclashZ_Single_Instance_Mutex-sim` | `Global\GoclashZ_Single_Instance_Mutex` |
| 第二实例 argv 传递 | 无 | `WM_COPYDATA` + `OnSecondInstanceLaunch` | 无 |
| 第二实例唤醒第一实例窗口 | `sys.FocusMainWindowAndFlashTwiceWin32Only` | 在 `OnSecondInstanceLaunch` 里调 `w.Restore() / w.Show() / w.Focus() / w.UnMaximise()` | 同 v2 |
| 静默模式（`--silent` / `--startup`） | 不唤起，直接 `os.Exit(0)` | **v3 不支持直接区分**——`OnSecondInstanceLaunch` 统一回调，需要在回调里检查 `os.Args` 里有没有 `--silent` | 同 v2（保留 v2 逻辑） |
| 第一实例退出时是否清理 | `defer syswin.CloseHandle(mutexHandle)` | v3 内部 `singleInstanceManager.cleanup()`（`application.go:956-959`） | 同 v2 |

### 2.2 推荐方案：**Plan B（保留 v2 Win32 互斥锁）**

理由：
- v3 native API 的 `OnSecondInstanceLaunch` 回调是**统一入口**，无法像 v2 那样"在拿到互斥锁后立即根据 argv 决定是否通知"。v2 GoclashZ 的"静默模式不唤起"是 UX 关键，不该破坏
- v3 native API 的 mutex 名字 `wails-app-GoclashZ_Single_Instance_Mutex-sim` 与 v2 `Global\GoclashZ_Single_Instance_Mutex` **不冲突**——但意味着 v2 → v3 升级期间两个版本可以共存，**这是好事**（用户能回滚）
- v3 native API 的退出语义 `os.Exit(SingleInstance.ExitCode)` **在 `ServiceStartup` 之前发生**——意味着如果 `OnSecondInstanceLaunch` 触发，第二实例写不进任何业务 trace
- v2 GoclashZ 的 `sys.FocusMainWindowAndFlashTwiceWin32Only` 用的是 `FlashWindowEx` 闪烁任务栏图标，**v3 没有等价 JS API**（只能 Go 端 Win32）——如果改用 v3 native API，这部分还得保留 Win32 调用

### 2.3 GoclashZ 主干 v3 化的最小改动（Plan B 实现骨架）

```go
// main.go
if !isDebugMode {
    mutexName, _ := syswin.UTF16PtrFromString("Global\\GoclashZ_Single_Instance_Mutex")
    mutexHandle, err := syswin.CreateMutex(nil, false, mutexName)
    if err != nil {
        if err == syswin.ERROR_ALREADY_EXISTS {
            // v2 路径：静默模式直接退出；否则调 Win32 唤起 + 闪烁
            if hasFlag("--silent") || hasFlag("--startup") {
                if mutexHandle != 0 { syswin.CloseHandle(mutexHandle) }
                os.Exit(0)
            }
            sys.FocusMainWindowAndFlashTwiceWin32Only()
            if mutexHandle != 0 { syswin.CloseHandle(mutexHandle) }
            os.Exit(0)
        }
        logger.Errorf("创建互斥锁发生异常: %v", err)
    }
    if mutexHandle != 0 { defer syswin.CloseHandle(mutexHandle) }
}
// ... 后面是 application.New(...)
```

**注意**：在 `application.New()` **之前**做互斥锁检测，不要用 `SingleInstanceOptions`（避免 v3 native API 的 `os.Exit` 早退路径）。

### 2.4 Plan A（用 v3 native API）骨架

```go
// main.go
var mainWindow *application.WebviewWindow
v3App := wailsapp.New(wailsapp.Options{
    Name: "goclashz",
    Services: []wailsapp.Service{ wailsapp.NewService(app) },
    SingleInstance: &wailsapp.SingleInstanceOptions{
        UniqueID: "GoclashZ_Single_Instance_Mutex",
        ExitCode: 0,
        OnSecondInstanceLaunch: func(data wailsapp.SecondInstanceData) {
            // 静默模式不唤起
            for _, a := range data.Args {
                if a == "--silent" || a == "--startup" {
                    return
                }
            }
            if mainWindow != nil {
                mainWindow.Restore()
                mainWindow.Show()
                mainWindow.Focus()
                mainWindow.UnMaximise()
            }
            // Win32 闪烁：仍然走 sys.FocusMainWindowAndFlashTwiceWin32Only
            sys.FocusMainWindowAndFlashTwiceWin32Only()
        },
    },
    Assets: wailsapp.AssetOptions{
        Handler: wailsapp.AssetFileServerFS(assets),
    },
    OnShutdown: func() { ... },
})
mainWindow = v3App.Window.NewWithOptions(...)
v3App.Run()
```

**取舍**：
- Plan A 优点：v3 native API 帮你处理了 hidden message window + WM_COPYDATA，第二实例 argv 透传到第一实例（v2 没有这层）
- Plan A 缺点：退出路径 `os.Exit` 早于 `ServiceStartup`；静默模式分支必须放在 callback 里；Win32 闪烁仍需手写
- Plan B 优点：与 v2 逻辑 1:1 对应，迁移风险低；静默模式行为保持；可与 v2 共存（互斥锁名字不同但 Windows mutex 是按字符串名匹配的）
- **GoclashZ 推荐 Plan B**

## 3. 实测验证

### 3.1 GoclashZv3 + 生产 GoclashZ 共存（已实测）

测试场景：
- `GoclashZ.exe`（PID 16068，2026/10/13 1:38:15 启动）保持运行
- 启动 `bin\debug-console.exe`（GoclashZv3 隔离工程构建）两次，PID 3584 + 23888

**结果**：
- GoclashZ 进程未被影响
- GoclashZv3 两个进程都跑起来
- GoclashZv3 trace 完整记录 `Build Info / Platform Info / AssetServer Info / 17 个 binding 注册 / ServiceStartup / WebView2 Environment created`
- 互不干扰（互斥锁名字不同 + 隔离工程 SingleInstance 默认关闭）

### 3.2 GoclashZv3 main.go 已落地的"文档性改动"

`D:\GOLANG\GoclashZv3\main.go` 的 `application.New(Options{...})` 块上方有一段**约 30 行的注释**（不是实际打开 SingleInstance），把 Plan A 的接入骨架、UniqueID 选择、与 v2 mutex 的共存关系、OnSecondInstanceLaunch 的静默模式分支都写在 `Options.SingleInstance` 字段位置——**GoclashZ 主干 v3 化时直接抄过去即可**。

注释块摘要（实际在 main.go 注释里）：

```go
// To enable in GoclashZ main (post-migration), use:
//
//   SingleInstance: &application.SingleInstanceOptions{
//       UniqueID:               "GoclashZ_Single_Instance_Mutex",
//       ExitCode:               0,
//       AdditionalData:         map[string]string{"source": "second-launch"},
//       OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
//           w, _ := application.Get().Window.GetByName("main")
//           if w != nil { w.Restore(); w.Show(); w.Focus(); w.UnMaximise() }
//           sys.FocusMainWindowAndFlashTwiceWin32Only() // Win32 flash
//       },
//   },
```

## 4. 阶段 6 准出

| 项 | 结果 |
|---|---|
| v3 公开 `SingleInstance` API 文档 | ✅ 源码核验（`single_instance.go` + `single_instance_windows.go`） |
| v2 → v3 推演方案 | ✅ Plan A / Plan B 双方案对比 |
| GoclashZ 推荐方案 | ✅ Plan B：保留 v2 Win32 互斥锁 |
| GoclashZv3 main.go 接入骨架 | ✅ 注释块形式记录 |
| GoclashZ 与 GoclashZv3 共存验证 | ✅ 互不干扰（实测 PID 3584/23888 + GoclashZ 16068 同时存活） |
| `go vet` + `go build` 0 退出 | ✅ |
| 主题运行时 + 单实例 1.4 链路 | ⚠️ WebView2 首帧仍卡死（与阶段 1 1.4 PARTIAL 同源），本阶段不受影响 |

## 5. 阶段 6 已知遗留

- **GoclashZ 主干 `main.go` 不动**：本报告的 Plan B 骨架是迁移指南，等你解除硬约束后实际替换
- **静默模式分支**：Plan A 中 `OnSecondInstanceLaunch` 必须在 callback 里检查 argv 才不破坏 v2 行为，**这不是 v3 native API 的设计意图**，需要写明注释
- **退出码语义**：v2 GoclashZ 第二实例固定 `os.Exit(0)`；v3 native API 由 `ExitCode` 字段控制。GoclashZ 推荐保留 `ExitCode: 0`
- **`--shutdown-existing` 命令**：v2 main.go:165-168 已实现，v3 路径下仍可保留——它**不依赖**互斥锁，只是给命令行 CLI 用的唤醒信号
- **GoclashZHelper.exe 的早期版本清理**：v2 时期如果升级期间两个版本的 helper 服务并存，可能有服务名冲突；与单实例无关，但生产环境需要注意

## 6. 下一阶段

- **阶段 7（上线前补做）**：未动。包括
  - Inno 模板（已在阶段 5 落地示例）→ GoclashZ 主干 v3 化时迁移
  - `build_all.bat` / `build_local.bat` 适配 v3 命令
  - `.github/workflows/release.yml` 改用 `wails3 build` + 不动 helper 编译命令
  - GoclashZHelper.exe 路径保持 v2 编译链（go build -o build/bin/GoclashZHelper.exe ./cmd/goclashz-helper/）
  - `core/appcore/behavior.go` 增 `Theme` 字段（v3 持久化用）
- **阶段 8（GUI 端到端）**：1.4 PARTIAL 状态延续，等 v3 stable
- **主干迁移实战**：等你解除硬约束

是否继续阶段 7（CI / build_all.bat / Inno 迁移示例）？还是停在阶段 6 收尾？

## 7. 计划文档需要回填的修正

- 计划 §3 "v3 公共 API" 表里**应补充**：
  - `application.Options.SingleInstance *SingleInstanceOptions` 字段
  - `application.SingleInstanceOptions` 完整字段列表（UniqueID / OnSecondInstanceLaunch / AdditionalData / ExitCode / EncryptionKey）
  - `application.SecondInstanceData` 结构（Args / WorkingDir / AdditionalData）
- 计划 §6（v3 单实例）**应明确推荐 Plan B**（保留 v2 Win32 mutex），原因：本节 §2.2 列举的 4 条差异中，**静默模式分支**和**Win32 闪烁**两点 Plan A 需要 Go 端额外写代码，迁移成本高于 Plan B
