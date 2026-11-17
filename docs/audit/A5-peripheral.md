# A5 外围子系统审计（更新 / 下载 / 任务框架 / 备份 / 配置事务 / 提权 / 托盘）

> 生成：2026-11-16；角色：A5 只读审计（不改任何源码）
> 证据等级：**A** = 代码可确证；**B** = 代码 + 运行时状态一致推断；**C** = 假设，待实测
> 唯一写入物：本文件。审计期间未运行应用、未构建、未 kill 进程。

---

## 0 覆盖范围与方法

### 0.1 逐文件覆盖（范围内 100% 通读）

| 文件 | 行数（约） | 读法 |
|---|---|---|
| `core/appcore/updater_orchestrator.go` | 588 | 全文 |
| `core/appcore/component_update_service.go` | 187 | 全文 |
| `core/appcore/component_update_state.go` | 102 | 全文 |
| `core/appcore/geo_update_manager.go` | 328 | 全文 |
| `core/appcore/subscriptions.go` | 163 | 全文 |
| `core/appcore/backup_service.go` | 132 | 全文 |
| `core/appcore/config_service.go` | 52 | 全文 |
| `core/appcore/config_editor_controller.go` | 40 | 全文 |
| `core/appcore/desired_state.go` | 176 | 全文 |
| `core/appcore/behavior.go` | 539 | 全文 |
| `core/appcore/cleanup.go` | 106 | 全文 |
| `core/tasks/manager.go` | 122 | 全文（整个 `core/tasks/**` 只有这 1 个文件） |
| `core/downloader/{types,errors,clients,utils,small_file,large_asset,github,app_update}.go` | 约 1180 | 8 个文件全文（`core/downloader/**` 全覆盖） |
| `core/backup/archive.go` | 746 | 全文（`archive_test.go` 未读） |
| `core/sys/{win,uwp_win,admin_win,startup_windows,shutdown_windows}.go` | 约 1180 | 全文 |
| `core/sys/{helper_client_windows,helper_protocol}.go` | 约 520 | 全文（提权/helper 链路必需） |
| `core/runtimeassets/migrate.go` | 172 | 全文 |
| `tray_runtime_windows.go` | 665 | 全文 |
| `hotkey_windows.go` | 82 | 全文 |

合计范围内约 **7,000 行**。

### 0.2 为判读结论而额外读的关联文件（只读）

`main.go`（全文）、`app.go`（重点段 60-400 / 600-840 / 1240-1440）、`v3_events.go`（全文）、`wails_event_sink.go`（全文）、`core/logger/buffer.go`（全文）、`core/utils/{settings,atomic}.go`（全文）、`core/clash/{setup,sub,config,config_editor}.go`（相关函数）、`core/appcore/{controller,supervisor}.go`（相关函数）、前端 `store.ts`(700-880)、`UpdateTaskRow.vue`(1-96)、`App.vue`(700-730)、`Subscriptions.vue`(495-515)。

外部依赖源码核对（用于判定“事件/主线程/下载库”真实语义，非本仓代码）：
- `github.com/wailsapp/wails/v3@v3.1.0`：`pkg/application/{mainthread.go, event_manager.go, events.go, events_strict.go, application.go:1002-1044}`
- `github.com/cavaliergopher/grab/v3@v3.0.1`：`client.go`（断点续传/Range 行为）

### 0.3 检索手段

1. 全量通读上表文件；
2. `grep` 调用点统计：`Tasks.Manager` 用法、`componentUpdateMu` 三处持锁、`logger.*` 在 12 个外围文件中的出现次数（结论：**总共 2 处**）、`FetchSmallFileAtomic/DownloadLargeAssetAtomic` 调用点、`ClearOwnedSystemProxy/DisableSystemProxy` 调用点、`Supervisor.Reconcile*` 调用点；
3. **Go 发射侧 ↔ 前端监听侧 对照**：把 `v3_events.go` 注册表 + `Emit(...)` 字面量与 `frontend/src` 的 `Events.On` 逐个对照，得到“死事件”清单（§2 F-A5-06/F-A5-23）；
4. 事件类型校验走读 wails v3 源码 `events.go:339-368`（推翻方案 §10.3 的一条判断，见 §3.3）；
5. 时间预算对照：把 `HelperStopCoreRPCTimeout()`(30s)、`safeQuit` 8s、`ServiceShutdown` 5s、`http.Client.Timeout`(10min)、`grab` 重试矩阵放在一起算最坏耗时。

### 0.4 方法局限（诚实声明）

- 未运行应用 → 所有“阻塞时长/是否弹窗”的判定来自代码路径 + 既有 ctx 预算，**不是实测**；凡涉及“用户实际看到什么”，我给 A（代码路径确定）但不给计时实测值。
- 不掌握火绒/杀软对 `core/bin` 的实际拦截行为，相关放大倍数一律 C 级。
- 前端只读了与本审计相关的监听/渲染片段，未做全站 UI 审计（属 A0/A4 范围）。

---

## 1 链路清单表

> 严格度口径：**严** = 校验内容/哈希/PE/进程探测；**中** = 结构校验 + 限额；**—** = 无校验只做动作。
> 单次成本栏：若为“无总超时”，列出的是**代码允许的最坏值**（由 `http.Client.Timeout`、重试矩阵、`INFINITE` 等推出，A 级），不等于典型值。

