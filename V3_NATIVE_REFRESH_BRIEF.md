# GoclashZ v3 原生能力升级（第二轮）执行简报

> 目标目录：`D:\GOLANG\GoclashZv3`。**禁止改动 `D:\GOLANG\GoclashZ`。**
> 前提：第一轮 v2→v3.1.0 迁移已完成并验收（见 `docs/v3-migration/VERIFICATION.md`）。
> 本轮目标：采纳第二轮建议中的 1/2/3/4/5/6/7/8/10；**明确不做 9（主题改为重启生效）与 11（Menu/KeyBindings/FileAssociations/Notifications/Clipboard 等新增能力）**。
> 硬约束：**主题必须保持实时切换**（w32.SetTheme + SetBackgroundColour 现有实现不变）；**不得改变任何现有用户可见行为**（启动矩阵、托盘、热键、单实例、主题、关闭隐藏、退出、事件）。

## 一、本轮要做的项

| # | 内容 | 说明 |
|---|---|---|
| 1 | 强类型事件 | 用 `application.RegisterEvent[T]` 注册现有事件（38 个静态名；动态名前缀跳过），生成 `eventdata.d.ts`。仅类型，不改事件行为 |
| 2 | 窗口引用 + `IsVisible()` | App 持有 `*application.WebviewWindow`，替代 `GetByName` 与内部 visible 状态；Toggle 用 `w.IsVisible()` |
| 3 | `ServiceShutdown` + `ShouldQuit` | shutdown 归 service；`ShouldQuit` 置为返回 true（Windows 当前不消费，保持行为；注释说明） |
| 4 | `WindowRuntimeReady` | 窗口运行时就绪事件里按 isSilent 再隐藏；删除 `SafeHideMainWindowWin32` / `EnsureMainWindowHiddenSilently` 兜底（保留 `Hidden: isSilent` 防白闪） |
| 5 | 显式 `WebviewUserDataPath` | 设置为与 v2 相同的 `%AppData%\GoclashZ.exe`（v2/v3 默认本就一致，显式化避免未来漂移） |
| 6 | `Options.Icon` | 设置 `Icon: iconData`。注意 v3.1.0 Windows 实际用 exe 资源（syso），此字段在 Windows 基本无效，仅 API 完整性 |
| 7 | `SingleInstanceOptions` | 用 v3 原生单实例；保留 `--silent/--startup` 不唤醒、非静默唤醒+闪烁、`--shutdown-existing`、debug 跳过 |
| 8 | 拆分 Service | 3 个转发服务（App/Core/Update）+ 前端聚合 facade；**108 个绑定方法名与调用点保持不变** |
| 10 | v3 `SystemTray` + `GlobalShortcut` | 替换 749+63 行自研 Win32；菜单/点击/多语言/勾选/工具提示与现状 1:1；热键仍为 Ctrl+Alt+Q 退出 |

## 二、不可改变的行为清单（验收基线）

1. 启动：普通启动显示窗口；`--silent` / behavior.SilentStart=true 隐藏；`--startup` 仅在 SilentStart=true 时隐藏（v2 口径）。
2. 关闭按钮：隐藏到托盘，进程存活（`WindowClosing` cancel+hide）。
3. 托盘：右键菜单；双击显示主窗口；左键无动作；菜单项 = 显示 / 系统代理(或端口代理，勾选) / TUN(勾选) / 出站模式子菜单(rule/global/direct 勾选) / 重启内核 / 退出；随语言与状态更新；图标/提示与现状一致。
4. 热键：Ctrl+Alt+Q 退出；被占用时静默失败（仅日志）。
5. 单实例：第二个实例非静默 → 唤醒并闪烁第一实例；`--silent/--startup` → 静默退出；退出码 0；debug 模式不启用。
6. 主题：设置页/侧栏切换后**立即**生效（w32.SetTheme + SetBackgroundColour），不重启。
7. 退出：托盘/热键退出走原 shutdown 顺序（tray stop → helper shutdown → core stop，5s 超时）。
8. 事件：51 处 `Events.On` 的 payload 语义不变。
9. 构建：`go build .`、`go vet ./...`、`wails3 generate bindings -ts -d frontend/bindings -i`、`npm run build`、`wails3 build` 全部通过。

