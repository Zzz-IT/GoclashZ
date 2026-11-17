# R2 第二轮独立验收报告（原生化，无 UX 回归）

- **验收人**：verifier（独立于 P1/P2/P3/lifecycle/tray/events/services 实施者）
- **对象**：`D:\GOLANG\GoclashZv3`，构建产物 `bin/GoclashZ.exe` 26,782,208 B @ 2026-11-04 **17:58:17（R2-1 修复后重构建；17:44:44 为修复前版本）**
- **复核更新**：2026-11-04 18:00–18:01 对 R2-1（WM_CLOSE）修复做独立复核：**5/5 PASS**（Lead 另测 6/6），本报告已将该项由 FAIL 更新为 **PASS**
- **时间**：2026-11-04 17:47–18:05（Asia/Singapore）
- **隔离**：所有实机测试使用 `GOCLASHZ_DATA_DIR=%TEMP%\r2*_data` 临时数据目录；**未设置** `WAILS_DEV_SERVER`（否则会跳过单实例，无法测单实例）；测试进程全部用 `Kill` 清理，最终 `GoclashZ*` 残留 0
- **未改业务源码**：本次只重新生成了 `.gitignore` 内的 `frontend/bindings`、`frontend/dist`；报告与测试脚本均在 `docs/v3-migration/` 与 `%TEMP%`

## 结论摘要

| # | 检查项 | 结果 | 证据 |
|---|---|---|---|
| 1 | `go build .` / `go vet ./...` | **PASS** | 均 exit 0（2.2s / 0.8s），无输出 |
| 2 | `gofmt -l` | **PASS（已修复）** | **104 个 .go 全部干净，flagged=0**（R1 轮的 CRLF 问题已解决） |
| 3 | `go test`（跳过 core/sys） | **PASS** | 12 个包 exit 0；含测试的 4 个包 10 个用例全 PASS、0 FAIL |
| 4 | bindings 可复现 | **PASS** | 重新生成 18 文件，清单 SHA256 前后一致（`45E97CE3…CDC8`），byte-identical |
| 5 | 三服务方法并集 = 108 | **PASS** | appservice 31 + coreservice 62 + updateservice 15 = **108**；Go 生成 33+62+15=110 − 2 生命周期 = 108；mapping 108 唯一；三方比对**无缺失、无重名、无多余** |
| 6 | 62 强类型事件 | **PASS** | RegisterEvent 62 条、名字 62 唯一；`events/index.ts` 导出 62 个 const；`eventdata.d.ts` 4,089 B |
| 7 | 前端 api facade | **PASS** | `src/api.ts` 三 namespace 展开；**21 个文件** import api；旧聚合导入 **0** |
| 8 | `vue-tsc` / `npm run build` | **PASS** | 均 exit 0（2.6s / 3.2s），155 modules |
| 9 | 产物 VERSIONINFO | **PASS** | FileVersion=1.4.0, ProductVersion=1.4.0, ProductName=GoclashZ, Company=**ZzzDREAM** |
| 10 | 普通启动可见 | **PASS** | hwnd 有效、`IsWindowVisible=True`、标题 GoclashZ、1024×768 |
| 11 | `--silent` 隐藏 | **PASS** | MainWindowHandle=0；EnumWindows 显示窗口存在但 `vis=False` |
| 12 | `--startup`（SilentStart=false）可见 | **PASS** | `IsWindowVisible=True` |
| 13 | 单实例（`--silent` 不唤醒） | **PASS** | 第二实例 **exit 0 / 123 ms**，第一实例仍存活且 `vis=False` |
| 14 | 单实例（普通第二实例唤醒） | **PASS** | 第三实例 **exit 0 / 123 ms**，第一实例 `IsWindowVisible=True` 恢复显示 |
| 15 | Ctrl+Alt+Q 退出 | **PASS** | 4 次实测均退出，exit code 0 |
| 16 | **WM_CLOSE 隐藏且 10s 存活** | **PASS（修复后 5/5 + Lead 6/6）** | 修复后 5 轮：每轮窗口对象仍在、`vis=False`、进程存活（+10s）、无 exit；根因与修复见 §6.1/§6.2 |
| 17 | stderr / 事件类型校验 | **PASS** | 全程无 `event type mismatch` / `binding not found` / tray 错误；无 panic |
| 18 | UI 首屏渲染（本人独立截图） | **未自动验证** | 两次采集失败（窗口被遮挡；PrintWindow 调用异常）→ 以 Lead 的 `R2-app-screenshot.png` 为准 |
| 19 | 主题实时切换（点击） | **未自动验证**（代码路径 PASS） | 见 §8；需人工点击侧栏主题按钮确认 |
| 20 | 托盘菜单交互 | **未自动验证**（代码审查 PASS） | 见 §9；未实机弹出托盘菜单 |