| # | 入口/场景 | file:line | 严格度 | 单次成本（最坏） | 是否阻塞用户 | 误报面 | 触发频率 |
|---|---|---|---|---|---|---|---|
| 1 | 检查软件更新（手动） | `app.go:1299-1302` → `updater_orchestrator.go:402-448` | 中（只读 API） | 12s ctx 上限；成功后 fn 自带 `app-update-check-start/end` 复位 | 否（异步任务） | 失败文案靠 `includes('github')` 等关键字猜（`updater_orchestrator.go:563-587`） | 手动点击 |
| 2 | 检查软件更新（启动自动） | `updater_orchestrator.go:507-510, 413-416` | 中 | 同上 | 否 | **失败完全静默**：`manual=false` 时既不 emit 也不写日志 | 每次启动（`AutoUpdate` 默认 true，`behavior.go:499`） |
| 3 | 下载软件更新包（~40MB） | `app.go:1296-1298` → `updater_orchestrator.go:481-546` → `downloader/app_update.go:362-413` | 严（MZ+≥1MB+可选 SHA256） | **无总超时**：单 client `Timeout=10min`（`downloader/clients.go:37`）× 2 client × 3 候选 URL × 6 次尝试（`large_asset.go:49-63`） | 否，但**无取消入口**（不进 `UpdateTasks`，前端 cancel 只认任务行 key，`UpdateTaskRow.vue:68-75`） | 网络类失败统一文案（`updater_orchestrator.go:571-578`） | 用户点“下载” |
| 4 | 检查内核更新 | `updater_orchestrator.go:148-193` | 中 | 12s | 否 | 任务框架派生 `core-update-check-start` 被前端消费（`store.ts:787`），复位却依赖 fn 内部事件 → 契约脆弱（见 F-A5-23） | 手动/自动 |
| 5 | 更新内核（61MB 事务） | `app.go:1247-1249` → `updater_orchestrator.go:80-146` → `component_update_service.go:25-187` | 严（zip 结构 + PE + 停核替换） | 下载无总超时 + 停核 + 替换 + 起核 + 等 API/适配器 | 否（任务面板），可取消（`controller.go:277-283`） | “准备失败/提交失败/内核恢复失败”三套字符串（`:94/:145/:167`），前端无 code 可用 | 手动 |
| 6 | 重装 Wintun 驱动 | `app.go:740-742` → `component_update_service.go:25-187` | 严 | 同 #5（文件小，但同样停核/起核） | 否 | `driver-install-error` **前端 0 监听**（F-A5-06） | 手动 |
| 7 | 用种子内核替换 | `app.go:1259-1261` → `updater_orchestrator.go:314-394` | 严（PE 5MB~300MB + `WaitFileReleased` 5s） | 同步：读 61MB 种子 + 停核 + 替换 + 起核（`EnsureCoreRunning` 在内） | **是（同步 Wails 调用）** | 假失败面：`WaitFileReleased` 超时/占用 → “替换内核文件失败”，与“权限不足”同一文案 | 手动 |
| 8 | 更新单个 geo 库 | `app.go:1267-1269` → `geo_update_manager.go:213-250` | 中（≥1KB + 扩展名） | 无总超时；`MaxBytes` 200MB 事后判定（`large_asset.go:193-197`） | 否 | `geo-update-<key>-error` **前端 0 监听**；且 `-start` 在拿信号量之前就发（`:138` 早于 `:140`）→ 排队显示成“下载中” | 手动 / 启动补齐 |
| 9 | 批量更新 geo（4 库） | `app.go:1271-1273` → `geo_update_manager.go:252-327` | 中 | 并发 2（`:15`），无总超时 | 否 | `geo-update-all-start/success/error` **前端 0 监听** | 手动 |
| 10 | 更新全部订阅 | `app.go:439-441` → `subscriptions.go:46-68` | 中（`StrictVerifyClashConfig` + 引用校验，`clash/sub.go:103-116`） | 每订阅 10min 上限，**串行**（`:54`） | 否（异步任务） | **所有错误被吞，恒报 success**（F-A5-02） | 手动 |
| 11 | 更新单个订阅 | `app.go:435-437` → `subscriptions.go:26-44` → `clash/sub.go:51-150` | 中 | 10min 上限（2 client 并发竞速） | **是（同步 await）** | 空 body → 假成功（F-A5-07） | 手动/定时 |
| 12 | 新增订阅 | `subscriptions.go:11-24` | 中 | 同上 | **是** | 同上 | 手动 |
| 13 | 导出备份 | `app.go:1383-1408` → `backup_service.go:16-18` → `archive.go:33-120` | —（只打包不解包） | 全量复制 Settings/Subscriptions/profiles + zip；**无体积上限、无进度、无取消** | **是（同步）** | `zw.Close()` 错误被丢 → 坏包报成功（F-A5-10） | 手动 |
| 14 | 还原备份 | `app.go:1427-1434` → `backup_service.go:21-132` → `archive.go:130-203` | 中（manifest + 1000 文件/300MB/单文件 50MB） | 停核 → 解压（≤300MB）→ 覆盖 → `Supervisor.Reconcile`（用 UI ctx，**无 deadline**） | **是**，且 **ctx 被完全忽略**（F-A5-09） | 回滚失败静默（F-A5-09） | 手动 |
| 15 | 保存 TUN/DNS/网络设置 | `app.go:815-833` → `config_service.go:10-38` | 中（`SaveSetting` 原子写） | 写盘 + 若内核在跑则 `RestartCore`；`UpdateTunConfig` 内同步 spawn `-v`（最长 5s，`config.go:242` → `check.go:157`） | **是** | 保存成功但重启失败 → 配置已落盘无回滚 | 手动 |
| 16 | 保存配置文本 | `config_editor_controller.go:12-39` → `clash/config_editor.go:55-99` | 严（YAML + 引用完整性） | 校验 + 原子写 + 若在跑则 `ReloadConfig` | 是 | 重载失败时文件已保存（错误文案自认，`:31`） | 手动 |
| 17 | 选择本地配置（未运行） | `subscriptions.go:101-151` | 中 | Behavior + desired 两次写盘 | 是 | `Desired.Update` 失败后**没有回滚 apply 的 Behavior**（`:110-117` vs `:142-147`） | 手动 |
| 18 | 删除当前配置 | `subscriptions.go:70-99` | 中 | 关代理 → 关 TUN → 删文件 → 清 desired | 是 | 非事务；3 处 `_ =` 吞错（`:87/:92`） | 手动 |
| 19 | 开机自启信息/修复 | `app.go:602-612` → `startup_windows.go:146-247`；启动路径 `controller.go:343,446-496` | 中 | 注册表读；**落空时同步 spawn `schtasks`，无超时**（`startup_windows.go:108-112`） | 是 | “检查”里写注册表（`:120-136`）；`startup-task-*` 事件前端 0 监听 | 打开设置页 / 每次启动 |
| 20 | 提权类操作（装/删/重启 helper、修数据目录权限、UWP 豁免） | `app.go:622-661, 708-725, 777-797` → `admin_win.go:128-190` | — | UAC + `WaitForSingleObject(INFINITE)`（`:172`），子进程 `SW_HIDE`（`:158`） | **是：无超时、不可取消、用户看不到子进程** | “用户取消 UAC”与“子进程失败”给同类文案 | 手动 |
| 21 | 以管理员身份重启 | `app.go:769-771` → `admin_win.go:39-71` | — | `os.Exit(0)`（`:69`） | 是（进程直接消失） | 跳过全部清理（F-A5-03） | 手动 |
| 22 | 托盘命令（代理/TUN/模式/重启/退出） | `tray_runtime_windows.go:497-535` | — | 单 goroutine 串行；开 TUN 可达 20~45s（`app.go:370-374` 25s + 适配器等待） | **是**：命令通道满 10 条即丢弃（`:247-255`） | 失败一律 `source=tray` → 前端**模态**（`store.ts:752-776`） | 手动 |
| 23 | 全局退出热键注册 | `hotkey_windows.go:34-70` | — | 失败只写内存日志（`:45`） | 否 | 用户永远不知道热键失效 | 启动一次 |
| 24 | 启动期迁移/清理 | `main.go:56`、`app.go:167`、`controller.go:310` → `runtimeassets/migrate.go`、`appcore/cleanup.go` | 严（哈希对账） | `MigrateLegacyAssets` 对 6 个现存资产各算 1 次 SHA256（`migrate.go:33-41`）≈91MB；`CleanLegacyFiles` 通配删除 | **是（在 `ServiceStartup` 内，UI 出现前）** | 种子/旧目录异常 → 启动变慢但**无结构化日志**（`log.Printf` 进 stderr，`:129/:138`） | 每次启动 |
| 25 | 退出链路 | `app.go:251-276`（`ServiceShutdown`）、`app.go:1456-1472`（`safeQuit`） | — | `safeQuit` 8s 后 `os.Exit(0)`；`ServiceShutdown` 对停内核只给 5s；而 stop-core RPC 预算 30s（`helper_protocol.go:52-54`） | 是 | 预算不匹配 → 代理/内核残留（F-A5-13） | 每次退出 |

