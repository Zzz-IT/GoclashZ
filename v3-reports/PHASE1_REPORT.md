# GoclashZv3 — Wails v3 Phase 1 Verification Report

> 范围：`D:\GOLANG\GoclashZv3` 隔离对照工程（v3.0.0-beta.28）
> 计划来源：`D:\GOLANG\GoclashZ\WAILS_V3_MIGRATION_PLAN.md` 第 5 章 阶段 1
> 报告生成：阶段 1.7 终稿（基于实测 + 源码核验）

## 0. 环境指纹

| 项 | 值 |
|---|---|
| OS | Windows 10 IoT Enterprise LTSC 2024 / Build 26300 |
| Go | go1.27.0 windows/amd64，CGO_ENABLED=1 |
| Wails v3 CLI | `wails3` v3.0.0-beta.28（`C:\Users\Jiguang\go\bin\wails3.exe`） |
| WebView2 | 154.0.4258.62，runtime 加载 `go-webview2 v1.0.22`（`Go-WebView2Loader=true`） |
| Node | 11.17.0（`minimum-release-age` 警告无影响） |
| 工程 module | `goclashzv3` / `go 1.24` / `wails/v3 v3.0.0-beta.28` |
| 对照工程 | `D:\GOLANG\GoclashZv3\`（含 `bin\goclashzv3.exe` 11.1 MB、`bin\debug-console.exe` 18.6 MB、`bin\debug-console-prod.exe` 15.8 MB、`frontend\dist\` 已构建） |

> **关键事实**：v3 beta.28 仍依赖 WebView2；`webviewloader.UsingGoWebview2Loader=true` 来自
> `pkg/application/application_windows.go:454`，日志里 `Go-WebView2Loader=true` 是真值。
> 计划文档 v0 草稿中"消除 WebView2 依赖并实现原生桌面渲染"的断言**与源码不符**，已被本报告驳回。

## 1. 逐项结论

| # | 项 | 结论 | 证据 |
|---|---|---|---|
| 1.1 | `wails3 init -t vue` 建对照工程 | ✅ PASS | `wails3 init -n goclashzv3 -t vue -d . -mod goclashzv3 -skipgomodtidy -nocolour`（CLI 忽略 `-d .`，实际生成到子目录 `goclashzv3\`，随后平铺到根）。最终根目录含 `build\`、`frontend\`、`go.mod`、`main.go`、`Taskfile.yml`、`README.md`、`.gitignore`；原占位 `go.mod` (29 B) 与空目录 `goclashzv3\` 已 `rm --` 回收 |
| 1.2 | `wails3 generate bindings` 输出 + `@wailsio/runtime` import | ✅ PASS | 1 Service / 15 Methods / 2 Events；`frontend\bindings\goclashzv3\{index.ts, checkservice.ts}`；`node_modules\@wailsio\runtime` = `name @wailsio/runtime`、`version 3.0.0-beta.28`、`type module`；`dist/plugins/vite.js` 存在。`package.json` `"@wailsio/runtime": "latest"` → 实装 3.0.0-beta.28。坑：bindings **namespace 导出**必须 `import { CheckService } from '../../bindings/goclashzv3'`；`-nocolour` 标志对 `generate bindings` 不存在（`flag provided but not defined: -nocolour`）；默认 `-ts=false`，必须显式 `-ts` |
| 1.3 | Go 端覆盖 Window/Event/Dialog/Quit 四类 API | ✅ PASS | `checkservice.go` 实现 15 个绑定方法 + `ServiceStartup` + `interceptClose` + `trace`；`main.go` 用 `application.AssetFileServerFS(assets)`、包裹式 `http.HandlerFunc` Asset Handler、`RegisterHook(events.Common.WindowClosing)`、每秒 `app.Event.Emit("check:time", ...)`、窗口 `Hidden:false` + `Frameless:true` + `BackgroundColour:NewRGBA(17,17,17,255)`；`go vet ./...` = 0；前端 `npm run build` 成功（vue-tsc + vite build v8.3.4，dist 107.6 KB JS + 1.36 KB CSS） |
| 1.4 | 前端 `Events.On(e.data)` 双向打通 | ⚠️ PARTIAL（代码就绪、运行时尚未触发） | 前端 `CheckPanel.vue` 实际写入了 `Events.On('check:time', e => timeValue.value = e.data)` + `Events.On('check:echo', e => log(...))` + `CheckService.AutoRun()` 触发 `runChecks()`（11 项断言 + `WriteReport`）；`@wailsio/runtime` 实际安装为 3.0.0-beta.28；`@wailsio/runtime` 的 `Events` API 暴露 `On/Once/Off/OffAll/Emit/OnMultiple`。**运行端未触发**：`bin\debug-console.exe` 与 `bin\debug-console-prod.exe`（`-tags=production -ldflags="-H windowsgui"`）启动 50 秒后，`PHASE1_TRACE` 仅含 `main() entered` + `attempting WakeAndReload at +500ms`，**无任何 `asset request:`、无 `AutoRun()` 回调、无 `WriteReport`、无 `phase1-report.json`**。详见 §3 阻塞分析 |
| 1.5 | `wails3 dev` 启动 Vite + `FRONTEND_DEVSERVER_URL` 注入 | ✅ PASS | `wails3 dev -config ./build/config.yml -port 9245 -nocolour` 后台运行；`127.0.0.1:9245` `State Listen`（node PID 28136）；HTTP 200 返回本项目 `index.html`（含 `/@vite/client`、`<title>GoclashZv3 · Wails v3 Phase 1 Check</title>`）；dev 进程 `bin\goclashzv3.exe` 存活；进程已用 `task_stop` 结束并确认无残留 |
| 1.6 | `wails3 build` 产出 exe 并可启动 | ✅ PASS | `wails3 build` BUILD_EXIT=0，产物 `bin\goclashzv3.exe` 11,120,640 字节（2026/10/13 0:34:24）。同时手工 `go build -o bin/debug-console.exe .` 18,560,000 字节（带 console，0:36:57）与 `go build -tags=production -ldflags="-H windowsgui" -o bin\debug-console-prod.exe .` 15,819,264 字节（0:03:45）均成功 |
| 1.7 | 阶段 1 验证报告 | ✅ PASS（本文件） | 见 §2/§3/§4 |

## 2. 已通过的源码级验证

- **绑定结构**（1.2）：`frontend\bindings\goclashzv3\checkservice.ts` 暴露 `CheckService` 命名空间，含 `Ping/EmitEvent/LastEvent/WindowShow/WindowHide/WindowUnmaximise/WindowToggleMaximise/WindowIsMaximised/WindowIsVisible/SetWindowBackground/OpenFileDialog/SaveFileDialog/Quit/CloseCount/AutoRun/WriteReport/String`；`index.ts` 同步导出。`@wailsio/runtime@3.0.0-beta.28` package.json 实装；`dist/plugins/vite.js` 存在 → `vite.config.ts` 的 `wails("./bindings")` 插件能取到 dist 入口
- **资产处理链**（1.3）：`application.AssetFileServerFS(assets)` → `assetserver.NewAssetFileServer(assets)`（`internal\assetserver\build_dev.go:67`），无 `FRONTEND_DEVSERVER_URL` 时走 `newAssetFileServerFS(vfs)`；`internal\assetserver\options.go` `Options{Handler, Middleware, Logger}` 校验通过；`application.go:111-158` 中间件链顺序：自定义 `Assets.Middleware` → `eventPayloadPath` → `streamPath` → `/wails/runtime.js` → `/wails/transport.js` → `/wails/custom.js` → `next`
- **窗口选项**（1.3）：`webview_window_options.go:177-178` `Hidden bool`；`webview_window_windows.go:540-542` 在 `Hidden` 时移除 `WS_VISIBLE`（解决 issue #4611 白闪）；`BackgroundColour` 在 `:2696-2705` 与 Chromium `SetBackgroundColourWithError` 同步
- **窗口关闭拦截**（1.3）：`events.Common.WindowClosing` 通过 `OnWindowEvent` 注入 + `e.Cancel()` + `Hide()`（v2 `HideWindowOnClose: true` 的替代方案）；`RegisterHook` 同样路径
- **事件注册**（1.3）：`application.RegisterEvent[string]("check:time")` / `RegisterEvent[string]("check:echo")` 生成强类型绑定；`application.go` `App.Run` 中 `applicationEvents/windowEvents/webviewRequests/windowMessageBuffer/windowKeyEvents/windowDragAndDropBuffer/menuItemClicked` reader goroutine 在 `service.ServiceStartup` 之后启动
- **CLI 工具链**（1.5/1.6）：`wails3 doctor` SUCCESS；`wails3 dev` `-config -port -s -nocolour -help` 全可用；`wails3 build` 同步产出 exe

## 3. 阻塞分析：WebView2 控制器在「无主窗口焦点」下未触发首帧

### 3.1 观察事实

- 启动流程：ServiceStartup → 17 个 binding 注册成功 → WebView2 Environment 创建成功
  → **没有 `Asset Request:` 日志、没有 `asset request:` trace、没有 `processRequest`、没有前端 `index.html` 加载**
- 50 秒后 `debug-console.exe`（PID 15800/18788/27484）与 `debug-console-prod.exe`（PID 5028）仍存活
- 推迟 500 ms 的 `w.Show()` + `w.SetURL("/")` 唤醒尝试**没有让 trace 出现**任何后续
- 把窗口从 `Hidden:true` 改为 `Hidden:false` 没有改变结果
- 把 build 从 `!production` 切到 `-tags=production -ldflags="-H windowsgui"` 也没有改变结果

### 3.2 源码核验（`internal/webview2/pkg/edge/devtools.go` + `pkg/application/request_cancellation_windows.go` + `pkg/application/webview_window_windows.go`）

```
webview_window_windows.go:2718-2720  startRequestCancellation(w.navigateInitialPage)
request_cancellation_windows.go:40-64 ready() 仅在 done(err) 之后调用
request_cancellation_windows.go:76-98  enableSession 依次串行下发 7 条 CDP 命令
request_cancellation_windows.go:100-112 configure 递归串行下发，错误即 done(err)
request_cancellation_windows.go:135-157 call 通过 chromium.CallDevToolsForSession 异步执行
navigateInitialPage 在 ready 之后才会调用 setURL -> chromium.Navigate
```

- `chromium.CallDevToolsForSession` 走 WebView2 `ICoreWebView2_11.CallForSession`（GUID `{0be78e56-c193-4051-b943-23b460c08bdb}`），`internal/webview2/pkg/edge/devtools.go:67-106`
- `done` 闭包内 `if !w.parent.isDestroyed() { ready() }`，逻辑路径清楚
- **关键观察**：在测试中 `globalApplication.error("Windows request cancellation unavailable: %v", err)` 也**没有出现**。这意味着 `done(err)` 路径（错误日志）未被触发，但同时 `done(nil)`（成功路径）也未被触发——即**任一 CDP 命令的回调都没在 50 秒内完成**。这与 Wails GitHub issue #4611（"Window doesn't navigate on first show"）和 issue #4961（"beta.28 request cancellation silently never finishes"）的描述一致

### 3.3 结论

- 阶段 1.4 的**「EventsOn 双向打通」在 runtime 层面已就绪**（Go 端 `app.Event.Emit("check:time", ...)` 与前端 `Events.On("check:time", e => e.data)` 的代码契约、类型声明、绑定产物均已落地）
- 但**「EventsOn 真实在浏览器内核里跑起来」**这条端到端验证在本机环境下被 WebView2 控制器的首帧导航问题打断。这并非 GoclashZv3 工程代码问题——把窗口换 `Hidden:false`、换 `production` 标签、加 `SetURL` 唤醒、增 `Logger:Debug` 都**无法绕过**
- 计划文档阶段 1 的 1.4 准出条件**严格说未达到**。建议在阶段 1 准出前用以下任一方式补测：
  1. 改用 `wails3 dev`（已通过 1.5）——`wails3 dev` 启动 Go exe + Vite 9245，**Go exe 在 dev 模式下**走 `assetserver.NewAssetFileServer(assets)` 的 reverse-proxy 分支，反向代理到 Vite，无需 `processRequest` 走 CDP 取消链路；本机 dev 模式已确认 Vite 200，**这是当前唯一可信的运行时验证路径**
  2. 等 v3 stable 或 beta.29+，等 WebView2 `navigateInitialPage` 修复合入
  3. 切换到 `windowsgui` + 在 Go 代码里手动 `chromium.Navigate("http://wails.localhost/")`（无法直接拿到未导出的 `chromium` 字段，需打 patch）

## 4. 计划文档需要回填的修正

- 计划 v0 中"消除 WebView2 依赖并实现原生桌面渲染"**应改为**："v3.0.0-beta.28 仍强制依赖 WebView2（runtime `154.0.4258.62`，`Go-WebView2Loader=true`），`docs.microsoft.com` 路径与 v2 一致；v3 内部用 `go-webview2` 替代 `jchv/go-winloader`，但运行时资产未消除"
- 阶段 1 准出标准中"1.4 事件双向打通"建议增加"以 `wails3 dev` 模式作为运行时端到端验证通道"或"在阶段 1 准出前可接受 `1.4 PARTIAL`，运行时端到端验证后移到阶段 5/6"
- 阶段 2.1 `go.mod` 中 `jchv/go-winloader // indirect` 移除必须保留 `github.com/wailsapp/go-webview2`（**v3 仍间接使用**，由 `wails/v3/internal/webview2/pkg/edge` 引入）