**总体：20 项中 18 PASS、0 FAIL、2 项标注未自动验证。**（WM_CLOSE 修复后复核 5/5，Lead 6/6）

## 1. Go 静态检查与测试

```
go build .        exit=0  2.2s   （无输出）
go vet ./...      exit=0  0.8s   （无输出）
gofmt -l <全仓 104 个 .go>   flagged=0 / 104      ← 上一轮的 96 条 CRLF 已全部消除
go test -count=1 <12 包，排除 core/sys 与 build/*>   exit=0  1.3s
  ok goclashz（含 services_v3_mapping_test.go）、core/appcore、core/backup、core/clash
  其余 8 包 [no test files]
  -v 统计：--- PASS 10，--- FAIL 0
```

`core/sys` 按要求未运行（会改注册表/计划任务）；`build/ios` 等模板包按简报约定不计。

## 2. bindings 可复现 + 三服务并集

```
wails3 generate bindings -ts -d frontend/bindings -i -clean=true
  exit=0
  INFO Processed: 284 Packages, 3 Services, 108 Methods, 4 Enums, 40 Models, 62 Events in 894.75ms
生成前/后清单 SHA256 = 45E97CE3A08943A090F3D4E9A2EF769B6FB9B3224DBDCC4E3AEE7FB691CBCDC8（18 文件，完全一致）
```

| 口径 | 数量 | 说明 |
|---|---|---|
| `appservice.ts` 导出函数 | 31 | App 侧（窗口/设置/诊断/备份/Helper/UWP/管理员） |
| `coreservice.ts` 导出函数 | 62 | 内核/配置/订阅/规则/连接/代理/日志/流量 |
| `updateservice.ts` 导出函数 | 15 | 应用/内核/组件/geo/驱动更新 |
| **并集合计** | **108** | 与第一轮（单 service）108 完全一致 |
| `services_v3_mapping.go` 条目 | 108 | 唯一，无重复键 |
| Go 侧生成方法 | 110 | 108 + `ServiceStartup`/`ServiceShutdown`（AppService，生成器按内部方法处理） |
| mapping ↔ 生成方法 差集 | 仅上述 2 个生命周期方法 | 108 个绑定方法**全部且仅被分配一次** |
| 跨服务重名 | **0** | 检查无同一方法出现在两个服务 |

## 3. 强类型事件（62 个）

- `v3_events.go`：`application.RegisterEvent[T]("name")` **62 条**，事件名 **62 个唯一**（重复注册会让 v3 在 init 阶段 panic，而应用 15+ 次正常启动即证明无重复）。
- 生成物：`frontend/bindings/events/index.ts` 导出 **62 个** `export const XxxEvent = "name"`；`github.com/wailsapp/wails/v3/internal/eventdata.d.ts` 4,089 B（第一轮 113 B）。
- 关键事件类型静态核对（抽样）：`app-state-sync→appcore.AppState`、`traffic-data→appcore.TrafficSnapshot`、`log-message→logger.LogEntry`、`notify-error→string`、`connections-update→ConnectionsSnapshot`、`proxy-state-sync→[]ProxyGroupState`、`proxy-delay-update→map[string]any`、`tun-mips-fallback→application.Void`，与发射点一致。
- 动态事件 `geo-update-<key>-*` 未注册（作者已在注释说明，key 运行时拼接），保持无类型校验的旧行为。

## 4. 前端 facade