## 三、技术要点（必须读源码确认，不要猜）

- v3.1.0 源码：`C:\Users\Jiguang\go\pkg\mod\github.com\wailsapp\wails\v3@v3.1.0\pkg\application\`
- `SingleInstance`：`application.go:207` 在 `application.New` 内处理；第二实例 `os.Exit(ExitCode)`；第一实例 `OnSecondInstanceLaunch(SecondInstanceData{Args,WorkingDir,AdditionalData})`。
- `ShouldQuit`：仅 macOS `shouldQuitApplication` 使用（`application_darwin.go:526`），Windows 无消费；实现为返回 true。
- `IsVisible`：Windows 用 `WS_VISIBLE`（`webview_window_windows.go`），最小化仍为 true，与现有状态语义一致。
- `WindowRuntimeReady`：runtime 加载完成后 emit（`webview_window.go:882`）。
- `SystemTray`：`app.SystemTray.New()`（`system_tray_manager.go:16`）；`SetIcon/SetMenu/SetTooltip/OnClick/OnRightClick/OnDoubleClick/ShowMenu/Destroy`；默认右键=ShowMenu、默认单击=ToggleWindow（需显式覆盖以保持“左键无动作”）。
- `GlobalShortcut`：`app.GlobalShortcut.Register("Ctrl+Alt+Q", cb)`（`global_shortcut_manager.go:100`）；返回 error；必须在 app 运行后（ServiceStartup 内）注册。
- 托盘图标：`w32.CreateSmallHIconFromImage` 支持 PNG/ICO，现有 `iconData`（ICO）可用。
- `ServiceOptions.Name` 只影响日志/调试名；bindings 命名空间以类型名为准。转发服务类型建议 `AppService`/`CoreService`/`UpdateService`。
- `ServiceShutdown` 接口：`services.go:118`，签名为 `ServiceShutdown() error`。
- `WindowsOptions.WebviewUserDataPath` 是 **应用级** `Options.Windows` 字段（`application_options.go:298`），不是窗口级 `WindowsWindow`。

## 四、协作与文件所有权

- `lifecycle-v3`：`main.go`、`app.go`（窗口/生命周期/单实例/主题 helper）。不得改 tray/hotkey 文件。
- `tray-v3`：`tray_runtime_windows.go`、`hotkey_windows.go`（可新增 `tray_v3.go`/`hotkey_v3.go` 并删除旧实现）。**保持** `NewTrayRuntime(app *App) *TrayRuntime`、`(*TrayRuntime).Start(ctx)`、`Stop()`、`UpdateState(appcore.AppState)` 签名不变；不改 `app.go`/`main.go`，缺接口先找 Lead。
- `events-v3`：新增 `v3_events.go`（可加 `v3_event_types.go`），不改 `main.go`/`app.go`/tray 文件。
- `services-v3`（后续）：新增转发服务文件 + `main.go` 仅改 Services 列表；依赖 lifecycle 完成。
- `frontend-v3`（后续）：`frontend/src/api.ts` 聚合 + 21 个 import 路径；不改业务逻辑。
- `verifier`（后续）：只读验收 + 报告。

## 五、每人必须自测并汇报

- 命令 + 退出码 + 关键输出；
- 行为矩阵中与自身相关的项（A：启动/单实例/退出/主题；B：托盘/热键；C：事件类型/前端构建；D/E：全量）；
- 不得声称“应该没问题”，必须实际运行；无法自动验证的项明确标注“需人工”。

## 六、已知风险

- v3 `SystemTray` 的默认单击会 ToggleWindow：必须显式 `OnClick(func(){})` 保持左键无动作。
- `SetMenu` 动态更新：菜单项回调在 framework goroutine，需避免阻塞；状态更新频繁时重建 menu 可能闪烁/句柄增长，需节流或复用 MenuItem。
- 单实例切换后 v2/v3 锁名不同：过渡期两版本可同时运行（文档记录，不作为回归）。
- `WebviewUserDataPath` 与 v2 相同路径：v2/v3 同时运行会争用 WebView2 用户数据目录（过渡期风险，文档记录）。
- `WindowRuntimeReady` 隐藏窗口若时序不当可能闪一下：必须实测 `--silent` 启动。