## 5. 下一阶段推荐动作

- **用户决策点 1**：`wails3 dev` 模式下补做 1.4 端到端验证（推荐） vs 接受 1.4 PARTIAL、运行时端到端验证移后
- **用户决策点 2**：是否进入阶段 2（修改 `D:\GOLANG\GoclashZ\go.mod`、删除 `wails.json`、新增 `build/config.yml` + `Taskfile.yml`）——阶段 2 需在阶段 1 准出后启动
- 留给阶段 5/6 的运行时验证项：`WindowClosing` 真实点击拦截 + `CloseCount` 自增、`Dialog` 原生弹窗真实弹出 + 选择回填、`Quit()` 实际退出、`SetBackgroundColour` 视觉切换、`--wails-draggable` 真实拖拽、最大化/恢复状态翻转——这些都需 `wails3 dev` 配合浏览器内核

## 附录 A：实测命令与产物

```powershell
# 1.1 初始化（CLI 忽略 -d .，生成到子目录后平铺）
wails3 init -n goclashzv3 -t vue -d . -mod goclashzv3 -skipgomodtidy -nocolour
mv goclashzv3\* .\; rm -- .\go.mod; rm -- -r goclashzv3

# 1.2 绑定生成
cd frontend; npm install --no-audit --no-fund
cd ..; wails3 generate bindings -d frontend/bindings -ts

# 1.3 Go 端 + 前端
go vet ./...                                       # 0
cd frontend; npm run build                          # vite v8.3.4, dist 107.6KB JS

# 1.5 dev
wails3 dev -config ./build/config.yml -port 9245 -nocolour
curl -I http://127.0.0.1:9245/                      # HTTP 200

# 1.6 build
wails3 build                                        # bin\goclashzv3.exe 11.1 MB
go build -o bin\debug-console.exe .                 # 18.6 MB
go build -tags=production -ldflags="-H windowsgui" -o bin\debug-console-prod.exe .  # 15.8 MB

# 1.4 runtime probe (PARTIAL)
$env:PHASE1_TRACE = "D:\GOLANG\GoclashZv3\phase1-trace*.log"
$env:PHASE1_AUTORUN = "1"
$env:PHASE1_REPORT = "D:\GOLANG\GoclashZv3\phase1-report.json"
Start-Process -FilePath ".\bin\debug-console-prod.exe" -RedirectStandardError ".\dbg9-err.txt" -RedirectStandardOutput ".\dbg9-out.txt" -PassThru
# 50s 后 phase1-trace9.log 仅 main() entered + WakeAndReload；
# phase1-report.json 未生成
```

## 附录 B：未结清项清单

- 1.4 运行时端到端验证（GUI 内）→ 阻塞，等阶段 1 用户决策
- `WindowClosing` 真实拦截次数递增、`Dialog` 真实选择回填、`Quit` 实际退出、`SetBackgroundColour` 视觉切换、`--wails-draggable` 真实拖拽、`ToggleMaximise/UnMaximise` 状态翻转 → 需 dev 模式 GUI 交互
- v3 `Window.IsVisible` 仍**未在 `@wailsio/runtime` JS 端暴露**（`frontend\src\components\CheckPanel.vue` 第 130 行的 "ABSENT in v3" 已记录），迁移到 GoclashZ 时需保留 Go 端 `WindowIsVisible` 服务方法作为可见性真相
- v3 `WindowSetDarkTheme/SetLightTheme` 仍**无运行时 API**（`Application.vue:120-127` 记录），主题运行时替代方案 T1（重启/跟随系统）成为阶段 4 唯一可落地方案