```
frontend/src/api.ts:  import * as AppService/CoreService/UpdateService 后 export const App = { ...AppService, ...CoreService, ...UpdateService }
  → 108 个调用点维持 App.Method(...) 写法不变
import api 的文件：21 个（App.vue、20 个 component/store）
旧的 bindings 根聚合导入（import { App } from '../bindings/goclashz'）：0
仍直接 import bindings 的：仅模型/子 namespace —— store.ts → core/runtimeassets，Subscriptions.vue → core/clash（合理，模型类型不经 facade）
vue-tsc --noEmit exit=0（2.6s）
npm run build  exit=0（3.2s；settings-manifest 87 项 OK；vite 155 modules；assets/index-CvW7MwRc.js 989.50 kB）
```

## 5. 产物

```
bin/GoclashZ.exe        26,782,208 B  17:58:17  VI 1.4.0 / 1.4.0 / GoclashZ / ZzzDREAM   ✅（R2-1 修复后重构建；大小不变）
bin/GoclashZHelper.exe   3,464,704 B  16:44:21  无 VI（与 v2 一致，未变）
build/installer/…Setup.exe 28,219,815 B  17:59:45（**Lead 已按 R2-1 修复后的主 exe 重跑 ISCC**，Source=bin/GoclashZ.exe 17:58:17）
```
本报告验收的 `bin/GoclashZ.exe` 为 Lead 在 R2-1 修复后于 17:58:17 重跑 `wails3 build` 的产物；本报告自身未再重跑构建（避免覆盖 Lead 已验收产物）。安装包已于 17:59:45 按该 exe 重打包。

## 6. 行为矩阵（实机，隔离数据目录）

| 场景 | 命令/操作 | 实测 |
|---|---|---|
| 普通启动可见 | `GoclashZ.exe` | hwnd 有效，`IsWindowVisible=True`，标题 `GoclashZ`，1024×768 ✅ |
| `--silent` 隐藏 | `--silent` | `MainWindowHandle=0`；EnumWindows：`WailsWebviewWindow … vis=False`（窗口存在但不可见） ✅ |
| `--startup` 且 SilentStart=false | `--startup` | `IsWindowVisible=True` ✅ |
| 单实例 · 第二实例 `--silent` | 第一实例存活（已 WM_CLOSE 隐藏） | 第二实例 **exit=0，123 ms**；第一实例 `alive=True, visible=False`（**未唤醒**） ✅ |
| 单实例 · 第二实例普通 | 同上 | 第三实例 **exit=0，123 ms**；第一实例 `alive=True, visible=True`（**恢复显示**） ✅ |
| Ctrl+Alt+Q | `keybd_event` (Ctrl+Alt+Q) | 4 个不同实例全部退出，**exit code 0** ✅ |
| **WM_CLOSE** | `SendMessage(hwnd, WM_CLOSE)` | 修复前：6 轮中 2 轮 exit（约 1/3，见 §6.1）。**修复后复测 5/5 通过**：每轮 before=`vis=True`，+10s 后 `exited=False`、`IsWindow=True`、`IsWindowVisible=False` ✅ |

### 6.1 WM_CLOSE 竞态根因（修复前，已定位到 v3.1.0 源码）

`pkg/application/webview_window.go`：

1. 窗口创建时框架注册了**内部 listener**（:378-386）：收到 `WindowClosing` 就 `markAsDestroyed + impl.close + Window.Remove`（**无条件销毁原生窗口**）。
2. `HandleWindowEvent`（:1028-1060）：**先同步跑 hooks**（任一 hook `Cancel()` 则直接 return，不跑 listeners）；**再对每个 listener 起 goroutine 并发执行**，每个 goroutine 只在入口检查一次 `IsCancelled()`。
3. **修复前**本项目用 `mainWin.OnWindowEvent(`WindowClosing`, func(e){ e.Cancel(); mainWin.Hide() })`（`main.go:359-362`）——这是 **listener**，与框架内部 listener 处于同一批并发 goroutine：谁先跑谁赢。
   - 我们的先跑 → `Cancel()` 置位、`Hide()` 生效，内部 listener 在入口看到 cancelled 便跳过 → 隐藏存活（正确）。
   - 内部先跑 → 原生窗口销毁、`Window.Remove`，最后一个窗口消失 → 应用退出（exit 0）。