### 1.1 三个横向事实（先给结论，后面逐条引用）

- **F1 日志事实**：本审计 12 个外围文件（downloader 8 个、tasks 1 个、geo 1 个、component_update_service 1 个、backup_service 1 个、backup/archive 1 个、config_service 1 个、config_editor_controller 1 个、subscriptions 1 个、behavior 1 个、desired_state 1 个、migrate 1 个）里，`logger.*` 调用**总共 0 处**；整个 A5 范围只有 2 处（`updater_orchestrator.go:535`、`cleanup.go:98`）。而 `logger` 本身只写 500 条内存环（`core/logger/buffer.go:82,100-116`，无任何文件 IO）→ **这 12 个子系统的全部失败在进程退出后不可复盘**。A
- **F2 事件事实**：任务/更新类事件里有 6 个“Go 侧在发、前端无人听”的死通道：`driver-install-error`、`driver-install-cancelled`、`geo-update-all-start/success/error`、`startup-task-unhealthy`、`startup-task-check-warning`、`notify-error`（`v3_events.go:48,90-92,165-174` 注册 vs 前端 grep 命中 0）。A
- **F3 锁事实**：`componentUpdateMu` 只有 3 个持有者（`component_update_service.go:58`、`updater_orchestrator.go:324`、`backup_service.go:27`），三者都是“先 `componentUpdateMu` 再 `coreLifecycleMu`”，**锁序一致、无死锁**；问题是持锁跨越整个下载，且这 3 个入口都没有 `TryLock`/超时（F-A5-05）。A

---

## 2 问题清单

> 排序：P0 → P1 → P2；同级按“可造成的用户可见损失”降序。每条给证据等级。

### P0

#### F-A5-01 任务框架的 goroutine 没有 recover：任务内 panic 直接杀进程
- **严重度**：P0　**证据等级**：A
- **位置**：`core/tasks/manager.go:45-68`（`Run`）、`:88-110`（`RunIfIdle`）
- **现象**：两个 goroutine 都是 `m.events.Emit(name+"-start"); err := fn(taskCtx)` 直调，**没有任何 `defer recover`**。而 `main.go:37-43` 的 recover 只在主 goroutine 上，Go 的 panic 不会跨 goroutine 传播 → 任务里的 panic 变成**进程级 fatal**。
- **影响**：所有走任务框架的重活（内核更新 61MB、软件更新 40MB、订阅批量、geo 批量）任一 panic（downloader 里 `resp.Request.HTTPRequest` 空指针、grab 内部、map 并发写等都在这个调用栈里）→ 应用瞬间消失，**无栈、无日志、无通知**（GUI 子系统无 stderr，见 F1）。用户视角是“点更新，软件没了”。
- **修法**：在 `manager.go:45` 与 `:88` 的闭包体首行加
  `defer func(){ if r:=recover(); r!=nil { logger.Errorf("task %s panic: %v\n%s", name, r, debug.Stack()); m.events.Emit(name+"-error", fmt.Sprintf("任务内部错误: %v", r)); m.mu.Lock(); delete(m.tasks,name); m.mu.Unlock() } }()`；
  更彻底的做法是把 `runTask` 抽成一个私有函数，两个入口共用同一份 recover+清理逻辑，避免再出现两份拷贝。

#### F-A5-02 「更新全部订阅」把所有失败吞掉并报成功
- **严重度**：P0　**证据等级**：A
- **位置**：`core/appcore/subscriptions.go:54-66`（关键 `:56-61`）
- **现象**：循环里 `id, err := clash.DownloadSub(...)` 的 `err` 只用来判断 `needsRestart`，**从不聚合、从不返回、从不记录**，函数恒 `return nil`。
- **影响**：任务框架因此发 `subs-update-success`（`manager.go:65-67`）→ 前端 `Subscriptions.vue:503` 弹“更新成功”并刷新列表；同时触发 `scheduleOutboundIPRefresh`。实际可能**全部订阅都 404/超时**，用户以为已拿到新节点，继续用过期订阅 → 表现为“订阅更新了但还是连不上”。
- **修法**：`subscriptions.go:54-61` 收集 `failed []string`（`fmt.Sprintf("%s: %v", item.Name, err)`），循环后：
  `if len(failed)>0 { logger.Errorf("订阅批量更新失败 %d/%d: %s", len(failed), len(items), strings.Join(failed, "; ")); return fmt.Errorf("%d 个订阅更新失败: %s", len(failed), strings.Join(failed, "; ")) }`。
  这样 `manager.go:61` 会发 `subs-update-error`，前端 `Subscriptions.vue:509` 已有处理分支。

#### F-A5-03 提权重启在库函数里 os.Exit(0)，跳过全部清理
- **严重度**：P0　**证据等级**：A（代码路径）；后果为 B（未实测）
- **位置**：`core/sys/admin_win.go:63-70`（`RequestAdmin`）、`:90-96`（`RequestAdminWithArgs`）、调用方 `app.go:769-771`（`RestartAsAdmin`）
- **现象**：`windows.ShellExecute` 成功后立即 `os.Exit(0)`。这条路径上：(1) 不调 `sys.DisableSystemProxy()`、不 `c.DisableAll()`（对比正常退出 `controller.go:900-902 → 1390-1401`）；(2) 不写日志（F1：写了也不落盘）；(3) 不给 helper 做善后。
- **影响**：用户在“系统代理已开/TUN 已开”时点“以管理员身份重启”（设置页常见操作）→ 老进程瞬间消失、注册表 ProxyServer 仍是 `127.0.0.1:7890`、内核子进程随之退出 → **整机断网**且无提示。下次正常启动时 watchdog 只在 `desired` 有 true 时才调和（`supervisor.go:123-129`），全 false 时不触发 `disableAll`，残留代理**可能一直保留**（B 级）。
- **修法**：在 `ShellExecute` **之前**执行 `sys.ClearOwnedSystemProxy()` + `c.DisableAll()`（幂等，`proxy_guard_win.go:115` 已实现“只清本程序的代理”）；并且不要在 `sys` 包里 `os.Exit`——改为返回错误由 `app.go` 决定退出。

### P1

#### F-A5-04 提权子进程「无限等待 + 隐藏窗口 + 无取消」
- **严重度**：P1　**证据等级**：A
- **位置**：`core/sys/admin_win.go:144-175`（`:158` `sei.NShow = 0`，`:172` `WaitForSingleObject(..., windows.INFINITE)`）
- **现象**：以 `SW_HIDE` 启动管理员子进程后无限等待退出码，**没有超时、没有 ctx、没有可取消通道**。6 个 UI 调用点全走它：`app.go:634`（装 helper）、`:646`（删）、`:654`（重启）、`:714`（修数据目录权限）、`:722`（修 core 布局）、`:790`（UWP 豁免）。
- **影响**：子进程执行 `RecoverHelperServiceForUser`（`helper_client_windows.go:305-348`），内部 `Stop` + `Start` + `WaitForHelperReady(30, 300ms)` 最长约 9s，失败还走 `delete → sleep 1200ms → rebuild → start`（`:325-340`）。若卡在 SCM `STOP_PENDING` 或弹任何系统对话框，**用户在界面上看不到任何东西**（窗口隐藏），只能看设置页转圈，唯一出路是结束进程，而服务/权限可能处于中间态。
- **修法**：`admin_win.go:172` 改 `WaitForSingleObject(sei.HProcess, 60_000)`，超时返回错误并 `TerminateProcess`；`app.go` 六个入口先发一个“等待 UAC/正在提权”的事件。

