# GoclashZ Wails v3 迁移工作简报（团队共用）

> 本目录 = GoclashZ 的 v3 升级分支，完成后取代 main。工作目录 `D:\GOLANG\GoclashZv3`。
> **禁止改动 `D:\GOLANG\GoclashZ`（main 主干）**，它只作只读参考。

## 一、目标与既定事实

- 目标框架：**Wails v3.1.0 stable**（CLI 已安装 `%USERPROFILE%\go\bin\wails3.exe`，版本 v3.1.0；要求 Go 1.27+，本机 1.27）。
- Go module 名已改为 **`goclashz`**（与主干一致，便于最终替换 main）。
- 平台：**仅 Windows**；功能与 GoclashZ v1.4.0 1:1 等价（不新增、不改 UI）。
- 已完成（Lead）：
  - 已把 GoclashZ main 全部 tracked 源码复制进本目录（100 个 Go 文件 / 19.5k 行、26 个 .vue / 9k 行）。
  - 已用 v3.1.0 官方模板覆盖 `build/`（Taskfile/config.yml/各平台 Taskfile）。
  - `go.mod` 同时保留 `wails/v2 v2.12.0`（过渡）与 `wails/v3 v3.1.0`；`go vet ./...` 已通过。
  - 已删除隔离工程的演示代码（CheckService/CheckPanel/Phase 报告）。
  - `frontend/dist` 目前是占位 index.html（前端构建后会被覆盖）。
- **尚未迁移任何 v2 API**。当前 4 个文件 import wails v2：
  `main.go`、`app.go`、`wails_event_sink.go`、`core/appcore/controller.go`。

## 二、架构决策（不要擅自更改）

1. 整个 `*App` 作为**单个 v3 service**：`application.NewService(app)`；`*App` 共 109 个导出方法（含 `ServiceStartup`），其中 **108 个进入 bindings**，与 v2 `App.d.ts` 的 108 个方法同名同集合，前端调用名不变。
2. 事件出口保持单点：`WailsEventSink.Emit` → `application.Get().Event.Emit(name, args...)`。
3. 单实例：**保留 v2 自研 Win32 互斥锁**（`Global\\GoclashZ_Single_Instance_Mutex` + `sys.FocusMainWindowAndFlashTwiceWin32Only`），**不要**用 v3 `SingleInstanceOptions`。
4. 托盘（`tray_runtime_windows.go`）/ 热键（`hotkey_windows.go`）/ `core/sys` 自研 Win32 层：**零改动**。
5. 窗口：`Name: "main"`、`Frameless: true`、`MinWidth:900/MinHeight:600`、`BackgroundColour` 来自 `theme_setting.txt`；
   v2 的 `StartHidden:true` + startup 里 Show/Hide 改为 **`Hidden: isSilent`**（在 main 里先算出 isSilent：`--silent`/`--startup`/behavior.SilentStart），避免在 ServiceStartup 里操作尚未创建的窗口。
6. `HideWindowOnClose:true` → v3 `Window.OnWindowEvent(events.Common.WindowClosing, ...)` 中 `e.Cancel()` + `w.Hide()`。
7. 主题：v3 **没有**运行时 SetTheme API。用 `Window.SetBackgroundColour` + 持久化 `theme_setting.txt`（已有机制）；可选：通过 `w.NativeWindow()` 拿 HWND 调 `pkg/w32.SetTheme(hwnd, isDark)`。
8. bindings：`wails3 generate bindings -ts -d frontend/bindings`（生成到 `frontend/bindings/goclashz/`），前端命名空间导入。
9. 安装器：保留 Inno Setup（`package.iss`），不切 NSIS。
10. 所有产物名：**GoclashZ.exe**（`bin/GoclashZ.exe`），产品名/版本 1.4.0，公司 ZzzDREAM。

## 三、v2 → v3 Go API 映射

