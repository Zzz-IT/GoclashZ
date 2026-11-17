# P4 独立验收报告 — GoclashZ Wails v2 → v3.1.0 迁移

- **验收人**：verifier（独立于 P1/P2/P3 实施者）
- **验收对象**：`D:\GOLANG\GoclashZv3`（Wails v3.1.0 迁移分支，将取代 main）
- **验收时间**：2026-11-04（Asia/Singapore），迁移窗口 16:19–16:41
- **方式**：只读检查 + 可重复的构建/生成/测试命令；**未修改任何业务源码**（仅运行 `wails3 generate bindings`、`npm run build` 重新生成两份被 .gitignore 的产物，见 §3/§4）
- **基线对照**：`D:\GOLANG\GoclashZ`（main 主干，只读）

## 0. 环境

| 项 | 值 |
|---|---|
| OS | Windows（win32 x64） |
| Go | go1.27.0 windows/amd64 |
| wails3 CLI | v3.1.0（`C:\Users\Jiguang\go\bin\wails3.exe`） |
| Node / npm | v24.19.0 / 11.17.0 |
| Inno Setup | 6（用户级 `C:\Users\Jiguang\AppData\Local\Programs\Inno Setup 6\ISCC.exe`） |
| 版本控制 | **v3 目录非 git 仓库（无 .git）**，因此改动范围用「与 main 逐文件比对 + mtime」替代 git diff |
| 规模 | Go 100 文件 / 19,857 行；Vue 26 文件 / 9,033 行；frontend/src *.ts 20 文件（与简报 100/19.5k、26/9k 一致） |

## 1. 结论摘要

| # | 检查项 | 结果 | 一句话证据 |
|---|---|---|---|
| 1a | `go build .` | **PASS** | exit 0，2.2s，无输出 |
| 1b | `go vet ./...` | **PASS** | exit 0，0.8s，无输出 |
| 1c | `gofmt -l` 无输出 | **FAIL** | 96/100 文件被列出，根因为 CRLF 化（非格式化内容；见 R1） |
| 2 | 全仓残留（wails/v2、wailsjs、wails.json、package.json.md5） | **PASS** | 四类均为 0 命中 |
| 3 | bindings 可复现 | **PASS** | 重新生成前后 16 个文件 SHA256 **逐字节一致** |
| 4 | `vue-tsc --noEmit` / `npm run build` | **PASS** | 均 exit 0 |
| 5 | 产物与 VERSIONINFO | **PASS**（1 处既有不一致，见 R2） | GoclashZ.exe 1.4.0/GoclashZ/ZzzDREAM；Helper 无 VI（v2 同样无） |
| 6 | 9 个 core 包单元测试 | **PASS** | exit 0（appcore/backup/clash 有测试，其余无测试文件） |
| 7 | 代码抽查 6 项 | **PASS** | 单实例互斥锁/启动顺序/SetTheme/Event.Emit/5 处 Dialog 归一化/51 处 Events.On |
| 8 | 发布链 | **PASS** | bindings 先于前端构建；wails3@v3.1.0 + ISCC 步骤；iss Source=bin/；**ISCC 实编译 exit 0** |
| — | GUI 启动 smoke | **未验证（由 Lead 完成）** | 按 Lead 指示不重复，避免干扰；见 R6 |
| — | 运行时交互行为（托盘/热键/内核/TUN/自更新） | **未验证** | 需交互式桌面会话并会改系统状态，超出只读验收范围 |

## 2. 检查 1：Go 静态检查

```
go build .        exit=0  elapsed=2.2s    （无输出）
go vet ./...      exit=0  elapsed=0.8s    （无输出）
gofmt -l <root/*.go>                     → 4 个文件
gofmt -l <全仓 100 个 .go>                → 96 个文件
```

**结论：build/vet PASS；gofmt 未通过（FAIL）。**

根因分析（已做完整取证，结论：**迁移代码风格没问题，是行尾符 CRLF 化**）：