#### F-A5-05 组件更新锁跨越整段下载，其他入口无超时阻塞
- **严重度**：P1　**证据等级**：A
- **位置**：`component_update_service.go:58-59`（`componentUpdateMu.Lock(); defer Unlock()` 覆盖 `:63` 的 `Prepare` 下载）；同类 `updater_orchestrator.go:324-325`（`ReplaceCoreWithSeed`）、`backup_service.go:27-28`（`RestoreBackup`）
- **现象**：`Prepare` 阶段是 61MB 下载（`clash/setup.go:204` → `DownloadLargeAssetAtomic`，无总超时），**全程持锁**；另两个入口用阻塞 `Lock()`，没有 `TryLock`、没有 ctx。
- **影响**：用户在“内核更新中”点“恢复内置组件/用种子替换/还原备份”→ 该 Wails 调用**永久挂起**（最长等于下载最坏耗时），前端只有一个不会返回的 await，没有 busy 提示（`component_update_service.go:34` 的守卫只在**同一 taskName** 时生效，跨入口无效）。
- **修法**：两段式——`Prepare`（下载）**不持** `componentUpdateMu`，只在 `Commit`（`:109-133` 前后、真正动文件/停核的窗口）持锁；`ReplaceCoreWithSeed`/`RestoreBackup` 改用 `TryLock` 并立即返回 `ErrComponentUpdateBusy`（前端映射成非阻塞 toast）。

#### F-A5-06 三类更新失败的可见性为零：无日志、无提示、只有任务行
- **严重度**：P1　**证据等级**：A
- **位置**：`driver-install-error` 发在 `component_update_service.go:99/148/170`、注册在 `v3_events.go:167`，**前端 0 监听**；`geo-update-all-error` 发在 `geo_update_manager.go:322`，**前端 0 监听**，`geo-update-<key>-error`（`:197`）同样 0 监听（前端只有 `-success`，`store.ts:853`）；`startup-task-unhealthy` / `startup-task-check-warning` 发在 `controller.go:487/449`，**前端 0 监听**；三者全部**不写 logger**（F1）。
- **影响**：Wintun 重装失败后界面**没有任何反应**（失败文案本身就是 helper/权限类字符串，具体措辞未逐字核对，B）；只有 设置→组件与库更新 页、任务行还活着、状态是 error 时才在 `UpdateTaskRow.vue:33-35` 看到一行小字。自启项异常同理，静默到用户下次开机才发现没自启。
- **修法**：(a) `component_update_service.go:99/148/170`、`geo_update_manager.go:197/322`、`controller.go:449/487` 各补一行结构化日志；(b) 前端在 `store.ts:779-785` 的 `component-update-tasks-sync` 处理器里，对**新增**的 `status==='error'` 条目路由一次 toast（不是模态）。

#### F-A5-07 FetchSmallFileAtomic：空响应体返回成功；读错误被吞；超限截断不报错
- **严重度**：P1　**证据等级**：A
- **位置**：`core/downloader/small_file.go:66`（`body, _ := io.ReadAll(r)`）、`:73-92`（判定与返回）、`:62-65`（`LimitReader(MaxBytes+1)` 但**从不比较**）
- **现象**：(1) 只有 `res.err == nil && len(res.body) > 0` 才算成功；服务端 `200 + 空 body` 时走 `lastErr = res.err`（= **nil**），循环后 `return SanitizeDownloadError(nil)` → **返回 nil**。(2) `io.ReadAll` 错误被丢弃：连接中途断开 → 半截 body 且 err=nil → 当作成功。(3) `MaxBytes` 只用于 LimitReader，从不校验 `len(body) > opt.MaxBytes` → 超 50MB（`clash/sub.go:83`）的订阅被**静默截断**。
- **影响**：`DownloadSub`（`clash/sub.go:79-117`）空 body 时拿不到新内容也不报错，随后 `:129-135` 把**旧 origin** 复制成工作文件 → “更新成功但内容没变”；首次添加时则在 `:129-131` 因 `os.ReadFile` 失败返回与网络无关的内部错误。截断路径目前被子的 `StrictVerifyClashConfig`（`sub.go:109-111`）挡住，属下游兜底而非下载机保证。
- **修法**：`small_file.go` 三处补齐：读错误返回；成功后校验 `opt.MaxBytes`；循环结束 `if lastErr == nil { lastErr = fmt.Errorf("所有候选均返回空响应") }`。

#### F-A5-08 订阅下载单次上限 10 分钟且是同步调用
- **严重度**：P1　**证据等级**：A
- **位置**：`core/downloader/clients.go:19-40`（`http.Client{Timeout: 10 * time.Minute}`，`:37`）；入口 `app.go:431-437` → `clash/sub.go:79`（ctx = `a.ctx`，无 deadline，见 `app.go:199`）
- **现象**：`UpdateSub`/`UpdateSingleSub` 是**同步 Wails 方法**，下载用 10min client，且 ctx 是应用生命周期 ctx（无超时）。服务端“发完 header 后卡住”是最常见的机场故障形态。
- **影响**：点“更新订阅”→ 按钮转圈最长 10 分钟，无进度、无取消、无超时提示（`FetchSmallFileAtomic` 也没有 `OnProgress`）。`UpdateAllSubsAsync` 串行 N 个订阅 → 最坏 N×10min 的“假死更新”，直接违反方案 §2.1 “≤100ms 有反应”的契约。
- **修法**：`app.go:431-437` 改成 `ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)`；`clash/sub.go:79` 传 `AttemptsPerEndpoint:1`；在 `clients.go` 里区分“大文件/小文件”两套 timeout，而不是共用一个 10min。

#### F-A5-09 备份还原：ctx 完全不生效；回滚失败静默且恒返回 nil
- **严重度**：P1　**证据等级**：A
- **位置**：`core/backup/archive.go:130-203`（函数收 `ctx`，**函数体内一次都没用**）、`:332-357`（`rollbackRestorePlan` 里 4 处 `_ = copyDir/copyFile`，末尾 `return nil`）、`:196-200`（调用方只把 err 拼进消息）
- **现象**：从解压（最多 300MB/1000 文件）到 `backupCurrentTargets`（全量复制）到 `applyRestorePlan`（`os.RemoveAll(dst)` 后重建）**没有任何 ctx.Done() 检查点**；失败回滚把每个错误都丢掉。
- **影响**：还原不可中断；且**回滚失败时仍返回 “恢复执行失败，已尝试自动回滚: %v”**（`:199`）——“已尝试”掩盖了“回滚也失败、用户数据已丢”的事实。`applyRestorePlan:255-264` 的 `RemoveAll` + `copyDir` 窗口是真实的丢数据窗口。
- **修法**：`archive.go:257` 起每个目标前 `if err := ctx.Err(); err != nil { return err }`；`rollbackRestorePlan` 改为返回 error 并在 `:198` 如实上报“回滚也失败”。