| v2 | v3.1.0 |
|---|---|
| `wails.Run(&options.App{...})` | `application.New(application.Options{...})` + `app.Run()` |
| `OnStartup: app.startup` | `App` 实现 `ServiceStartup(ctx, application.ServiceOptions) error` |
| `OnShutdown: app.shutdown` | `Options.OnShutdown: func(){...}` |
| `Bind: []interface{}{app}` | `Services: []application.Service{application.NewService(app)}` |
| `AssetServer: &assetserver.Options{Assets: assets}` | `Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)}` |
| `runtime.WindowShow/Hide/Unmaximise/...` | `application.Get().Window.GetByName("main")` 后调 `w.Show()/Hide()/UnMaximise()/ToggleMaximise()/IsMaximised()` |
| `runtime.Quit(ctx)` | `application.Get().Quit()` |
| `runtime.EventsEmit(ctx, name, args...)` | `application.Get().Event.Emit(name, args...)`（`Emit(name string, data ...any) bool`） |
| `runtime.OpenFileDialog(ctx, opts)` / `SaveFileDialog` | `application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{...}).PromptForSingleSelection()` / `SaveFileWithOptions(...).PromptForSingleSelection()` |
| `runtime.FileFilter{DisplayName,Pattern,Extensions}` | `application.FileFilter{DisplayName,Pattern}`（Pattern 用 `*.yaml;*.yml` 形式） |
| `options.Windows{Theme: SystemDefault, DisableWindowIcon}` | `application.WindowsWindow{Theme: application.SystemDefault, DisableIcon:false}` |
| `HideWindowOnClose:true` | `w.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent){ e.Cancel(); w.Hide() })` |

注意：v3 的 `ServiceStartup` 在窗口实际创建/显示之前执行，**不要在 startup 里依赖 Win32 窗口句柄**；窗口可见性用 `Hidden` 选项控制。
注意：v3 的 `App` 服务方法只导出 `*App` 的导出方法；`ServiceStartup` 会在启动时自动调用。

## 四、前端映射（`@wailsio/runtime@3.1.0`）

| v2（`wailsjs/runtime/runtime`） | v3（`@wailsio/runtime`） |
|---|---|
| `EventsOn(name, cb)` | `Events.On(name, (e) => cb(e.data))`（返回 unsub） |
| `EventsOff(name)` / `EventsOffAll()` | `Events.Off(name)` / `Events.OffAll()` |
| `BrowserOpenURL(url)` | `Browser.OpenURL(url)` |
| `WindowMinimise()` | `Window.Minimise()` |
| `WindowToggleMaximise()` | `Window.ToggleMaximise()` |
| `WindowIsMaximised()` | `Window.IsMaximised()` |
| `WindowSetBackgroundColour(r,g,b,a)` | `Window.SetBackgroundColour(r,g,b,a)` |
| `WindowSetDarkTheme()/WindowSetLightTheme()` | **无**；改为调后端 `App.SaveThemePreference(isDark)` + `Window.SetBackgroundColour`（可选后端补 w32.SetTheme） |
| `Quit()` | `Application.Quit()` |
| `import * as API from '../wailsjs/go/main/App'`；`API.X()` | `import { App } from '../../bindings/goclashz'`；`App.X()`（以生成文件实际导出为准） |

前端最终必须：`npm install` + `npm run build`（vue-tsc --noEmit + vite build）通过；删除 `frontend/wailsjs/`、`package.json.md5`。

## 五、验收基线

1. `go build .` 与 `go vet ./...` 0 退出（`build/ios` 模板包报 `function main is undeclared` 属正常，不修）。
2. `wails3 generate bindings` 成功，`frontend/bindings/goclashz/` 生成。
3. `npm run build` 通过（vue-tsc 0 错）。
4. `wails3 build` 产出 `bin/GoclashZ.exe`。
5. Inno `ISCC.exe package.iss` 可编译（若本机有 ISCC）。
6. 启动 smoke：进程能起来、WebView2 首帧加载（v3.1.0 预期已修复 beta.28 首帧问题）。
7. 不改 `D:\GOLANG\GoclashZ`。

## 六、协作纪律

- 只改自己 task 的 write scope；跨 scope 的改动先发消息给 Lead。
- 改完必须自己跑构建/类型检查，报告**命令 + 退出码 + 关键输出**。
- 不要提交 git；Lead 最后统一处理。
- 遇到 v3 API 不确定，读 `C:\Users\Jiguang\go\pkg\mod\github.com\wailsapp\wails\v3@v3.1.0\` 源码或 `%USERPROFILE%\go\bin\wails3.exe` 的 `--help`。