1. 逐字节检查：`core/appcore/behavior.go` = CRLF 239 / 裸 LF 0；而 `main.go`（P1 重写）= CRLF 0 / LF 352。
2. 全仓行尾统计：v3 的 100 个 .go 中 **96 个为 CRLF**（CRLF 化前应为 LF）；对照 main 的 103 个 .go 中仅 18 个含 CRLF。
3. 把 100 个文件统一归一为 LF 后再跑 `gofmt -l` → **只剩 8 个**：
   `core/appcore/desired_state.go`、`core/appcore/outbound_ip.go`、`core/clash/component_replace.go`、`core/runtimeassets/catalog.go`、`core/runtimeassets/migrate.go`、`core/runtimeassets/types.go`、`core/sys/shutdown_windows.go`、`core/sys/startup_windows.go`。
4. 对 main 主干做同样处理（LF 归一后 `gofmt -l`）→ **同样恰好是这 8 个文件**。
   ⇒ 这 8 处是 **upstream 既有** 的 gofmt 偏差，不是本次迁移引入；96 条输出中有 88 条纯属行尾符。
5. 本次 4 个被重写的文件（`main.go`、`app.go`、`wails_event_sink.go`、`core/appcore/controller.go`）是 LF 且 gofmt 干净。

> 影响：`wails3 build` 与 `go build` 不受 CRLF 影响（编译通过）；但「取代码 → 替换 main」会产生 **96 个文件的整文件级 diff 噪声**，且任何 `gofmt -l` CI 门禁会红。处置建议见 R1。

## 3. 检查 2：v2 残留全仓扫描

| 目标 | 命令/方式 | 结果 |
|---|---|---|
| `wailsapp/wails/v2` import | grep `wails/v2`，include `*.go` | **0 命中** PASS |
| 全仓任意位置 v2 引用 | grep `wailsapp/wails/v2`（含 go.mod/go.sum） | **0 命中**（go.mod 已无 v2，go.sum 亦无残留） PASS |
| `frontend/src` 下 wailsjs | grep `wailsjs` | **0 命中** PASS |
| `frontend/wailsjs/` 目录 | glob `**/wailsjs/**` | **0 文件** PASS |
| `wails.json` | glob `**/wails.json` | **不存在** PASS |
| `frontend/package.json.md5` | glob `**/package.json.md5` | **不存在** PASS |
| `frontend/bindings` 由 Taskfile 生成 | `build/Taskfile.yml` + `.gitignore` | `common:generate:bindings` 为 `build:frontend` 的 dep；`frontend/bindings/` 在 .gitignore 中 PASS |

前端运行时导入已全部切到 v3：`@wailsio/runtime` 8 处、`from '../../bindings/goclashz'` 类导入 23 处。

## 4. 检查 3：bindings 可复现性

```
备份 frontend/bindings → %TEMP%\bindings_backup_verify（16 文件 / 34,036 B）
SHA256 清单（生成前）→ 清单哈希 80CB9BC57238B2FC07799F8C1D43518E57900E8F296F670108358F6ED4012764
wails3 generate bindings -ts -d frontend/bindings -i -clean=true
   → exit 0
   → "Processed: 284 Packages, 1 Service, 108 Methods, 4 Enums, 36 Models, 0 Events in 854.6672ms"
SHA256 清单（生成后）→ 清单哈希 80CB9BC5…2764  → Compare-Object: IDENTICAL
```

**结论 PASS：16 个文件重新生成后逐字节一致（无时间戳/顺序噪声）。** 完整哈希见附录 A。

方法数交叉核对（重要发现，见 R7）：

| 口径 | 数量 |
|---|---|
| `*App` 全部方法（含未导出） | 120 |
| `*App` 导出方法 | 109（其中 `ServiceStartup` 是 v3 生命周期方法，不进 bindings） |
| 进入 bindings 的导出方法 | **108** |
| v2 main 的 `frontend/wailsjs/go/main/App.d.ts` | **108** |
| v2 方法与 v3 方法集合比对 | **完全一致（108/108，名字逐一对齐）** |