#### F-A5-10 导出备份把 zip.Close() 的错误丢掉：坏包可被判成功
- **严重度**：P1　**证据等级**：A
- **位置**：`core/backup/archive.go:95-119`（`:99` `defer f.Close()`、`:102` `defer zw.Close()`、`:104` `return filepath.Walk(...)`）
- **现象**：zip 中央目录在 `zw.Close()` 里写，其错误（磁盘满、文件被占用）被 defer 丢弃；`return filepath.Walk(...)` 只反映遍历/写 entry 的错误。
- **影响**：用户导出备份 → 界面显示 SUCCESS（`app.go:1403-1407`）→ 文件其实截断/无中央目录，等要还原时才发现。同函数还用 `os.ReadFile`（`:113`）把每个文件整个读进内存，profiles 稍大就是内存峰值 + 无进度。
- **修法**：收尾改显式顺序：walk 错误 → `zw.Close()` 错误 → `f.Sync()` → `f.Close()`，逐项返回；`:113` 换 `io.Copy(w, f)` 流式写。

#### F-A5-11 删除当前配置不是事务，且 3 处错误被 _ 吞掉
- **严重度**：P1　**证据等级**：A
- **位置**：`core/appcore/subscriptions.go:70-99`（`:75-81` 先关代理/TUN、`:83` 删文件、`:87` `c.Behavior.SetActiveConfig("")` 返回值被丢、`:92` `_ = c.Desired.SetAndSave(desired)`、`:94` `c.StopCoreProcess()`）
- **现象**：不是事务：关代理 → 关 TUN → 删文件 → 清 desired，任一中间步失败没有反向操作；最要命的是 `:92` desired 保存失败被丢弃。
- **影响**：desired 仍指向已删除配置 → 下次启动恢复时找不到配置，配合 `supervisor.go:149-152` 的兜底取 `Behavior.ActiveConfig`（也可能是旧值）→ 用户开机后看到“未选择配置”，而之前开的代理/TUN 期望值仍在 → 状态错乱。另外 `:75-81` 已关代理/TUN，若 `:83` 删除失败，用户回到“什么都没开”且只收到一句错误。
- **修法**：`:83-96` 包成 apply/rollback 结构（与 `SelectLocalConfig:110-129` 同款）；至少修正 3 处 `_`：`:87`、`:92` 失败必须返回错误，且返回前 `c.SyncState()`；`:83` 失败也要先 `SyncState`。

#### F-A5-12 helper RPC 的默认 10s deadline 罩住了 61MB 文件替换
- **严重度**：P1　**证据等级**：A（代码）；触发概率 B
- **位置**：`core/sys/helper_client_windows.go:26-44`（`default: return 10 * time.Second`，`:41-43`）；受影响方法 `replace-core-file`（`:155-168`，被 `clash/setup.go:247-254` 与 `updater_orchestrator.go:353-358` 调用）、`install-wintun`（`:171-184`）、`repair-permission`（`:142-152`）
- **现象**：只有 `ping/core-status/register-parent/start-core/stop-core` 有专门预算，其余全部吃 `default 10s`。`replace-core-file` 在 helper 侧要做“备份 + 复制 61MB + rename”。
- **影响**：客户端 `SetDeadline` 到点 → `decode response failed: i/o timeout` → `CommitCoreUpdate` 失败（`setup.go:253`）→ 事务进入“提交失败 → 尝试恢复运行”（`component_update_service.go:133-149`）→ 用户看到“内核更新失败：通过 Helper 替换内核失败”，**但文件其实已替换成功**；下次点更新又提示“已是最新”。典型“假失败”。
- **修法**：`helper_client_windows.go:27-44` 显式加 `case "replace-core-file", "install-wintun": return 60 * time.Second`、`case "repair-permission": return 30 * time.Second`，并把 `default` 降到 5s。

#### F-A5-13 退出链路三套预算互相打架：8s 硬退 vs 5s 上限 vs 30s 优雅停机
- **严重度**：P1　**证据等级**：A
- **位置**：`app.go:1464-1469`（`safeQuit` 8s 后 `os.Exit(0)`）、`app.go:262-273`（`ServiceShutdown` 对 `StopCoreService` 只等 5s）、`core/sys/helper_protocol.go:28-54`（优雅 20s + 强杀 5s + 余量 5s = **30s**，客户端 `helper_client_windows.go:37-40` 直接用该值）
- **现象**：停内核走 `StopCoreService → DisableAll → stopCoreProcessLocked`（`controller.go:900-902 → 1390-1401`），而 `sys.DisableSystemProxy()` 在 `:1401`，即**停内核之后**；`ServiceShutdown` 只给这个 goroutine 5s，超时后主线程继续退出（只记一条 warn 到内存环）。若走带 `confirmStop` 的路径（RPC 预算 30s），5s 必然截断。
- **影响**：TUN/代理开启状态下退出，可能停在“`stopCoreProcessLocked` 未返回 → `DisableSystemProxy` 从未执行”的窗口，`safeQuit` 8s 兜底再硬退 → 系统代理残留（同 F-A5-03 后果，触发面更宽）。
- **修法**：`ServiceShutdown` 里把清理系统代理提到停内核**之前**（或先跑一次幂等的 `sys.ClearOwnedSystemProxy()`）；5s 提到 `HelperStopCoreRPCTimeout()`；`safeQuit` 硬退前再补一次 `ClearOwnedSystemProxy()`。

### P2

#### F-A5-14 MigrateLegacyAssets 每次启动多算 6 次 SHA256（≈91MB），且日志走 stderr
- **严重度**：P2　**证据等级**：A
- **位置**：`core/runtimeassets/migrate.go:33-41`（`targetHash, _ = calculateSHA256(target)` 对每个已存在目标无条件执行）、`:54`（每个 legacy 候选再来一次）、`:129/:138`（`log.Printf`）
- **现象**：在 `app.go:167` 的 `runStartupRuntimeAssetMaintenance` 最前面被调用，**UI 出现之前**。正常安装下 6 个资产都存在 → 启动路径多出 61MB(clash) + 0.4MB(wintun) + 4 个 geo(≈30MB) 的哈希，与 `EnsureReady → GetStatus` 的哈希（`status.go:19-27` → `check.go:128/268/302`）**重复**。
- **影响**：方案 §1.1#16 把启动成本记为“Migrate + 2×EnsureReady”，**漏了这 6 次哈希**；冷启动+杀软首扫时是秒级开销，且完全无日志（`log.Printf` 在 GUI 子系统没有 stderr）。
- **修法**：`migrate.go:38-41` 先 stat 再判定——只有 legacy 候选确实存在时才计算 target 哈希；`:129/:138` 换 `logger.Infof`。