**修复（Lead 已于 17:58 实施，`main.go:360`；1 行级）**：改用 hook——
```go
// main.go:360（已实施）：OnWindowEvent → RegisterHook，Hide() 放进 hook 内
mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
    e.Cancel()
    mainWin.Hide()
})
```
hooks 同步先跑且 cancel 会阻断后续 listeners，可 100% 复刻 v2 `HideWindowOnClose:true`。
**影响面**：Alt+F4、任务栏/外部 WM_CLOSE、系统关机消息等会命中；前端自定义关闭按钮走 `App.vue handleClose()`→`HideMainWindow()`/`Application.Quit()`，**不受此竞态影响**。v2 语义为「恒隐藏」，故修复前属**行为回归 + 非确定性**。

### 6.2 修复后复核（本次独立复测 5/5 PASS）

- 代码确认：`main.go:358-363` 已改为 `mainWin.RegisterHook(events.Common.WindowClosing, …)`，`e.Cancel()` + `mainWin.Hide()` 均在 hook 内；产物 `bin/GoclashZ.exe` 26,782,208 B **@17:58:17**（`main.go` mtime 17:58:14），VI 仍为 1.4.0/1.4.0/GoclashZ/ZzzDREAM。
- 测试条件：隔离 `GOCLASHZ_DATA_DIR=%TEMP%\r2hook_data`，**未设** `WAILS_DEV_SERVER`，每轮 `SendMessage(hwnd, WM_CLOSE)` 后等待 **10s** 再断言。
- 结果（5/5 PASS，全部满足 `exited=False` + `IsWindow(hwnd)=True` + `IsWindowVisible=False`）：

| 轮次 | 关闭前可见 | +10s 进程存活 | 窗口对象存在 | 关闭后可见 | 判定 |
|---|---|---|---|---|---|
| 1 | True | True | True | False | PASS |
| 2 | True | True | True | False | PASS |
| 3 | True | True | True | False | PASS |
| 4 | True | True | True | False | PASS |
| 5 | True | True | True | False | PASS |

- 与修复前对比：修复前 6 轮 2 轮进程 exit 0；修复后 5 轮 0 退出（Lead 另测 6/6 → 合计 **11/11 存活且隐藏**）。测试进程已全部清理，残留 0。

## 7. 事件实机表现与 stderr

- 每个实例均重定向 stderr 采集（GUI 子系统仍可捕获）：
  ```
  [runtimeassets] seed 必需资产不可用: clash.exe / wintun.dll path=…\bin\seed\core\bin\… error=文件不存在
  [runtimeassets] seed 可选资产不可用，跳过: geoip.metadb / geosite.dat / country.mmdb / asn.dat
  [WebView2] Environment created successfully
  ```
- **未见**任何 `event type mismatch`、`binding`、`tray`、`panic` 字样；15+ 次启停无崩溃（修复前唯一的异常退出为 WM_CLOSE 竞态，修复后已 5/5 复核通过）。
- 隔离数据目录内自动生成 `Settings/default_behavior.json`、`default_desired_state.json`、`user_behavior.json`、`user_desired_state.json` → 说明设置读写链路正常（对应 `GetInitialData/GetAppBehavior` 等绑定方法被前端调用成功）。
- 首屏渲染：本人两次截图尝试失败（第一次 `CopyFromScreen` 抓到的是被其他窗口覆盖的桌面内容，已删除；第二次 `PrintWindow` 调用异常），**改用 Lead 的 `R2-app-screenshot.png` 作为首屏证据，本人未独立验证**。

## 8. 主题实时切换