⇒ 简报中的「110 个导出方法」不准，实际 **108**（+1 个 ServiceStartup）；**对外 API 与 v2 1:1 等价**。

## 5. 检查 4：前端

```
cd frontend
npx vue-tsc --noEmit    exit=0  elapsed=3.2s   （0 error）
npm run build           exit=0  elapsed=3s
  [settings-manifest] OK，87 个设置项锚点全部就位
  vite v8.0.10 … ✓ 153 modules transformed
  dist/index.html 0.38 kB
  dist/assets/index-CXB-vc8S.css  99.33 kB │ gzip: 15.06 kB
  dist/assets/index-DX1mv5SY.js  983.88 kB │ gzip: 298.99 kB
  dist/assets/logo-CeRV8_D7.ico  217.83 kB
  （另有 dist/webfonts/* 21 个字体文件）
```

两类非阻断告警（见 R4）：npm 11 提示 `frontend/.npmrc` 的 `minimum-release-age` 已不再被识别；单 chunk > 500 kB 的 vite 体积告警。

## 6. 检查 5：构建产物与 VERSIONINFO

| 产物 | 大小 | 时间 | VERSIONINFO |
|---|---|---|---|
| `bin/GoclashZ.exe` | 26,382,336 B（25.16 MB） | 16:33:40 | **FileVersion=1.4.0, ProductVersion=1.4.0, ProductName=GoclashZ, CompanyName=ZzzDREAM**, FileDescription=A modern Clash client, Copyright (C) 2026 ✅ |
| `bin/GoclashZHelper.exe` | 3,464,704 B（3.30 MB） | 16:33:41 | **无 VERSIONINFO**（v2 基线同样无；构建命令与 v2 相同 `go build -ldflags "-s -w"`，见 R-note） |
| `build/installer/GoclashZ_win_amd64_Setup.exe` | 27,886,716 B（26.59 MB） | 16:34:04 | FileVersion=**1.4.0.0**, ProductVersion=1.4.0, ProductName=GoclashZ, CompanyName=**Zzz** |

交叉核对：`build/config.yml`（companyName=ZzzDREAM / productName=GoclashZ / version=1.4.0）与 `build/windows/info.json`（file_version 1.4.0、CompanyName ZzzDREAM）**与主 exe 的 VI 完全一致** ✅。

v2 基线对照（`D:\GOLANG\GoclashZ\build\bin\`，2026-10-30 18:59）：`GoclashZ.exe` 26,710,528 B **VI 全空**、`GoclashZHelper.exe` 3,464,192 B 无 VI。
⇒ v3 主 exe 反而**新增**了完整版本资源（wails3 由 info.json 生成），非回归。Helper 无 VI 属既有行为，本次未劣化。

## 7. 检查 6：单元测试

```
go test -count=1 ./core/appcore ./core/backup ./core/clash ./core/downloader ./core/utils \
                ./core/runtimeassets ./core/logger ./core/tasks ./core/traffic