#### F-A5-15 大文件下载的「换 URL 就丢断点」防护是死代码
- **严重度**：P2　**证据等级**：A
- **位置**：`core/downloader/large_asset.go:78-88`（读 `tmpPath + ".meta.json"`）、`:33`/`:85`（只删）、`appcore/controller.go:304`（只删）
- **现象**：全仓 grep `meta.json` 只有“读+删”，**没有写入方**；`grab/v3` 自身也不写这个文件（`grab@v3.0.1/client.go:255-280` 的续传只依赖目标文件大小 + Range）。
- **影响**：分支永不成立 → 断点续传只按“目标文件大小”续，**跨候选 URL 复用**。镜像 A 下 30MB 失败、下一轮首个候选是镜像 B 时，grab 会对 B 发 `Range: bytes=30MB-` → 若两镜像内容不同则得到损坏文件。目前被下游校验挡住（app 更新 `VerifyGitHubSHA=true`（`app_update.go:401`）校验失败删 tmp 重试；内核 zip 有 `validateKernelZip`），因此**不是数据损坏的直接风险，而是“以为有防护其实没有”的脆弱点**。
- **修法**：要么在下载前**写入** `{URL}` 到 `tmpPath+".meta.json"` 让校验生效，要么删掉 `:78-88` 并在 `:30` 起手就 `os.Remove(tmpPath)` 显式放弃跨轮续传。

#### F-A5-16 acquireHostLimiter 是阻塞信号量，不响应 ctx
- **严重度**：P2　**证据等级**：A
- **位置**：`core/downloader/utils.go:77-93`（`:88` `githubHostSem <- struct{}{}`）
- **现象**：GitHub 域并发上限 4，获取时**不看 ctx**。若 4 个下载都卡住（10min client timeout），第 5 个任务在这里干等，即使它的 ctx 已取消（用户点了“暂停”）。
- **影响**：`CancelUpdateTask → Tasks.Cancel` 已取消 ctx，但协程可能卡在信号量上直到前 4 个自然超时，用户看到“已暂停”却仍在跑。
- **修法**：改签名收 ctx，用 `select { case sem <- struct{}{}: case <-ctx.Done(): return nil, ctx.Err() }`；`:76` 调用点传入 ctx。

#### F-A5-17 开机自启的“检查”带写副作用，且同步 spawn schtasks 无超时
- **严重度**：P2　**证据等级**：A
- **位置**：`core/sys/startup_windows.go:108-112`（`exec.Command("schtasks","/Query",...)` + `cmd.Run()`，无 ctx/超时）、`:120-136`（`checkAndMigrateLegacyTask` 在“查”的时候调 `CreateStartupTask` 写注册表 Run 项）
- **现象**：`CheckStartupTask`（`:146`）在 Run 键不存在时进入 `checkAndMigrateLegacyTask`，它会 spawn 一个无超时的 `schtasks` 子进程，且一旦发现旧计划任务就**直接写注册表**把自启项建起来 —— 用户没做任何操作。
- **影响**：两个触发点：启动期 `controller.go:343 → 446-447`，以及 UI 方法 `app.go:602-604 GetStartupTaskInfo`（打开设置页）。杀软环境下 `schtasks` 偶发卡数秒，Wails 调用跟着卡；与方案批评的“检查里夹带写入”（§1.2-B）是同一类反模式，只是发生在自启链路。
- **修法**：`startup_windows.go:109` 改 `exec.CommandContext(ctx, ...)` 并传 2s ctx；把迁移写入从 `CheckStartupTask` 拆成独立的 `RepairStartupTask`（`app.go:606` 已有入口），检查函数只读。

#### F-A5-18 BehaviorStore.SetAndSave 是读-改-写，且先写 cache 后写盘
- **严重度**：P2　**证据等级**：A
- **位置**：`core/appcore/behavior.go:118-130`（`s.mu.Lock(); s.cache = b; ...; return utils.SaveSetting(...)`）
- **现象**：所有调用方都是 `b := c.Behavior.Get(); b.X = ...; c.Behavior.SetAndSave(b)` 的读-改-写（`updater_orchestrator.go:267-273`、`controller.go:453-495` 四处、`updater_orchestrator.go:396-400`）。两个并发保存不同字段 → 后者覆盖前者；且 cache 先落、磁盘后落，写盘失败时内存与磁盘不一致，多数调用点用 `_ =` 吞掉（`controller.go:459/469/494`、`updater_orchestrator.go:272`）。
- **影响**：启动期 `syncStartupTaskStateSafe`（`controller.go:446-496`）与前端保存设置可能并发 → 用户刚保存的主题/模糊设置被自启同步覆盖；`EvaluateCoreComparison` 自动关闭 `CoreComparisonPrompt`（`updater_orchestrator.go:270-273`）同样可能吞掉别人的修改。属“静默丢配置”。
- **修法**：加 `BehaviorStore.Update(func(b *AppBehavior))`（与 `DesiredStateStore.Update`（`desired_state.go:89-103`）同款：锁内拷贝-修改-写盘-提交 cache），把上述 6 处改走它。

#### F-A5-19 LoadSetting 解析失败静默回落默认，且不提示不改名
- **严重度**：P2　**证据等级**：A
- **位置**：`core/utils/settings.go:29-42`（`:30` unmarshal 失败后**掉到** `:39-42` 返回 defaults 且 err == nil）
- **现象**：`user_behavior.json` 损坏（断电写坏、手改错）→ 不报错、不备份、不写日志，直接当“没有用户配置”处理。而 `BehaviorStore.Load`（`behavior.go:154-160`）的错误分支因此**永远不会被执行**。
- **影响**：一次损坏 = 全部设置静默重置；用户随后改任意一项 → `SaveSetting`（`:46-70`）用默认值+这一项**覆盖**原文件 → 备份都不剩，全程零日志。
- **修法**：unmarshal 失败分支里把原件改名为 `*.corrupt-<ts>` + 写日志 + **返回错误**让上层决定是否提示（配合方案 §5.2 `CONFIG_INVALID`）。

#### F-A5-20 托盘命令串行执行且满队列静默丢弃
- **严重度**：P2　**证据等级**：A
- **位置**：`tray_runtime_windows.go:247-255`（`PostCommand` 满 10 条 `default:` 丢弃）、`:451-466`（`commandLoop` 单 goroutine 串行调 `handleCommand`）
- **现象**：命令在同一个 goroutine 里顺序执行，单条可能阻塞数十秒（`TrayCmdToggleTun → ToggleTunFromCurrentDesired`，`app.go:370-374` 给 TUN 的预算就是 25s，内部还有 20s 适配器轮询）。阻塞期间队列最多堆 10 条，超出**无提示丢弃**（包括 `TrayCmdQuitApp`）。
- **影响**：开 TUN 的几十秒里连点托盘（或点“退出程序”）→ 命令被吞，用户以为点到了；与 `store.ts:752-776` 的模态叠加，体验更差。
- **修法**：`PostCommand` 的 `default` 补一行 warn 日志；把 `TrayCmdQuitApp`/`TrayCmdShowMainWindow` 走旁路（直接 go 或独立通道），不与长事务共用队列。