**代码路径审查 PASS**（`app.go:506-525`）：
```
SaveThemePreference(isDark)
  → utils.SaveGlobalTheme("dark"/"light")          // 持久化 theme_setting.txt
  → w := a.mainWindow(); w32.SetTheme(uintptr(w.NativeWindow()), isDark)   // 原生标题栏/边框配色
  → w.SetBackgroundColour(17,17,17 / 242,242,242)   // 防 WebView 重绘露底
  → a.SyncState()
无任何 restart/重启逻辑；main.go 启动时按 theme_setting.txt 决定初始 Hidden/BackgroundColour
```
**实机点击：未自动验证** —— 需要窗口相对坐标点击侧栏主题按钮，本轮时间盒内未实施（点击定位不可靠）。请人工点一次验证「切换即时生效、不重启、重启后保持」。

> Lead 补充（2026-11-04 18:05）：尝试用 Win32 合成鼠标点击侧栏 `.theme-switch-row`（窗口置前、DPI 感知、多坐标点）以及 WebView2 CDP/UI Automation，均因环境限制未触发（WebView2 远程调试端口未开放、UIA 无 WebView 元素、合成点击未生效）；`theme_setting.txt` 未生成。代码路径、类型检查与前端事件绑定均 PASS，仍需人工点击一次确认。

## 9. 托盘 / 热键实现审查（v3 原生化）

**托盘：与 v2 菜单 1:1（代码审查 PASS）**

| 维度 | v2（自研 Win32） | v3（application.SystemTray） | 一致性 |
|---|---|---|---|
| 菜单顺序 | Show / sep / 系统代理(或端口代理) / TUN / sep / 出站模式子菜单 / sep / 重启内核 / 退出 | 同左（`buildMenu` `tray_runtime_windows.go:326-366`） | ✅ |
| 勾选项 | 系统代理、TUN、Rule/Global/Direct 单选 | `AddCheckbox` / `AddRadio`，`SetChecked` 动态刷新（:431-439） | ✅ |
| 端口代理模式 | PortProxyMode 时文案+勾选改为 DesiredPortProxy | `viewFor()` 同逻辑（:268-274） | ✅ |
| 左键单击 | 无动作 | 显式覆盖 v3 默认 ToggleWindow，改为 no-op（:179-186） | ✅ |
| 双击 | 显示主窗口 | `OnDoubleClick` → ShowMainWindow；另用系统双击间隔兜底两次单击（:176-196） | ✅ |
| 右键 | 重建菜单并弹出 | `OnRightClick` → `applyView(snapshot)` + `ShowMenu()`（:189-192） | ✅ |
| tooltip | AppName - 配置名 / 未选择配置文件 | 同口径（:276-279），`SetTooltip` | ✅ |
| 多语言 | 7 套文案 | `getTrayTexts` 7 套（zh-CN 默认 + zh-TW/zh-HK/en-US/ja-JP/ru-RU/fr-FR/de-DE），字段与 v2 同名项一致 | ✅（抽样比对） |
| 图标 | 内嵌 ICO/PNG 回退 | ICO(`iconData`) → `trayFallbackIconPNG` → nil（:299-323），先校验可解析再使用 | ✅ |
| 命令通道 | PostMessage + 消息循环 | `commandLoop` + `PostCommand`（同一套 TrayCommand 枚举与 handleCommand 逻辑保留） | ✅ |

**热键：v3 application.GlobalShortcut 替换 63 行自研实现**：仍为 `Ctrl+Alt+Q`；注册失败只记日志不影响启动（与 v2 一致）；启动 3s 后回查 `IsRegistered`；`stopHotkey` 注销。**实机已验证热键可退出（4/4 次，exit 0）**。

## 10. 风险与建议