exit=0  elapsed=3.7s
ok  goclashz/core/appcore   1.932s
ok  goclashz/core/backup    1.278s
ok  goclashz/core/clash     1.916s
?   goclashz/core/downloader   [no test files]
?   goclashz/core/utils        [no test files]
?   goclashz/core/runtimeassets[no test files]
?   goclashz/core/logger       [no test files]
?   goclashz/core/tasks        [no test files]
?   goclashz/core/traffic      [no test files]
```

**PASS，无失败用例。** 按要求**未运行 `core/sys`** 测试（会改注册表/计划任务）。
覆盖说明：9 个包中仅 3 个实际含测试；其余 6 个仅验证「可编译」。`core/sys`（注册表/计划任务/SCM）与 `core/version`、`cmd/goclashz-helper` 无自动化覆盖运行时行为（见 R5）。

## 8. 检查 7：关键代码路径抽查

### 7.1 单实例：保留 v2 自研 Win32 互斥锁（PASS）
`main.go:213-256`：`CreateMutex(nil,false,"Global\\GoclashZ_Single_Instance_Mutex")`（:215）→ 判 `ERROR_ALREADY_EXISTS`（:220）→ `--silent`/`--startup` 时静默退出不唤醒（:223-229；v2 的静默碰撞修复保留）→ 否则 `sys.FocusMainWindowAndFlashTwiceWin32Only()`（:234）→ `defer CloseHandle`。
全仓无 `SingleInstanceOptions` 使用，符合架构决策 3。

### 7.2 `ServiceStartup` 启动顺序与 v2 完全一致（PASS）
`app.go:206-261` 与 main 的 `app.go:150 func (a *App) startup` 逐步比对，11 个步骤顺序逐一相同：
`cleanupAppliedUpdatePackage` → `a.ctx=ctx` + `sink.SetContext` → `runStartupRuntimeAssetMaintenance` → `clash.LoadIndex` → `clash.MigrateRuleStorageV2` → `Behavior.Get` → `core.Bootstrap`（IsStartupLaunch/Silent 口径一致）→ helper `RegisterParent` 异步重试 → 窗口可见性 → `SyncState` → `startHotkeyWorker` + `tray.Start`。
唯一差异（符合架构决策 5）：v2 的 `runtime.WindowShow/Hide` 改为 `main.go` 的 `Hidden: isSilent` + `a.setWindowVisible()`，静默时仍保留 `sys.SafeHideMainWindowWin32` 与 `sys.EnsureMainWindowHiddenSilently` 兜底。

### 7.3 主题（PASS）
`app.go:518-540 SaveThemePreference`：持久化 `utils.SaveGlobalTheme` → `w.NativeWindow()` 取 HWND → `w32.SetTheme(uintptr(hwnd), isDark)`（:530）复刻 v2 标题栏/边框配色 → `w.SetBackgroundColour(17,17,17 / 242,242,242)` 防重绘露底 → `SyncState`。
`main.go:261-277` 启动时读取 `theme_setting.txt` 决定初始 `BackgroundColour`（默认 #111111），与 v2 默认夜间一致。

### 7.4 事件出口单点（PASS）
`wails_event_sink.go:26-52`：`WailsEventSink.Emit` 保留 nil 接收者保护 + **ctx 判据（启动前事件丢弃，行为与 v2 一致）** + panic recover → 终点 `application.Get().Event.Emit(name, args...)`（:51）。
另有 `app.go:83-89 emitEvent()`（仅 1 处调用：`app.go:198` runtime-assets-status），同样终点为 `app.Event.Emit`。
全仓 `.Emit(` 调用 **85** 处，均通过 `appcore.EventSink` 接口（`core/appcore/events.go:5`）→ 唯一实现 `WailsEventSink`；直接调用 v3 终端 API 的位置全仓**只有 2 处**（`wails_event_sink.go:51`、`app.go:88`），故事件出口仍是单点。

### 7.5 文件对话框取消归一化（PASS，实为 5 处）
| 位置 | 方法 | 归一化 |
|---|---|---|
| `app.go:790` | SelectLocalFile (OpenFile) | `dialogCancelled(err)` → `FileInfo{}, nil` |
| `app.go:1147` | ExportConfig (SaveFile) | → `nil` |
| `app.go:1326` | ExportBackup (SaveFile) | → `"CANCELLED", nil` |
| `app.go:1353` | SelectBackupFile (OpenFile) | → `"", nil` |
| `core/appcore/controller.go:1968` | ExportDiagnostics (SaveFile) | inline 判 `"cancelled by user"` → `nil` |

`dialogCancelled`（`app.go:100-104`）统一封装 v3 的 `cfd.ErrorCancelled`。
v2 main 同样有 **5** 个对话框调用点（app.go 718/1069/1245/1269 + controller.go 1968）⇒ 调用点数量与取消语义 **1:1 对齐**（任务描述的“4 处”是 app.go 内的 4 处）。

### 7.6 前端事件订阅（PASS）
`frontend/src`：`Events.On(` **51 处**；`EventsOn`（v2 写法）**0 处**；`@wailsio/runtime` 导入 8 处。

## 9. 检查 8：发布链

### 9.1 `build/Taskfile.yml`：bindings 先于前端构建（PASS）
- `build:frontend`（:72）`deps`: `install:frontend:deps` + **`generate:bindings`**（:87-94），`run: once`；
- `generate:bindings`（:176-194）命令：`wails3 generate bindings -ts -d frontend/bindings -f '{{.BUILD_FLAGS}}' -clean=true [-obfuscated] -i`；
- `build/windows/Taskfile.yml`：`build:native` 的 deps 含 `common:go:mod:tidy`、`common:build:frontend`、`common:generate:icons`、`generate:syso`。

### 9.2 `.github/workflows/release.yml`（PASS）
- `WAILS3_VERSION: "v3.1.0"`（:11）→ `go install "github.com/wailsapp/wails/v3/cmd/wails3@$ver"`（:56）+ `wails3.exe version` 校验（:63）；
- `setup-go`（go-version-file: go.mod）、`setup-node` 22 + npm 缓存；
- `wails3 generate bindings -d frontend/bindings -ts`（:89）+ 存在性断言（:91）；
- `wails3 build`（:97）；helper `go build -ldflags "-s -w" -o bin/GoclashZHelper.exe ./cmd/goclashz-helper/`（:103）；
- `Validate build outputs` 断言 ProductVersion 匹配发布版本、ProductName=GoclashZ（:106-124）；
- Inno：`choco install innosetup`（:68）+ `ISCC.exe /DMyAppVersion=<ver> package.iss`，带 `Get-Command` 兜底（:131-141）；
- 便携版 zip/7z、安装包归档（:143-177）。

### 9.3 `package.iss`（PASS）
- `Source: ".\bin\GoclashZ.exe"`、`Source: ".\bin\GoclashZHelper.exe"`（已从 v2 的 `build\bin\` 改到 v3 的 `bin\`）；
- seed 资产仍取自 `build\runtime-assets\core\bin\*`；`OutputDir=.\build\installer`；
- `AppMutex=Global\GoclashZ_Single_Instance_Mutex` 与程序内互斥锁一致；`VersionInfoVersion={#MyAppVersion}.0` → 1.4.0.0。

### 9.4 ISCC 实编译（PASS，本次独立重跑）
```
ISCC.exe /O%TEMP%\iscc_verify_out /DMyAppVersion=1.4.0 package.iss
exit=0   elapsed=8.9s   "Successful compile (8.844 sec)"
产出 %TEMP%\iscc_verify_out\GoclashZ_win_amd64_Setup.exe  27,886,716 B
  FileVersion=1.4.0.0  ProductVersion=1.4.0  ProductName=GoclashZ  Company=Zzz
```
- 输出目录重定向到 `%TEMP%`，**仓库内 `build/installer`、`bin` 文件 mtime/大小均未变化**（16:34:04 / 16:33:40 原样）。
- 与 Lead 已产出的 `build/installer/GoclashZ_win_amd64_Setup.exe` **字节数完全相同**（27,886,716），说明构建可复现。
- 唯一告警：`Warning: Line 190: [Hint] Variable 'RESULTCODE' never used`（v2 既有写法，非阻断）。

## 10. 未验证项（明确边界）

| 项 | 原因 |
|---|---|
| GUI 启动 smoke（窗口 1024x768、WebView2 renderer 数、UI 渲染） | Lead 已用隔离数据目录完成并截图确认；按指示 verifier **不重复**，避免 WebView2/单实例互斥锁互相干扰。故该项**由 Lead 佐证，非 verifier 独立验证** |
| 托盘菜单、全局热键、主题切换、代理内核启停、TUN、系统代理写入、自更新 | 需交互式桌面会话且会改变系统网络/注册表状态，属写操作，超出只读验收范围 |
| GitHub Actions 真实执行 | 本地无法复现 CI 环境；已用「静态审阅 + 本地 ISCC 编译 + 本地等价构建链」替代 |
| git 级改动范围界定 | v3 目录非 git 仓库（无 .git）；改用「与 main 逐文件比对 + mtime」 |

## 11. 风险与建议

| ID | 级别 | 问题 | 影响 | 建议 |
|---|---|---|---|---|
| **R1** | **中高** | 96/100 个 .go 为 CRLF（main 仅 18/103 含 CRLF），导致 `gofmt -l` 输出 96 行 | 替换 main 时产生整文件级 diff 噪声；gofmt CI 门禁红 | 合并前统一行尾：`gofmt -w`（同时可顺手修掉 8 处 upstream 偏差）或在仓库根加 `.gitattributes`（`*.go text eol=lf`）；建议同时执行 `gofmt -w` 让门禁变绿 |
| **R2** | 低 | 安装包 VERSIONINFO 的 CompanyName/Copyright 为 `Zzz`（`package.iss` 硬编码），主 exe 为 `ZzzDREAM` | 资源管理器属性/签名信息不一致（**v2 既有**，非本次回归） | 若统一为 ZzzDREAM，改 `#define MyAppPublisher` / `VersionInfoCompany` / `VersionInfoCopyright` 三行即可；不影响功能 |
| **R3** | 中 | 本次验收中 `npm run build` 重新生成了 `frontend/dist`（16:40），而 `bin/GoclashZ.exe` 是 16:33 构建的 | 二进制内嵌的前端产物与当前 dist 时间戳不同源（源文件未变，理论内容一致） | 交付/发版前让 Lead 再跑一次 `wails3 build`，确保 exe 与最终 dist 对齐（CI 上本就每次重建，无风险） |
| **R4** | 低 | 前端两类告警：`npm warn Unknown project config "minimum-release-age"`；单 chunk 983.88 kB > 500 kB | 仅告警，不影响产物 | 清理 `frontend/.npmrc` 中已废弃的 `minimum-release-age`；体积告警可接受（v2 同量级），如在意可后续 code-split |
| **R5** | 低 | 自动化覆盖偏窄：9 个包中仅 appcore/backup/clash 有测试；`core/sys`（注册表/计划任务/SCM/代理）按要求跳过 | v3 迁移改动集中在 main/app.go/sink/controller，已被 build+vet+手动路径审阅覆盖；但系统集成层无自动化回归 | 后续可为 `core/sys` 增加不触碰系统状态的纯函数测试；正式发布前手工回归「系统代理/开机自启/计划任务」三项 |
| **R6** | 低 | GUI smoke 仅由实施方（Lead）自证，无第二方独立证据 | 首帧/渲染问题属 v3 已知风险点 | 建议保留 Lead 的截图与 renderer 计数作为发版依据；如需强证据，可在隔离数据目录下由另一人复跑一次 |
| **R7** | 信息 | 简报写「110 个导出方法」，实测 = 109 导出（含 ServiceStartup）+ **108 进 bindings**，与 v2 的 108 完全一致 | 文档口径偏差，容易误判「少了 2 个方法」 | 更正 V3_MIGRATION_BRIEF.md 为「108 个绑定方法（另 ServiceStartup 为 v3 生命周期方法）」 |
| **R8** | 中 | main 主干 `D:\GOLANG\GoclashZ` 工作区不干净：`M README.md`、`M README_en.md`、`M go.mod`(go 1.25→1.26)、`?? FYNE_MIGRATION_ASSESSMENT.md`；mtime 16:02–16:03，**早于迁移窗口 16:19** | 非本次迁移造成（无 16:19 之后的写入），但替换 main 前需决定这些改动去留 | Lead 在切换 main 前处理/提交或丢弃；不影响 v3 分支本身 |
| **R9** | 低 | `frontend/bindings`、`frontend/dist` 均在 .gitignore | 新克隆需先 `npm i` 并生成 bindings 才能 `vue-tsc` | 属 v3 标准工作流（`build:frontend` 已把 generate:bindings 设为 dep），无需改动 |

## 12. 验收结论

- **代码级验收：通过**。`go build`/`go vet`/bindings 生成/前端类型检查/前端构建/单元测试/发布脚本 全部 0 退出；v2 API 面 **108/108 完全对齐**，事件出口、单实例、对话框取消语义、启动顺序均与 v2 等价。
- **唯一 FAIL 项是仓库卫生层面的 CRLF**（R1），不影响编译与运行，但必须在替换 main 前处理，否则 diff 噪声与 gofmt 门禁会阻塞评审。
- **需 Lead 决策的三件事**：R1 行尾统一（建议 `gofmt -w`）、R3 发版前重建一次、R8 main 主干脏工作区处置。

---

## 附录 A：`frontend/bindings` 文件 SHA256（重新生成前后一致）

清单哈希：`80CB9BC57238B2FC07799F8C1D43518E57900E8F296F670108358F6ED4012764`（生成前 = 生成后）

```
events/index.ts                                        B103683F94EC8779E49311845E4246FDA26504184B133B850CDFF8969AB5D068
github.com/wailsapp/wails/v3/internal/eventcreate.ts   6EAEAFCF797A1670E8AE925E7D125EF4EFF930171874462E37E767B1C52CF7ED
github.com/wailsapp/wails/v3/internal/eventdata.d.ts   1D7B9F4209D8CB01D7F848C848B53DC582B8DD4DF44B09FF3A001C2CEF455858
goclashz/app.ts                                        BBA7169D30DA5608A5A0FD31D97265993EB09EC39598B708BBBBC756B9C6A5D7
goclashz/core/appcore/index.ts                         ADA6745115A60E0E23BB039C3FCF4D08AB600313B872676FA20F198D36AFB684
goclashz/core/appcore/models.ts                        F368CBE57D49632ACC7AE16F57B82A7A6FFB64443D23E81EEAFE678DFF8C6FCB
goclashz/core/clash/index.ts                           DD1622B558DA9AEBE732B9CAADDFC0E5FCE4B6F5AD201480C07BF9AD871A3C48
goclashz/core/clash/models.ts                          11844CA70BC43664F3B0864C2D77F193BCAB0C0CC82A5957502C8D32A711AEA7
goclashz/core/logger/index.ts                          F1BAD777C3155C6FFE0CA660F3C08F11D5D5418F1A7D417B3941611038587341
goclashz/core/logger/models.ts                         04B6804E39C2D478E95D5A9D212993C9B5957FDC86B4F13BC2CE2C15845EF6F8
goclashz/core/runtimeassets/index.ts                   B23BF8D3E4B2C3902A784A7418625C01C9F5518714324EA92B2E7D88231F3078
goclashz/core/runtimeassets/models.ts                  B2429960330FF82E67A620B31C04510B0AA42F05BE0DA75A00201D541EAFA6AD
goclashz/core/sys/index.ts                             A3D5C46AF7B5F62217D7492C952B4BB3498939F2C025F85E4544C7A9EF9F7AAF
goclashz/core/sys/models.ts                            60D970DA03E1CF9288FE071DBE1D3112EA815A018C5B694EC45191841FCB9F0C
goclashz/index.ts                                      E886BABC844C1F52C2D265CA57E9F42DC9B8384268A10CADA931F7B3139A922A
goclashz/models.ts                                     FD772E6CFD439A6E426312844A0649508F7E54AD449345C40005B41870752E97
```

## 附录 B：本次验收执行的命令（可复现）

```powershell
cd D:\GOLANG\GoclashZv3
go build .                                  # exit 0
go vet ./...                                # exit 0
gofmt -l .\*.go                             # 4 files
gofmt -l (全仓 100 个 .go)                   # 96 files（CRLF，见 R1）

wails3 generate bindings -ts -d frontend/bindings -i -clean=true   # exit 0

cd frontend
npx vue-tsc --noEmit                        # exit 0
npm run build                               # exit 0

cd ..
go test -count=1 ./core/appcore ./core/backup ./core/clash ./core/downloader \
  ./core/utils ./core/runtimeassets ./core/logger ./core/tasks ./core/traffic   # exit 0

ISCC.exe /O%TEMP%\iscc_verify_out /DMyAppVersion=1.4.0 package.iss             # exit 0
```

> 说明：`frontend/bindings` 与 `frontend/dist` 是被 .gitignore 的生成物，本次验收重新生成了两者，未触碰任何业务源码；即 R1 的 96 个 CRLF 文件在验收前后内容一致（仅 bindings/dist 为生成物）。

---

## 附录 C：Lead 复核补充（2026-11-04 16:45，R1 / R3 已闭环）

### C1. R1 行尾与格式（已修复）

- 将 106 个 `.go` 中 96 个 CRLF 文件统一归一为 LF（UTF-8 无 BOM），新增 [.gitattributes](../.gitattributes)：
  `*.go text eol=lf`、`*.bat/*.cmd/*.ps1 text eol=crlf`、二进制类型 `binary`。
- 然后全量 `gofmt -w`（顺带修掉 upstream 既有的 8 处格式问题）。
- 复核：`gofmt -l` **无输出**；`go build .` **exit 0**；`go vet ./...` **exit 0**。

### C2. R3 产物对齐（已重跑）

| 命令 | 退出码 | 产物 | 时间 |
|---|---|---|---|
| `wails3 build` | **0** | `bin/GoclashZ.exe` 26,382,336 B | 16:44:21 |
| `go build -ldflags "-s -w" -o bin/GoclashZHelper.exe ./cmd/goclashz-helper/` | **0** | `bin/GoclashZHelper.exe` 3,464,704 B | 16:44:21 |
| `ISCC.exe /DMyAppVersion=1.4.0 package.iss` | **0** | `build/installer/GoclashZ_win_amd64_Setup.exe` 27,883,452 B | 16:44:38 |

主 exe VERSIONINFO 复核：FileVersion/ProductVersion **1.4.0**、ProductName **GoclashZ**、CompanyName **ZzzDREAM**。

### C3. GUI 运行时验证（Lead 实测，v3.1.0 首帧问题确认已消失）

- 隔离数据目录启动：`GOCLASHZ_DATA_DIR=%TEMP%\goclashz-v3-final-smoke2`、`WAILS_DEV_SERVER=1`（跳过单实例，避免与 D:\GoclashZ 生产实例的互斥锁冲突）。
- 结果：窗口 **1024x768**、标题 **GoclashZ**、WebView2 renderer 子进程 **2 个**，界面完整渲染（控制台/代理节点/当前连接/实时日志/配置规则/订阅管理/软件设置）。
- 截图：[app-screenshot.png](app-screenshot.png)。
- 关闭语义：向主窗口发 `WM_CLOSE` 后 **visible=False 且进程存活** → v2 `HideWindowOnClose` 的“关闭即隐藏到托盘”已 1:1 复刻。
- 测试进程已 `taskkill /T`、临时数据目录已删除，未改动系统代理/TUN/内核状态。

### C4. 发版前仍需人工处理

1. 本地 `build/runtime-assets/core/bin` 缺 `clash.exe`：需运行 `scripts/fetch_runtime_assets.ps1`（或走 CI fetch）后再重打安装包；当前本地 Setup 不含内核 seed。
2. 未实测：安装/升级/卸载、自动更新、托盘菜单、全局热键、TUN/系统代理/UWP 等会修改系统状态的运行路径。
3. 主干 `D:\GOLANG\GoclashZ` 工作区本就存在未提交改动（README×2、go.mod、FYNE 评估文档），非本次迁移造成；替换 main 前需单独处理。
4. 本目录尚未初始化 git（用户要求不提交）；合并时建议以 `D:\GOLANG\GoclashZ` 仓库新建/更新分支后提交本目录内容。