#### F-A5-21 全局退出热键失败后用户不可知
- **严重度**：P2　**证据等级**：A
- **位置**：`hotkey_windows.go:41-47`（失败只 `a.logApp("warn", ...)`）、`:56-69`（3s 后回查仍失败也只 log）
- **现象**：`Ctrl+Alt+Q` 被其它程序占用时静默失败；`logApp` → `controller.go:266-275 LogApp → appLogger.AddApp` → 只进 500 条内存环 + 前端日志面板。
- **影响**：用户以为有一键退出，实际没有；窗口隐藏、托盘图标被系统折叠时找不到退出方式。
- **修法**：失败时走一次 `app-notification`（`level=warn`，`source=startup`），让前端至少有 toast 通道（不需要模态）。

#### F-A5-22 检查函数里写文件：IsCoreBinWritable 与 UpdateTunConfig 的隐藏进程探测
- **严重度**：P2　**证据等级**：A（`CoreSupportsMips` 是否复用缓存为 C）
- **位置**：`core/appcore/updater_orchestrator.go:70-78`（往 `core/bin` 写 `.write_test`，失败即 return false 且**不清理**半截文件）、`core/clash/config.go:241-246`（`UpdateTunConfig` 内 `CoreSupportsMips(context.Background())` → spawn `clash.exe -v`，5s 超时）
- **现象**：`IsCoreBinWritable` 在 `UpdateCoreComponentAsync:82` 与 `ReplaceCoreWithSeed:328/:353` 被调用，每次都在受杀软保护的 `core/bin` 里建删文件（正是方案 §0.2 事故里“目录时间戳被动过”的同类动作）；`UpdateTunConfig` 在“保存设置”路径里同步起进程。
- **影响**：都会在冷启动/杀软扫描窗口把“检查/保存”变成“可能失败或变慢”，并给事后复盘留下假的文件系统痕迹；`-v` 冷启动超时 5s 会让“保存 TUN 设置”卡 5s。
- **修法**：`updater_orchestrator.go:70-78` 改 `O_CREATE|O_EXCL` + 无条件 `defer os.Remove`，或改用 ACL 查询不动磁盘；`config.go:242` 的 `CoreSupportsMips` 结果按 version 缓存（**需先确认是否复用** `check.go:145-155` 的 60s 缓存，此点 C）。

#### F-A5-23 事件契约有死链，也有“同名双发”
- **严重度**：P2　**证据等级**：A
- **位置**：`v3_events.go:48`（`notify-error`）、`:167-168`（`driver-install-error/cancelled`）、`:135-139`（`geo-update-all-*`）、`:90-92`（`startup-task-*`）；发射点 `controller.go:1373`、`tray_runtime_windows.go:493`、`component_update_service.go:99/148/170`、`geo_update_manager.go:322`、`controller.go:449/487`
- **现象**：(a) 上述 6 个事件 Go 侧在发、前端零监听（F2），`notify-error` 更是注册了但无任何消费者；(b) `core-update`/`driver-install` 的 `-start/-error/-success` 由**任务框架和事务内部各发一次**（`component_update_service.go:46` vs `manager.go:46`），`v3_events.go:157-162` 注释已承认；`taskName` 同时是 `UpdateTasks` 的 key 和任务名，任一处改名就会触发 wails 的**类型校验取消**（`events.go:356-367`：类型不符 → `Cancel()` + 上抛 error → 事件静默消失）。
- **影响**：现在“没坏”只是因为所有注册类型恰好对齐，属“靠巧合正确”。一旦有人把 `-error` 的 payload 从 string 换成结构体，事件会被框架丢弃，只在 `handleError` 里出现（`event_manager.go:40-42`）。
- **修法**：删掉 `notify-error` 死事件（或补前端消费）；`component_update_service.go:46`/`:99/:148/:170/:182` 的自发事件删掉，统一交任务框架发；在 `v3_events.go` 加一次启动自检，列出“有发射点未注册/已注册无发射点”的名字并写日志。

#### F-A5-24 CleanLegacyFiles 在 core/bin 无差别删 *.zip/*.tmp/*.meta.json
- **严重度**：P2　**证据等级**：A（代码）/ B（风险）
- **位置**：`core/appcore/cleanup.go:37-52`；被 `controller.go:310`（Bootstrap）调用
- **现象**：对 `GetCoreBinDir()` 做 `*.tmp`、`*.zip`、`*.meta.json` 三个通配删除，而 `core/bin` 正是内核更新（`clash.update.zip`，`setup.go:188`）、Wintun 更新（`wintun.dll.zip`，`controller.go:297`）、geo 下载（`<db>.tmp`/`.tmp.meta.json`）的工作目录。
- **影响**：单实例下启动期没有并发下载，当前**不构成 bug**；但“清理器与工作目录同名空间重叠 + 无差别通配”是明确的可维护性风险：任何人以后把下载目录改成 `core/bin` 子路径、或加多开场景，就会删掉正在下载的断点。
- **修法**：`cleanup.go:38-52` 从“扩展名通配”改为**固定文件名列表**，或限定 `mtime < now-24h`。