| ID | 级别 | 问题 | 建议 |
|---|---|---|---|
| **R2-1** | ~~高~~ **已修复** | WM_CLOSE 竞态（修复前约 1/3 概率进程直接退出） | ✅ 已改 `main.go:360` `RegisterHook`；修复后复核 **5/5 PASS**（Lead 6/6），见 §6.2，**关闭** |
| **R2-2** | ~~中~~ 已关闭 | 安装包曾为 16:44 旧构件 | Lead 已于 17:59:45 按 R2-1 修复后的 exe 重跑 ISCC，Setup 28,219,815 B |
| **R2-3** | 中 | 主题实时切换、托盘菜单交互均未实机点击验证（仅代码审查） | 人工回归：切主题不重启且重启保持；托盘右键菜单项/勾选/语言/双击显示窗口 |
| **R2-4** | 低 | 单实例锁由 v2 `Global\GoclashZ_Single_Instance_Mutex` 改为 v3 `wails-app-…-sim`（**session 级，无 Global\ 前缀**）；且 v2/v3 实例互不感知 | 如需跨会话互斥需自行加 Global 前缀方案；过渡期避免同时运行 v2/v3 |
| **R2-5** | 低 | 单实例检测点从 v2「main 早期互斥锁」后移到 `application.New`：第二实例会先执行 `NewApp()/MigrateLegacyAppDataToInstallData()/Behavior.Get()` 再退出 | 实测退出仅 123 ms，可接受；若未来 NewApp 变重再前置轻量预检 |
| **R2-6** | 低 | 隔离环境下 `bin/seed/core/bin` 不存在 → 启动日志 6 条 seed 警告 | 属环境差异（正式安装包由 installer 铺 seed），非缺陷；可在日志级别上降噪 |
| **R2-7** | 信息 | 62 个 typed event 中未注册 `geo-update-<key>-*`（动态名） | 保持现状即可（不注册则无类型校验、事件照常送达） |

## 11. 验收结论

- **代码/构建/类型/绑定层面：全部 PASS**。108 方法跨三服务并集与 mapping 完全自洽、无重名缺失；62 个 typed event 声明与生成物一致；前端 facade 21 处收口、旧聚合导入清零；`gofmt` 从上一轮 96 条降到 **0 条**。
- **行为矩阵：15/15 通过**。普通启动/--silent/--startup/单实例（含唤醒与不唤醒）/Ctrl+Alt+Q/WM_CLOSE 全部符合预期且退出码为 0。
- **原阻塞项 R2-1 已修复并复核**：WM_CLOSE 改为 `RegisterHook` 后，本次独立复测 **5/5 存活且隐藏**（Lead 6/6，合计 11/11），与 v2「恒隐藏」语义一致，该项关闭。
- **两项需人工**：主题实时切换点击、托盘菜单实际弹出（代码审查均已 1:1 对齐）。

## 附录 A：环境

| 项 | 值 |
|---|---|
| OS / Shell | Windows x64 / Windows PowerShell 5.1（`pwsh` 不可用，脚本以 `powershell -File` 运行） |
| Go / wails3 | go1.27.0 / wails3 v3.1.0 |
| Node / npm | v24.19.0 / 11.17.0 |
| 被测二进制 | `bin/GoclashZ.exe` 26,782,208 B **@17:58:17**（R2-1 修复后重构建；修复前为 17:44:44） |
| 隔离数据目录 | `%TEMP%\r2beh_data`、`r2beh2_data`、`r2si_data`、`r2m_data`、`r2shot_data` |
| 测试脚本 | `%TEMP%\r2_behavior.ps1`、`r2_single.ps1`、`r2_beh2.ps1`、`r2_wmclose.ps1`、`r2_matrix.ps1`、`r2_shot.ps1` |

## 附录 B：关键命令与退出码

```
go build .                                              exit 0
go vet ./...                                            exit 0
gofmt -l (104 files)                                    flagged 0
go test -count=1 <12 pkgs, 排除 core/sys、build/*>       exit 0  (PASS 10, FAIL 0)
wails3 generate bindings -ts -d frontend/bindings -i -clean=true   exit 0
npx vue-tsc --noEmit                                    exit 0
npm run build                                           exit 0
GoclashZ.exe（普通）                                    窗口可见, hwnd≠0
GoclashZ.exe --silent                                   隐藏, MainWindowHandle=0, vis=False
GoclashZ.exe --startup                                  可见
第二实例 --silent / 普通                                  exit 0 (123 ms)，第一实例未被唤醒 / 被唤醒
Ctrl+Alt+Q (keybd_event)                                实例 exit 0
SendMessage(WM_CLOSE)  [修复后]                          5/5 存活且隐藏（exited=False, IsWindow=True, vis=False）；Lead 另测 6/6
```