#### F-A5-25 Shutdown 事件创建失败会导致启动即退出
- **严重度**：P2　**证据等级**：A（代码路径）/ C（触发条件）
- **位置**：`core/sys/shutdown_windows.go:33-45`（`CreateEventW` 返回 0 时函数**直接 return**）、`main.go:402-407`（`ListenForShutdownEvent()` 返回后**无条件** `ClearOwnedSystemProxy(); os.Exit(0)`）
- **现象**：全局命名事件 `Global\GoclashZ_Shutdown_Event` 创建若失败（权限/命名空间/session 隔离），`ListenForShutdownEvent` 立即返回；上层没有区分“被通知”与“失败返回”，直接当成“安装器请求退出”并 `os.Exit(0)`。
- **影响**：潜在“双击后闪退且无任何日志”。触发条件我无法确证（标准用户能否创建 `Global\` 命名事件受系统策略影响），故标 C；但代码上缺一个返回值是确定的。
- **修法**：`shutdown_windows.go:33` 改返回 error，`handle == 0` 时返回错误；`main.go:402-406` 改为 `if err := ...; err != nil { logger.Warnf(...); return }`，绝不在失败时退出进程。

---

## 3 与主方案（`docs/SELF_HEAL_AND_LOGGING_PLAN.md` v2）对应关系

### 3.1 方案已覆盖、我复核为成立（A）

| 方案条目 | 我的复核 | 位置 |
|---|---|---|
| §11.3-③ 任务失败只 Emit(name+"-error")、不写日志 | **成立**，且更严重：`core/tasks` 整个包 0 处 logger（`manager.go:56-67/98-109`） | `core/tasks/manager.go` |
| §1.3 日志只写 500 条内存环、无文件 | **成立**（`buffer.go:82,91-125`，四个函数都只 `AppLogs.Add`） | `core/logger/buffer.go` |
| §1.3 通知去重时连日志都不写 | **成立**（`controller.go:1362-1365` 直接 return） | `controller.go` |
| §11.3-① 多个 UI 方法直接跑 GetStatus | **成立**（我确证 4 个：`app.go:667/728/745/757`；另 2 个在 A0 范围） | `app.go` |
| §7-#33/#36 备份/数据目录类按钮同步阻塞 | **成立**，并补了“导出无进度无取消、zip.Close 错误被吞”（F-A5-10） | `app.go:1383-1408` |
| §4.4/§2.1 “任何失败 5 分钟内可复盘”现状不满足 | **成立**：A5 范围 12 个文件 0 条日志（F1） | 见 §1.1 |

### 3.2 方案未覆盖、本次新增

1. **任务框架 goroutine 无 recover**（F-A5-01）——方案 §11.3-③ 只说了“失败不写日志”，没发现 panic 会直接杀进程。
2. **订阅批量更新吞错报成功**（F-A5-02）——方案 §11.6 把订阅列为“只审调用边界”。
3. **提权链路的三处结构性缺陷**（F-A5-03/F-A5-04/F-A5-12）——方案只给提权 50%、C 级，本次读完 `admin_win.go`/`helper_client_windows.go`。
4. **备份/还原的事务边界缺陷**（F-A5-09/F-A5-10）——方案未提备份。
5. **下载机的 4 个具体缺陷**（F-A5-07/F-A5-08/F-A5-15/F-A5-16）——方案 §11.6 自认 downloader 未逐行，本次逐行。
6. **启动期 MigrateLegacyAssets 的额外 6 次哈希**（F-A5-14）——方案 §1.1#16 的启动成本漏项。
7. **死事件/双发事件契约**（F-A5-23）——方案 §10.3 只讨论“注册与否”，没做发射点↔监听对照。
8. **退出链路预算打架**（F-A5-13）与 **os.Exit 跳过清理**（F-A5-03）——方案 §6.4 只关心崩溃前 flush。
9. **BehaviorStore 读-改-写丢更新 / LoadSetting 静默回落默认**（F-A5-18/F-A5-19）——方案把“配置事务边界”列为仅看调用点。

### 3.3 方案（以及 A0 报告）说错的地方

| # | 方案原文 | 实际 | 证据 |
|---|---|---|---|
| 1 | §10.3：“新增事件必须在 `v3_events.go` 注册，否则会被 v3 的类型校验丢弃” | **错。** 未注册事件照常投递，只在 `strictevents` 构建下打一条 warning（`validateCustomEvent` 找不到注册项时 `return nil` 放行）。真正的风险**相反**：注册了但 payload 类型不符 → 事件被 `Cancel()` 并上抛 `handleError` | wails v3.1.0 `pkg/application/events.go:339-344`、`events_strict.go:15-27`、`events.go:171-174` |
| 2 | §11.3-⑤ + §7-#39：“CheckHelperService 的 Ping 挂在 UI 链上，服务假死会让设置页卡 10s” | **部分错。** Ping 有专属预算：连接 150ms（`helper_client_windows.go:20`）+ 方法 200ms（`:29-30`），上限约 350ms；10s 只作用于 `default` 分支（`replace-core-file`/`install-wintun`/`repair-permission`），那才是真问题（F-A5-12）。A0 报告 §2 沿用了同一错误口径 | `core/sys/helper_client_windows.go:26-44` |
| 3 | §1.1 #16：“启动资产维护 = Migrate + EnsureReady(RequireTun) + EnsureReady(RequireGeo) + Cleanup” | 成本**被低估**：`MigrateLegacyAssets` 自己还对 6 个现存资产各算一次 SHA256（≈91MB），`CleanupLegacyAssets` 再算 12×N×2 次；且都在 UI 出现前 | `core/runtimeassets/migrate.go:33-41,114-142`；`app.go:167,190` |
| 4 | 任务描述里的 `core/appcore/migrate.go` | **该文件不存在**。实际对应 `core/runtimeassets/migrate.go`（资产迁移）与 `core/utils/data_migration.go`（数据目录迁移，351 行） | 目录列举 |
| 5 | §11.1 “后台任务框架 100% 覆盖，manager.go 仅 122 行” | 行数对，但**结论过早**：122 行里藏着“panic 杀进程”（F-A5-01）与“无日志”（§11.3-③）两个 P0/P1 级问题 | `core/tasks/manager.go:45-113` |
| 6 | §1.2-C/§5.3 “前端错误靠文案猜语义” | 成立，但还有**更基础的一层**：后端更新类错误根本没有 code 可用（`component_update_service.go:94/145/167` 全是中文自由文本），前端就算想按 code 路由也做不到 | `component_update_service.go`、`updater_orchestrator.go:563-587` |

---

## 4 未覆盖与待实测

| 项 | 状态 | 为什么没做 | 补齐成本 |
|---|---|---|---|
| `core/utils/data_migration.go`（351 行，启动最前置同步迁移） | 未读全（只看了函数清单与 `exec.Command` 位置） | 不在 A5 文件清单内（“migrate”实际指向它），且属数据目录迁移域 | 1 h |
| `cmd/goclashz-helper/main.go` | 未读 | A2 范围；我只从 `helper_protocol.go` 反推预算 | — |
| `core/appcore/control_coordinator.go` 的 4~6 次 EnsureReady | 未读 | A1/A2 范围；我只复核了 backup/component 两条事务里的重复 `EnsureCoreRunning` | — |
| `CoreSupportsMips` 是否复用 `ReadCoreVersion` 的 60s 缓存 | **C 级，待实测** | 只读了 `config.go:242` 的调用，未读 `clash/setup.go:64-129` 实现 | 10 min |
| `Global\` 命名事件在标准用户下能否创建 | **C 级**（F-A5-25 触发条件） | 需提权/非提权各一次实机验证 `CreateEventW` 返回值 | 10 min |
| 10min client timeout 在真实机场的观测分布 | 未测 | A 级只证明“代码允许 10min”，不证明“实际常发生” | 需线下抓包 |
| 杀软对 `core/bin` 的拦截放大倍数 | 未测 | 无法在只读审计里注入 | 需复现 |
| grab 跨 URL 续传是否真会拼出坏文件（F-A5-15） | **B 级** | 依赖 grab 内部 Range 行为（已读 `client.go:255-280`）+ 镜像是否同源 | 1 次注入实验 |
| 备份导出/还原在真实 300MB 数据目录下的耗时 | 未测 | 需实机 | 15 min |
| 前端 28 处 `console.error` 与更新类事件的联动 | 只做了事件级对照 | 属 A4 范围 | — |

---

## 附：建议立刻加入回归用例的 5 个场景

1. 任务函数内 panic（F-A5-01）→ 断言进程存活 + 有 `<task>-error` + 有栈日志。
2. 三个订阅全部 404（F-A5-02）→ 断言 `subs-update-error` 且 UI 不显示成功。
3. 系统代理开启状态下点“以管理员身份重启”（F-A5-03）→ 断言注册表 ProxyEnable 已归零。
4. 导出备份到磁盘配额耗尽的分区（F-A5-10）→ 断言返回错误而不是 SUCCESS。
5. 还原时在 `applyRestorePlan` 中途删除目标目录（F-A5-09）→ 断言回滚失败被如实上报。
