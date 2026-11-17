# A2 审计：TUN / 内核 / helper 链路（分段时间预算 · 错误语义 · 生命周期 · 文件替换）

> 审计人：audit-tun（子代理）｜日期：2026-11-16｜性质：**只读审计**，未修改任何源码/配置，未运行应用、未构建、未 kill 进程
> 审计范围（任务派发）：`core/appcore/{controller,supervisor,control_coordinator,env}.go`、`core/clash/{runner,setup,component_replace,api_client}.go`、`core/sys/{helper_client_windows,helper_scm_windows,tun_win,proxy_win,proxy_guard_win}.go`、`cmd/goclashz-helper/main.go`
> 证据等级：**A** = 代码可确证；**B** = 代码 + 环境/推断，需实测确认；**C** = 假设
> 行号以当前工作树为准（`core/appcore/controller.go` 共 2484 行、`cmd/goclashz-helper/main.go` 共 1126 行）

---

## 0 覆盖范围与方法

### 0.1 读过的文件与覆盖度

| 文件 | 行数 | 覆盖 | 说明 |
|---|---|---|---|
| core/appcore/controller.go | 2484 | 100% | 全文分段通读（含 delay-core、启动恢复、notify、disableAll、EnsureHelperReady、prepareTunEnable） |
| core/appcore/supervisor.go | 251 | 100% | 全文 |
| core/appcore/control_coordinator.go | 407 | 100% | 全文（setLocked / switchConfigLocked / rollbackTransition） |
| core/appcore/env.go | 51 | 100% | 全文 |
| core/clash/runner.go | 355 | 100% | 全文 |
| core/clash/setup.go | 528 | 100% | 全文 |
| core/clash/component_replace.go | 85 | 100% | 全文 |
| core/clash/api_client.go | 577 | 100% | 全文 |
| core/sys/helper_client_windows.go | 394 | 100% | 全文 |
| core/sys/helper_scm_windows.go | 351 | 100% | 全文 |
| core/sys/tun_win.go | 288 | 100% | 全文 |
| core/sys/proxy_win.go | 262 | 100% | 全文 |
| core/sys/proxy_guard_win.go | 129 | 100% | 全文 |
| cmd/goclashz-helper/main.go | 1126 | 100% | 全文（含 console / supervise / handlers / install） |

**为取证而交叉阅读（不计入本任务范围，仅用于确认因果链）**：
`core/sys/helper_protocol.go`（122 行，全读）、`core/runtimeassets/status.go`（全读）、`core/runtimeassets/sync.go:107-254`、`core/runtimeassets/check.go:88-273`、`core/clash/wintun.go`（全读）、`core/utils/paths.go:45-240`、`core/utils/atomic.go:40-99`、`core/appcore/controller_long_connection.go:110-185`、`core/appcore/updater_orchestrator.go:320-390`、`app.go:340-415`、`tray_runtime_windows.go:150-160/490-535`、`frontend/src/store.ts:735-777`、`frontend/src/components/Overview.vue:250-319`。

范围覆盖度：**派发清单 100%**；但"TUN 全链路"必然跨出派发清单（资产自检、前端提示面、托盘、Wails 绑定），跨文件面约 **80%**（详见 §4）。

### 0.2 检索手段

1. 逐文件全文 `read`（不跳读、不抽样）。
2. `grep` 调用点计数：`EnsureReady(` → 7 处（app.go:170/181/902、setup.go:28、controller.go:655/1770、supervisor.go:155）；`ToggleTunMode|ToggleTunFromCurrentDesired|SetTun(` → 6 个入口；`DisableSystemProxy()` → 4 处；`InstallWintun|ReplaceCoreFile|ReplaceFileWithBackup|WaitFileReleased` → 全部调用点；`PendingReplace` → **全仓只命中 docs/**（无实现）。
3. **磁盘实测（只读 stat，不启动程序）**：当前真实运行时资产目录是 `%LOCALAPPDATA%\GoclashZ\core\bin`（clash.exe 61,105,152 B；wintun.dll 427,552 B；4 个 geo 合计 33,805,519 B → 单次全量哈希 **95,336,775 B ≈ 90.9 MiB**）；而 helper 白名单推导目录 `<exeDir>\core\bin` = `D:\GOLANG\GoclashZv3\bin\core\bin` **不存在**（Test-Path=False）。这两条实测直接支撑 F-A2-03。
4. 与 `docs/SELF_HEAL_AND_LOGGING_PLAN.md`（v2）逐条对账，结论见 §3。

### 0.3 方法论限制

- 任务禁止运行应用/构建，**所有耗时数字只能来自代码常量与文件大小推算**，标注为 B；本报告不声称任何运行时实测值。
- SCM 行为、helper 服务是否已安装、托盘 ctx 生命周期等需实机确认，列入 §4。

---

## 1 链路清单表

### 1.1 入口与场景

| # | 入口 / 场景 | file:line | 严格度 | 单次成本 | 是否阻塞用户 | 误报面 | 触发频率 |
|---|---|---|---|---|---|---|---|
| 1 | UI 开/关 TUN（Wails） | `app.go:370-374` | 最严（全链路共享 1 个 25s ctx） | 见 §1.2 预算表 | **是**（同步等待） | 任一层失败 → 前端模态（Overview.vue:310-313） | 每次点开关 |
| 2 | 托盘开/关 TUN | `tray_runtime_windows.go:513`（ctx 来自 :155，**无 deadline**） | 最严且**无预算上限** | 同 #1，适配器可等满 20s | 否（无调用方等待） | 错误经 reportTrayError → notify(source=tray) → 模态（store.ts:752-776） | 每次托盘点击 |
| 3 | `prepareTunEnable` | `controller.go:1760-1787` | 严 | helper 就绪（见 #4）+ `EnsureReady`#1（90.9 MiB 哈希 + ≤2 次 `-v`） | 是 | 只在 `ErrorCode==ErrMissing` 时硬拦（:1777-1779），其余仅 warn | 每次开 TUN（**含幂等点击**，见 F-A2-20） |
| 4 | `EnsureHelperReady` | `controller.go:1696-1750` | 中 | 快 ping 200ms（helper_client:29-30）；冷启 SCM `StartService`≤10s（helper_scm:235）+ 轮询 25×200ms（controller:1717）；管理员修复 ≤10s+10s+10s+9s（helper_client:314-345） | 是（**无 ctx、无超时**，singleflight 不传 ctx） | 非管理员 + 未安装 → 返回 `ErrHelperInstallRequired`/`ErrHelperRepairRequired` → 前端 confirm+安装（Overview.vue:267-287） | 每次开 TUN、内核更新 |
| 5 | `supervisor.Reconcile` | `supervisor.go:142-221` | 严 | `desired.Tun` 时**每 tick 一次** `EnsureReady(RequireTun)`（:154-175）= 90.9 MiB 哈希 | 是（持 `reconcileMu`，:143-144） | EnsureReady 失败 → 仅 `logger.Warnf`（:172），不弹但**也不落盘** | 事件 + 每 5s |
| 6 | watchdog tick | `supervisor.go:114-129` | 中 | 8s ctx（:124）内跑完整 Reconcile + `repairTunActualIfNeeded`（:132-134） | 否（后台） | 8s < 内层 20s → `Reconcile(watchdog) failed` 噪声（:126） | **每 5s**（:72） |
| 7 | 起内核 `ensureCoreRunningWithDesiredState` | `controller.go:626-821` | 最严 | `EnsureReady`#3（:655）+ helper（:670）+ `BuildRuntimeConfig`（:677）+ `EnsureRuntimeAssets`（:682）+ `-t` 3s（:687）+ `clash.Start`（:692）+ API 轮询 100×100ms（:698-750）+ 适配器 ≤20s（:792） | 是 | **任何**修复错误 → `运行组件不可用: %w` 硬失败（:655-661） | 每次起内核/重启 |
| 8 | `clash.Start` → `PrepareEnv` | `runner.go:114` → `setup.go:27-42` | 严 | **第 4 次** `EnsureReady(RequireCoreOnly)` | 是 | `内核不可用: ...` | 每次起内核 |
| 9 | `clash.ValidateConfig`（`-t`） | `runner.go:249-279` | 严 | spawn `clash.exe -t`，3s 上限（:253），**无配置哈希缓存** | 是 | 3s 内没跑完 → `配置预检未通过` 模态（controller.go:687-690） | 每次起内核 |
| 10 | `waitForTunAdapter` | `controller.go:1074-1113` | 中 | 20s / 200ms（:1029-1031）= 最多 100 次全量探测 | 是 | pending 判定依赖 `isTunAdapterPendingErr` 的**文案匹配**（:1009-1023） | 每次开 TUN / 重启 / 启动恢复 |
| 11 | `sys.ProbeTunAdapter` | `tun_win.go:106-151` | 只读 | `GetAdaptersAddresses`（16KB 起，≤4 次重试，:162-204）+ 命中后 `2×GetIpForwardTable2` **整表遍历**（:232-271） | 否 | 描述关键字 `"wintun"`（:74）可命中任意第三方 wintun 网卡；设备名前缀匹配（:217）可能误配 `GOCLASHZ2` 之类 | 200ms 轮询期内 100 次 / watchdog tick 各 1 次 |
| 12 | `stopCoreForTransition` + `clash.Stop` | `controller.go:1188-1223`；`runner.go:193-246` | 中 | helper `stop-core`：优雅 20s（helper_protocol:28）+ 强杀等 5s（:31），客户端 deadline **30s**（:52-54，**忽略外层 ctx**）；之后 3s 停止确认（controller:1198-1222） | 是 | StopCore 失败只 warn 并仍置 `isRunning=false`（runner.go:199-206） | 每次关 TUN / 重启 / 退出 |
| 13 | `disableAll` | `controller.go:1390-1450` | 中 | 停内核（见 #12）+ **无条件** `sys.DisableSystemProxy()`（:1401） | 是 | 误关其它程序写入的系统代理（见 F-A2-07） | 关 TUN、退出 |
| 14 | 系统代理写 `Enable/Disable` | `proxy_win.go:67-171` | 中 | `InternetSetOption` ×2 + RAS 枚举 2 次（:189-228）+ 双广播（:182-187）+ 所有权文件原子写（proxy_guard:38-51） | 是 | 失败 → 模态（controller:949） | 每次开关系统代理 / 内核启停 |
| 15 | 系统代理读 `GetSystemProxyState` | `proxy_win.go:237-262` | 轻 | 2 次注册表读 | 是 | 读失败 → `读取 Windows 系统代理状态失败`（controller:964）→ 事务失败 | 每 tick（Reconcile 末尾 :213） |
| 16 | `restartCoreWithReason` | `controller.go:1810-1866` | 中 | Stop（≤30s）+ 250ms（:1830）+ EnsureReady + Start + 适配器 | 是 | 长连接保护 → `ErrLongConnectionActive`（:1811-1814） | **关 TUN 且系统代理开着**（见 F-A2-05）、配置切换 |
| 17 | 文件替换（主进程路径） | `component_replace.go:54-85` | 严 | `WaitFileReleased` 5s（:58）+ 备份 rename 10×300ms（:67）+ 新文件 rename 10×300ms（:73） | 是 | 目标被映射 → rename 必然失败 | 内核/驱动更新、seed 自愈复制 |
| 18 | 文件替换（helper 路径） | `main.go:816-891`（core）/ `893-945`（wintun） | 严 | Kill 旧内核 ≤5s（:866-871）+ `ReadFile`+`WriteFile` 61 MB（:877-888） | 是 | 白名单目录推导错误 → **必然** `target must be core binary`（:836-839 vs :220-233） | 内核更新、驱动安装 |
| 19 | helper 生命周期（SCM） | `helper_scm_windows.go:108-173,197-251` | 中 | `StartManual`（:118-120），**未配置服务恢复动作**（全仓无 `RecoveryActions`） | 是 | helper 崩溃后无人拉起，直到下次开 TUN | 每次开 TUN（若已停） |
| 20 | helper `watchParent` | `main.go:366-414` | 轻 | 2s 轮询 + 8s 宽限（:368-370） | 否 | 主进程被强杀 → 内核与 helper 一起退出（设计如此） | 常驻 |
| 21 | 启动恢复 `runStartupRestoreJob` | `controller.go:380-444` | 严 | 5 次退避 2/5/10/20/40s（:381）；TUN 单次 ctx = 20s+15s=35s（:396-400） | 否（后台） | 5 次耗尽按错误类型弹（:431-439） | 每次自启 |
| 22 | `EnsureRuntimeAssets`（geo） | `env.go:15-51` 经 `controller.go:682` | 中 | 最多 4 个文件同步下载，单文件 ≤200 MB（env.go:42） | 是（在同一 25s 预算内） | 断网/慢速 → 后面的 `-t` 用过期 ctx 失败 → 模态 | 每次起内核（缺失时） |

### 1.2 开/关 TUN 的分段时间预算（任务重点）

**外层只有 1 个 25s ctx，所有段串行共享**（`app.go:371`），内部还有 7 个互不协调的常量预算：

| 段 | 预算常量 | file:line | 与外层 25s 的关系 |
|---|---|---|---|
| 外层（UI） | **25s** | `app.go:371` | 基准 |
| 外层（托盘） | **无上限** | `tray_runtime_windows.go:155,513` | 不受 25s 约束 |
| helper 快 ping | 0.2s | `helper_client_windows.go:29-30` | 计入 |
| SCM 启动等待 | 10s + 25×200ms≈5s | `helper_scm_windows.go:235`；`controller.go:1717` | 计入 |
| 管理员强修复 | ≤10s(停)+10s(启)+0.5s+10s(重试)+1.2s+10s(重建)+9s(轮询) | `helper_client_windows.go:314-345` | **单独即可超 25s** |
| 资产自检 ×4 | 无上限（90.9 MiB 哈希 + 每轮 ≤2×5s `-v`） | `check.go:128,157`；`status.go:19-42` | 4 轮串行，冷启动主要吃预算者 |
| helper 清理旧内核 | 5s | `main.go:580-583`（`HelperCoreStopKillWait`） | 计入 |
| `start-core` RPC | **10s**（helper 侧 **15s**） | `helper_client_windows.go:22-23`；`main.go:534` | 两侧不等（F-A2-11） |
| API 就绪轮询 | 100×100ms = **10s** | `controller.go:698` | 计入 |
| 适配器等待 | **20s** / 200ms | `controller.go:1029-1031` | 被"剩余预算"截断（:1076-1083） |
| 关 TUN 的 stop-core | 优雅 20s + 强杀 5s，客户端 **30s** | `helper_protocol.go:28,31,52-54` | **超过 25s**（F-A2-05） |
| watchdog 单次 | **8s**（独立于 25s） | `supervisor.go:124` | 与开关互不共享 |
| 异步调和 | **45s** | `supervisor.go:94` | 与开关互不共享 |
| 事务回滚 | **45s**（独立 base ctx） | `control_coordinator.go:381-386` | 与开关互不共享 |

**结论（B，需实测）**：冷启动一次开 TUN 的**代码可确证上限**是"4×资产自检 + SCM 10s+5s + `-t` 3s + start-core 10s + API 10s + 适配器 20s"，合计远超 25s；而**实际耗时分布**取决于 `-v`/`-t` 是否被安全软件拖住（方案 §0.2 已给出火绒扫描窗口的旁证）。这解释了"首次点 TUN 必定弹、重试就正常"：第一击付全量冷启动成本（含 helper 强修复最坏 ≈45s），必然吃穿 25s；第二击全部命中缓存/已就绪，几百 ms 完成。

---

## 2 问题清单

> 排序：P0 → P1 → P2；同级按影响面。

### F-A2-01（P0，A）watchdog 在 TUN 开启期间每 5s 做一次 90.9 MiB 全量资产自检

- **证据**：`supervisor.go:72`（ticker 5s）、`supervisor.go:123-129`（只要 `CoreRunning/SystemProxy/Tun/PortProxy` 任一为真就 Reconcile）、`supervisor.go:154-175`（`desired.Tun` 时无条件 `EnsureReady(RequireTun, RepairInvalid)`）、`status.go:19-42`（GetStatus 必然对 core+4 geo+wintun 全量哈希）、`check.go:128`（core SHA256 无 stat 缓存）、`check.go:302`（geo SHA256 无缓存）。
- **现象**：TUN 一开，后台每 5 秒读+哈希 95,336,775 B（磁盘实测合计），并每 60s 附带一次 `clash.exe -v` 子进程（`check.go:151` 缓存仅 1 分钟）。12 次/分钟 ≈ **1.09 GB/min 的纯读**，全部在 `reconcileMu` 内串行（`supervisor.go:143-144`）。
- **影响**：常驻 IO/CPU 噪声；杀软实时扫描被持续触发（反过来加剧 `-v`/rename 失败概率）；与任何用户动作抢 `reconcileMu`（F-A2-17）。
- **修法**：把 `supervisor.go:154` 的 EnsureReady 换成 L0 快检（stat + 上次结论 TTL），仅在以下三条件之一成立时才深检：①`runtimeState` 变化；②上次深检 >5min；③`reason` 属于用户动作（`controller_long_connection.go:113-119`）。并把 watchdog 的深检移到独立 goroutine，不占 `reconcileMu`。

### F-A2-02（P0，A）单次开 TUN 触发 **4 轮** EnsureReady，25s 预算结构上不够

- **证据**：`controller.go:1770`（#1 prepare）→ `supervisor.go:155`（#2 Reconcile）→ `controller.go:655`（#3 起内核前）→ `runner.go:114` → `setup.go:28`（#4 PrepareEnv）。四处都是 `RepairInvalid`，都可能进入 `RepairFromSeed`（`sync.go:118-134`）并复制 61 MB。
- **现象**：同一次点击内资产自检做 4 遍；若第 2 轮出现瞬时失败，60s 包级冷却（`sync.go:110-120`）会把后续 2 轮也一并打成同一条旧错误。
- **影响**：冷启动必然超预算（见 §1.2）；超时后错误在**最外层**表现为 `配置预检未通过`/`内核启动失败`/`运行组件不可用`，与真实原因（预算不足）无关 → 用户看到的因果是错的。方案 §1.1 #10 说"第三次"，**实际是第 4 次**。
- **修法**：在 `controller.go:1760` 一次 `EnsureReady` 后把 `RuntimeAssetStatus` 通过参数下传到 `supervisor.Reconcile`→`ensureCoreRunningWithDesiredState`→`clash.Start`，删除 `supervisor.go:155`、`controller.go:655`，把 `setup.go:28` 降级为纯 stat 校验；并在 `runtimeassets.EnsureReady` 内部加 (req,mode) 维度的 30s singleflight 缓存。

### F-A2-03（P0，A+B）helper 白名单目录推导错误 → 内核替换 / 驱动安装 / 权限修复在"标准安装"下必然失败

- **证据**：`main.go:220-233`（`getPathWhitelist`：`coreBinDir = filepath.Join(filepath.Dir(exe), "core", "bin")`、`dataDir = <exeDir>/data`）；对照 `core/utils/paths.go:206-208`（主进程 `coreBinDir = <dataDir>/core/bin`）与 `paths.go:99-121`（`dataDir` 优先 `--data-dir`/`GOCLASHZ_DATA_DIR`/`install-profile.json`，**默认 `%LOCALAPPDATA%\GoclashZ`**）。
- **磁盘实测（只读 stat）**：真实资产在 `%LOCALAPPDATA%\GoclashZ\core\bin`（存在，clash.exe 61,105,152 B），而 `D:\GOLANG\GoclashZv3\bin\core\bin` **不存在**。
- **现象**：`handleReplaceCoreFile` 计算 `expectedTarget = <exeDir>/core/bin/clash.exe`（:836），与主进程传来的 `<dataDir>/core/bin/clash.exe` 用 `strings.EqualFold` 比较（:837）→ 永远不等 → 直接返回 `target must be core binary`。`handleInstallWintun`（:912-916）与 `handleRepairPermission`（:800-804）同构、同样必败。服务是 `StartManual` 且 `cmd.Env = os.Environ()` 取服务环境（`main.go:598`），主进程并未传 `GOCLASHZ_DATA_DIR`（`StartCoreParams` 只有 corePath/binDir/runtimeConfig/args，`helper_protocol.go:84-89`）。
- **影响**：`core/clash/setup.go:247-254`、`core/clash/wintun.go:78-82`、`updater_orchestrator.go:353-358` 三条"目标目录不可写 → 走 helper"的兜底全部失效，且错误文案（`通过 Helper 替换内核失败: target must be core binary`）无法指向真实原因。**这是"严格检查→修复失败→误报"链上最硬的一个断点。**
- **修法**：①主进程在 `StartCoreParams`（`helper_protocol.go:84-89`）显式传 `dataDir`；②helper 侧以传入 `dataDir` 推导白名单，并校验其必须是 `%LOCALAPPDATA%\GoclashZ` 或应用目录下的真实路径（防滥用）；③`main.go:836` 的拒绝分支回传实际路径与期望路径，便于归因。

### F-A2-04（P0，A）helper 管道对 `IU` 授予 GENERIC_ALL，且 `start-core` 无路径白名单 → 任意交互用户可让 SYSTEM 服务启动任意 exe

- **证据**：`main.go:1089-1097`（`buildPipeSDDL` = `D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)`，即"任何交互用户"完整控制管道）；`main.go:540-571`（`handleStartCore` 只校验 `filepath.IsAbs` 与 `os.Stat` 存在，**不校验是否为本程序的 clash.exe**）；`main.go:311-360` + `main.go:1023-1055`（服务默认 LocalSystem，`installService` 未设 `ServiceStartName`）。
- **现象**：普通用户进程可写一条 JSON 到 `\\.\pipe\GoclashZ.Helper`，请求 `start-core` 指向任意绝对路径（如 `C:\Windows\System32\cmd.exe`），helper 以 SYSTEM 身份 `exec.Command(...).Start()`（`main.go:596-621`）。
- **影响**：本地提权（低权限 → SYSTEM）。比"误报弹窗"严重得多，且与本次治理主题自相矛盾：主进程侧对 61 MB 内核逐字节哈希，helper 侧却对任意 exe 放行。
- **修法**：①`main.go:1090` 删除 `IU` 授 GA，只保留 `SY`/`BA` 与 AllowedSids 的 `GRGW`，并让 `readAllowedSids`（:1074-1087）支持多值；②`handleStartCore`（:552-560）增加白名单：`corePath` 必须等于 `<dataDir>/core/bin/clash.exe` 且 `binDir` 为其父目录；③启动前对目标做 SHA256 与 `asset-manifest.json` 对账（manifest 由主进程随参数传入）。

### F-A2-05（P1，A）关闭 TUN 的最坏路径超过 UI 的 25s 预算：stop-core 的 30s deadline 完全忽略外层 ctx

- **证据**：`runner.go:198-206`（helper 模式 `Stop()` → `client.StopCore()`）；`helper_client_windows.go:187-197`（`sendRequest` 只设**连接 deadline**，不接受 ctx）；`getMethodTimeout("stop-core") = HelperCoreStopGrace()+KillWait+5s = 30s`（`helper_protocol.go:52-54`）；`main.go:743-759` + `675-723`（优雅 20s → 强杀 → 再等 5s）。
- **附加放大**：当 `SystemProxy` 仍为 true 时，关 TUN 会走 `RestartRequired`（`supervisor.go:245-249` → `controller.go:909-914`）→ `restartCoreWithReason` 里再一次 `stopCoreForTransition`（`controller.go:1823`）+ 250ms（:1830）+ 完整再起（含 20s 适配器等待）。
- **影响**：关 TUN 期间前端 `tunPending` 一直为 true（Overview.vue:253-258），按钮全程无进度；最坏 30s+3s 才返回，且外层 25s 到点后 ctx 已失效，后续任何 ctx 敏感步骤（API 轮询、`waitProxyGroupsReady`）立即以 `context deadline exceeded` 失败 → 在"其实只差一点"的情况下弹出失败模态。
- **修法**：①`sendRequest` 增加 ctx 参数（`helper_client_windows.go:187`），用 `min(methodTimeout, ctxDeadline)` 设 deadline；②`clash.Stop` 增加 `StopWithContext(ctx)`（`runner.go:193`）；③`stop-core` 增加 `graceSeconds` 参数，关闭路径用短预算（建议 6s）、重启路径用 20s（`main.go:752`）；④`app.go:371` 的 25s 只覆盖同步段，关闭动作应两段式（提交 desired 立即返回，落地交给 watchdog 收敛）。

### F-A2-06（P1，A）helper 停止失败仍置 `isRunning=false`，主进程状态与真实内核状态分叉

- **证据**：`runner.go:199-206`——`client.StopCore()` 出错只 `logger.Warnf`，随后无条件 `startedViaHelper.Store(false); isRunning.Store(false); return nil`。
- **现象**：`clash.IsRunning()` 变 false，`runtimeSatisfiesCore`（`controller.go:89-102`）判 false；若随后重启，`controller.go:1819` 的 `wasRunning := clash.IsRunning()` 也为 false → 跳过 250ms 端口缓冲（:1830）→ 新内核 bind 9090/7890 失败。
- **影响**：重启风暴 / "内核启动失败"误报；helper 侧托管的内核变孤儿（仅 `watchParent` 兜底，`main.go:366-414`）。
- **修法**：`runner.go:201-206` 中 StopCore 失败时**不要**清 `isRunning`/`startedViaHelper`，改为返回 error 交给 `stopCoreForTransition`（`controller.go:1188`）统一处置；`restartCoreWithReason` 的 250ms 缓冲改为"确认旧 PID 已退出才 sleep"。

### F-A2-07（P1，A）`disableAll` 无条件关闭系统代理，会误伤其它程序写入的代理配置

- **证据**：`controller.go:1401`（`_ = sys.DisableSystemProxy()`，无任何归属判断）；对照同文件里明确做归属判断的两处：`controller.go:1250-1264`（`captureSystemProxyState` 的 `owned/known` 语义）与 `proxy_guard_win.go:115-129` `ClearOwnedSystemProxy`（读 `system_proxy_state.json` + `currentProxyMatches` 才清）。
- **现象**：只要走到 `disableAll`（关 TUN 且无其它意图、退出、`ShutdownRuntime`），就会执行 `proxy_win.go:134-170`：写 `PROXY_TYPE_DIRECT` + 对全部 RAS 连接写同样的值 + 双广播。**注册表里原本属于别的工具（Clash Verge / v2rayN 等）的代理会被清零。**
- **影响**：静默打断用户的其它代理软件；同一仓库里两条路径语义矛盾。
- **修法**：`controller.go:1401` 改为 `sys.ClearOwnedSystemProxy()`（语义已实现于 `proxy_guard_win.go:115`）；`controller.go:332`（内核异常退出）同样加 owner 校验。

### F-A2-08（P1，A）任何 Reconcile 错误都回滚 desired → 一次瞬时故障撤销用户意图并弹窗

- **证据**：`control_coordinator.go:122-141`——`SetAndSave` 成功后 `Supervisor.Reconcile` 只要返回 error，就 `SetAndSaveIf(desired, previous)` 把 TUN/系统代理意图回滚，并把 err 抛给 UI。
- **现象**：适配器晚到（本可自愈）、helper 唤醒慢、一次 `-t` 抖动 → 意图被撤销 + 前端模态。
- **影响**：与方案 §4.4/§7#23 的目标（"A 类保留 intent 后台重试，仅 C 类回滚"）正好相反；用户必须"再点一次"，这就是"重试正常"体验的代码根因之一。
- **修法**：`control_coordinator.go:128-141` 改为按错误分类决策：新增 `apperr.Classify(err) → {A,B,C}`，仅 B/C 回滚；A 类保留 desired 并 `ReconcileAsync` 后台重试，返回"已受理"（配合 §4.4 的 phase 事件）。

### F-A2-09（P1，A）`EnsureHelperReady` 无 ctx、无超时，最坏路径 ≈45s

- **证据**：`controller.go:1698-1703`（`EnsureHelperReady(reason string)` **不接收 ctx**，`singleflight.Do` 也无法取消）；`controller.go:1716-1720`（`StartHelperService` + `WaitForHelperReady(25, 200ms)`）；`controller.go:1727-1733`（非管理员再调一次 `sys.CheckHelperService()`，又是一次 ping）；`helper_client_windows.go:305-348`（`RecoverHelperServiceForUser`：Stop 10s → 装 → Start 10s → 失败 500ms → 再 Start 10s → delete/1.2s/重建/Start 10s → `WaitForHelperReady(30, 300ms)`≈9s）。
- **影响**：`prepareTunEnable`（`controller.go:1766`）第一段就可能独占整个 25s，且**并发调用者无法中断**（关页/再点都取消不了）。方案 §7#21 只说"singleflight 无超时"，未量化最坏 ≈45s。
- **修法**：`EnsureHelperReady(ctx, reason)` 增加 ctx，把 `singleflight.Group` 换成 `DoChan` + `select{<-ctx.Done()}`；`WaitForHelperReady` 首轮预算降到 800ms 快 ping，返回"正在唤醒"状态而不是阻塞。

### F-A2-10（P1，A）后台自愈失败既不进 UI 也不落盘：`notify(source=appcore)` 被前端整类丢弃

- **证据**：`controller.go:1328-1330`（`setLastError` → `notify("error","RUNTIME_OPERATION_FAILED",...,"appcore")`）、`controller.go:1362-1375`（emit `app-notification`）；`frontend/src/store.ts:752-756`（只有 `source ∈ {tray,startup,deferred-action}` 且 level=error 才 `showAlert`，其余**静默丢弃**）。
- **现象**：`supervisor.Reconcile` 里 `setLastError`（`supervisor.go:165,186,214`）、`controller.go:949`、`controller.go:799` 产生的错误通知**没有 UI 也没有文件日志**（日志仅 500 条内存环，见方案 §1.3）；只有 `logger.Warnf` 进环。方案 §1.3 只指出"去重时连日志都不写"（`controller.go:1363-1365` 确实 return），忽略了更大的洞：**source=appcore 的通知本来就不会被展示**。
- **影响**：watchdog 自愈失败对用户完全不可见，也留不下证据——与"为什么判成 missing 查不下去"是同一类病。
- **修法**：①`store.ts:752` 去掉 source 白名单，按 `notification.code` 的 class 路由（方案 §5.3 路线）；②`notify`（`controller.go:1362`）在 `shouldSuppressNotify` 命中时补一条 `suppressed:true` 的 warn 日志；③`setLastError` 统一带真实 `code`（现在硬编码 `RUNTIME_OPERATION_FAILED`，把 code 丢了）。

### F-A2-11（P1，A+B）`start-core` 两侧预算不等（客户端 10s / helper 15s），且 helper 先等旧内核 5s 才启动新内核

- **证据**：`helper_client_windows.go:22-23`（`startCoreRPCTimeout = 10s`）vs `main.go:527-537`（`handlerTimeout("start-core") = 15s`）；`main.go:578-584`（`startCoreProcess` 先 `terminateCore(old, HelperCoreStopKillWait=5s)` 再 `cmd.Start()`）；`helper_client_windows.go:195`（客户端只设 10s 连接 deadline）。
- **现象**：最坏 5s（清理）+ 启动（冷启动 1~3s，杀软扫描下更久）→ 逼近或超过 10s → 客户端返回 `helper start-core failed: ... timeout`，而 `startCoreViaHelper`（`runner.go:136-138`）直接返回错误、**不置 `isRunning`**；此时 helper 侧内核可能已经起来。
- **影响**：主进程认为启动失败（→ 回滚 desired → 弹窗），实际内核在跑；随后 watchdog 因 desired 已回滚为 false 而调 `disableAll` 杀内核。用户看到"失败 → 又开 → 又关"的抖动。
- **修法**：①`helper_client_windows.go:22` 提到 15s 与 helper 齐平；②`runner.go:128-145` 收到错误后用 `core-status` 复核一次（500ms 预算），若 running 则视为成功并补 `isRunning.Store(true)`；③`main.go:580-584` 把"清理旧内核"从 `start-core` 拆出，避免与启动共用预算。

### F-A2-12（P1，A）`install-wintun` 不停内核，而 `replace-core-file` 停内核；TUN 运行中装驱动必然失败且错误无根因

- **证据**：`main.go:866-871`（replace-core-file 先 `terminateCore`）；对照 `main.go:893-945`（install-wintun **完全没有** stop 逻辑，直接 `os.Rename`+`os.WriteFile`）；上游 `core/clash/wintun.go:78-91`。同时 `runtimeassets/sync.go:181-185` 让 wintun 在 `RepairInvalid` 下**恒** `shouldCopy=true`（`checkWintunByPath` 从不置 `VersionProbeOK`，`check.go:230-273`；对照 `check.go:140` 只有 core 置位）。
- **现象**：TUN 开着 → `sync.go:211` `utils.CopyFile` 写 `wintun.dll.tmp` 成功、`os.Rename` 覆盖被映射的 DLL 失败（`core/utils/atomic.go:93-96` 删 tmp 并报 `replace ... failed: Access is denied`）→ 与方案 §0.2 的"tmp 出现又消失、CreationTime 从未更新"**完全吻合**（B：需一次复现确认时间戳）。helper 路径下则表现为无提示的 `write target failed`（:938-941）。
- **影响**：每次进入修复分支都必然尝试替换一次 → 每次"出事"必然多一条假故障 → 升级为 `运行组件不可用`（F-A2-02）。
- **修法**：①`sync.go:181-185` 对 `AssetWintun` 改为 `shouldCopy = !runtimeHealth.Ready`（与 core 分开判断）；②`main.go:893` 增加"若 `core-status` 报 running 则先 `stopCoreGraceful`"，或改为 `PendingReplace` 队列（方案 §4.2，当前**零实现**）；③`atomic.go:93-96` 区分 `ERROR_ACCESS_DENIED` 与其它错误，前者返回 `ASSET_WINTUN_BUSY` 语义。

### F-A2-13（P1，A）文件替换非原子、错误被吞：helper 用 ReadFile+WriteFile 覆盖，主进程用 rename 覆盖

- **证据**：`main.go:874-888`（`_ = os.Remove(target+".bak")`、`_ = os.Rename(target, target+".bak")` 两处错误被丢弃；随后 `os.ReadFile`+`os.WriteFile(target, input, 0755)`）；`main.go:929-942` 同构；`component_replace.go:38-51`（`retryRename` 10×300ms）与 `atomic.go:93-96`（`os.Rename` 覆盖）。
- **现象**：①被映射时 rename 失败但错误被吞，最终报后续 `write target failed`，掩盖真实原因；②`os.WriteFile` 非原子，进程崩溃/断电会留下**截断的 61 MB clash.exe**；③主进程路径 `os.Rename(tmp, dst)` 覆盖在 Windows 上要求 `FILE_SHARE_DELETE`，被映射/被杀软打开时必失败。
- **影响**：失败归因困难 + 极小概率的"内核文件损坏"（下次启动要重拷 61 MB，又踩 F-A2-02）。
- **修法**：①helper 侧改为"写 `target.new` → `os.Rename` 覆盖目标"，并在 `main.go:874-875` 保留并回传 rename 错误；②`atomic.go:93` 失败时按 `ERROR_ACCESS_DENIED`/`ERROR_SHARING_VIOLATION` 分类上抛；③`component_replace.go:38` 的 `retryRename` 在最终失败时保留 backup 并把 `backupPath` 带进返回值。

### F-A2-14（P1，A）`EnsureRuntimeAssets` 在用户同步路径上串行下载（最多 4×200 MB）

- **证据**：`controller.go:682-684`（起内核前调用，**错误只 warn 不拦**）、`env.go:28-48`（缺失即同步下载，`MaxBytes: 200<<20`，`AttemptsPerEndpoint: 3`）、`env.go:38`（用同一个 25s ctx）。
- **现象**：首装 / geo 被删 / 换数据目录时，开 TUN 在这段卡住；25s 到点 ctx 失效 → 紧接着 `ValidateConfig`（`controller.go:687`）以 `context deadline exceeded` 失败 → `配置预检未通过` 模态。
- **影响**：把"网络慢"翻译成"配置错误"，最典型的误报形态之一。
- **修法**：`controller.go:682` 改为 `go c.ensureRuntimeAssetsAsync(...)`（后台任务 + 进度事件），并在 `controller.go:687` 前加 `ctx.Err() == nil` 快判；`env.go:38` 用独立 ctx（如 `context.WithTimeout(c.ctx, 10*time.Minute)`）。

### F-A2-15（P1，A）`GetAppState` 每次同步都取内核版本；内核更新期间该锁被下载全程持有

- **证据**：`controller.go:572`（`Version: clash.GetLocalCoreVersion(c.ctx)`，在 `GetAppState` 内，而 `SyncState`（:603-605）每 tick/每事件都调）；`setup.go:96-101`（`GetLocalCoreVersion` 持 `coreBinaryMu`）；`setup.go:182-184`（`PrepareCoreUpdate` 持 `coreBinaryMu` **贯穿整个 61 MB 下载**）、`setup.go:236-238`（`CommitCoreUpdate` 持锁做替换）。
- **影响**：内核更新期间所有 `SyncState → GetAppState`（含 5s watchdog、托盘刷新、前端状态同步）全部阻塞在 `coreBinaryMu` 上；`onStateChange`（`controller.go:607-609`）一起卡住，前端表现为"更新时界面状态冻结"。
- **修法**：①`controller.go:572` 改用无锁的 `clash.CachedCoreVersion()`（只读原子变量，由后台刷新）；②`setup.go:183` 把下载移出 `coreBinaryMu`，只在 `CommitCoreUpdate`（:236）内持锁。

### F-A2-16（P1，A）托盘 TUN 开关无 deadline，与 UI 的 25s 语义分裂

- **证据**：`tray_runtime_windows.go:155`（`t.ctx, t.cancel = context.WithCancel(ctx)`，无超时）、`:513`（`ToggleTunFromCurrentDesired(t.ctx)`）；对照 `app.go:371`（25s）。
- **影响**：同一操作两条超时语义；托盘触发时 `waitForTunAdapter` 吃满 20s、`clash.Stop` 可到 30s，期间 `cc.mu`（`control_coordinator.go:70-71`）被独占，UI 的并发点击只能排队。方案 §1.1 只统计了 UI 入口，未列托盘入口。
- **修法**：托盘入口包一层 `context.WithTimeout(t.ctx, 25*time.Second)`，或在 `ControlCoordinator.set`（`control_coordinator.go:69`）内统一兜底注入预算。

### F-A2-17（P2，B）watchdog 的 8s ctx 与 `reconcileMu` 串行，开关事务会让 watchdog 拿到已过期 ctx

- **证据**：`supervisor.go:124`（8s）、`:143-144`（`reconcileMu` 串行）、`control_coordinator.go:128`（开关路径在 25s ctx 内跑完整 Reconcile，且**不置** `transitioning`——只有 `RestartCore`/`SwitchConfig` 会置，`:205-206,233-234`）。
- **现象**：TUN 开关执行 20s 时，watchdog 每 5s 起一个 Reconcile，排队 4 次后逐个以过期 ctx 运行 → `EnsureReady` 立即失败（仅 warn），日志出现周期性 `Reconcile(watchdog) failed`。
- **影响**：日志噪声 + 无效唤醒；这段时间 watchdog 的收敛语义失效。
- **修法**：`supervisor.go:117` 的 `Transitioning()` 判定扩到 `ControlCoordinator.set`（在 `setLocked` 首行 `cc.transitioning.Store(true)`，defer 清），并把 `reconcileMu` 换成带超时的信号量。

### F-A2-18（P2，A）`ErrLongConnectionActive` 的延迟队列每次失败都新起一个 10s ticker goroutine

- **证据**：`controller.go:1811-1814`（`guardDisruptiveAction` 失败 → `queuePendingDisruptiveAction`）、`controller_long_connection.go:154-184`（每次调用起一个 `time.NewTicker(10*time.Second)` goroutine，最长 30 分钟）。
- **现象**：watchdog（reason=`"watchdog"`，不在 `userInitiatedReasons`，`controller_long_connection.go:113-119`）每 5s 触发一次重启意图 → 每次失败泄漏一个 goroutine；30 分钟内最多 ~360 个，且长连接消失时各自调用一次 `RestartCoreWithReason`（:178）。
- **影响**：goroutine 泄漏 + 长连接结束后**重复重启**（用户看到内核反复重启）。
- **修法**：`queuePendingDisruptiveAction`（`controller_long_connection.go:154`）加单例守卫（`atomic.Bool` 或 `sync.Once`），已存在待执行动作时直接 return。

### F-A2-19（P2，B）`ReconcileAsync` 依赖 `s.ctx`，在 `Start` 前 / `Stop` 后可能为 nil 或已取消

- **证据**：`supervisor.go:93-103`（`context.WithTimeout(s.ctx, 45*time.Second)`，`s.ctx` 由 `Start` 在 :71 赋值）；`supervisor.go:66-88`（`Stop` 只清 `cancel` 不清 `ctx`；`Start` 前 `ctx` 为 nil）；`controller.go:312-345`（`SetOnExitCallback` 在 :312 注册，`Supervisor.Start` 在 :345——中间 33 行内若内核退出即触发回调）。
- **影响**：`context.WithTimeout(nil, ...)` 会 panic；`Stop` 之后 `ReconcileAsync` 生成的 ctx 立即失效且 `loop` 已退出（:108-109），请求被丢弃。
- **修法**：`supervisor.go:94` 前加 `if s.ctx == nil { return }`；`Stop` 中把 `s.ctx` 置回 `context.Background()` 或加 `stopped` 标志。

### F-A2-20（P2，A）幂等点击仍付全价：`prepareTunEnable` 在"意图未变"判定之前执行

- **证据**：`control_coordinator.go:76-80`（`tun && enable` 即 `prepareTunEnable`）vs `:116-121`（`desired == previous` 才走轻量 Reconcile）。`prepareTunEnable` 内含 helper 就绪 + `EnsureReady`（F-A2-02 的 #1）。
- **现象**：TUN 已开时再点一次"开"，仍跑一遍 helper ping + 90.9 MiB 哈希。
- **影响**：无谓 IO；若期间发生一次瞬时失败，UI 会重呈现模态。
- **修法**：`control_coordinator.go:76` 的判定移到 `:116` 之后（先算 `desired == previous`，幂等路径只做 L0 快检）。

### F-A2-21（P2，A）helper 服务未配置恢复动作，崩溃后只能靠下次开 TUN 拉起

- **证据**：`helper_scm_windows.go:115-120`（`CreateService` 的 `mgr.Config` 只设 DisplayName/Description/StartType/ErrorControl）；`:145-170`（`UpdateConfig` 同样只比这几个字段）；全仓 `grep RecoveryActions` 无命中。
- **影响**：helper 崩溃 → 当前 TUN 内核失管（`watchParent` 在主进程存活时不会退出）→ 直到用户下次开 TUN 才恢复。
- **修法**：`installOrUpdateServiceSCM`（`helper_scm_windows.go:108`）补 `cfg.RecoveryActions`（第一次/第二次 restart 5s，后续 restart 60s）+ `ResetPeriod`。

### F-A2-22（P2，A）适配器探测的成本结构不合理：每次探测都做 2 次整表路由遍历

- **证据**：`tun_win.go:106-151`（每次探测先 `listAdapters`）、`:162-204`（`GetAdaptersAddresses`，16KB 起、最多 4 次重试）、`:149`（仅成功时）→ `:232-271`（`GetIpForwardTable2` ×2 遍历**全部**路由并逐行比较）；调用点 `controller.go:1088`（200ms 轮询，20s 内最多 100 次）、`controller.go:1159`（watchdog tick）。
- **影响**：以典型 1000~3000 条路由计，200ms 轮询期等于每秒 5 次全表遍历；而 `probeTunRoutes` 的结果只在 `repairTunActualIfNeeded` 的日志里用（`controller.go:1170`），**判定 Up 根本不需要路由表**。
- **修法**：`tun_win.go:149` 把 `probeTunRoutes` 改为按需（只在生成诊断快照/首次成功时调用），轮询期只返回不含 `Routes` 的 `TunAdapterInfo`；`controller.go:1088` 的轮询间隔从 200ms 提到 500ms（20s 内 40 次）。

---

## 3 与主方案（docs/SELF_HEAL_AND_LOGGING_PLAN.md v2）的对应关系

### 3.1 已覆盖且经本审计确认（无需改方案）

| 方案条目 | 本审计确认点 |
|---|---|
| §1.1 #1/#2/#3 资产自检重 | 确认：`status.go:19-42` 恒算 6 个资产哈希；`check.go:128` 无 stat 缓存；`check.go:157` 的 `-v` 5s，缓存仅 1 分钟（`check.go:151`） |
| §1.1 #5/#6 EnsureReady 7 处调用点 + 60s 包级冷却 | 确认：grep 得 7 处；`sync.go:110-134` 确为包级变量 |
| §1.1 #9 watchdog 每 5s 全量 | 确认：`supervisor.go:72,123-129,154-175` |
| §1.1 #11 EnsureHelperReady 25×200ms / 非管理员引导 | 确认：`controller.go:1716-1733` |
| §1.1 #12 20s/200ms 适配器等待 | 确认：`controller.go:1029-1031,1074-1113` |
| §1.1 #13 `-t` 每次起内核都跑、3s | 确认：`runner.go:253`，且无 configHash 缓存 |
| §1.1 #19 geo 同步下载 | 确认：`env.go:38-48` 经 `controller.go:682` |
| §1.2-A wintun `VersionProbeOK` 恒 false | 确认：`sync.go:181-185` + `check.go:230-273`（`checkWintunByPath` 无 `VersionProbeOK` 赋值），唯一置位处 `check.go:140` |
| §1.3 helper 完全黑盒 | 确认：`cmd/goclashz-helper/main.go` 全文件无日志落盘，只有 `fmt.Fprintf(os.Stderr, ...)`（GUI 子系统下不可见，如 `:699,711,956`） |
| §7 #7/#14/#21/#22/#23 的改法方向 | 确认问题存在（见 F-A2-02/07/08/09/05），但 #23 的"仅 C 类回滚"仍是**未实现**状态（`control_coordinator.go:128-141`） |

### 3.2 本审计新增（方案未提或低估）

| 新增项 | 严重度 | 说明 |
|---|---|---|
| **F-A2-03 helper 白名单目录与真实 dataDir 不一致** | P0 | 方案 §11.6 把 `superviseCore/startCoreProcess` 标为"未读"；实际这里藏着"helper 替换路径永久失败"的硬伤，且是唯一的越权恢复通道 |
| **F-A2-04 helper 管道对 IU 授 GA + start-core 无白名单** | P0 | 方案完全未涉及安全面；与"严格检查"主题冲突 |
| **F-A2-05 关闭 TUN 的 30s stop-core 预算** | P1 | 方案只讨论"开 TUN 25s"（§1.1 #19），**开/关不对称**这一点没写 |
| **F-A2-06 Stop 失败仍置 false** | P1 | 方案 §1.2-B 只讲检查侧，未讲停止确认侧的假成功 |
| **F-A2-10 source=appcore 的通知被前端整类丢弃** | P1 | 方案 §1.4 引用了 `store.ts:749-777` 的 source 白名单，但没得出"后台自愈失败对用户不可见"的结论 |
| **F-A2-11 start-core 两侧预算不等** | P1 | 方案未提 |
| **F-A2-15 PrepareCoreUpdate 持锁贯穿下载** | P1 | 方案 §11.3 只提"按钮假死"，未定位到 `coreBinaryMu` 与 `GetAppState` 的耦合 |
| **F-A2-16 托盘无 deadline** | P1 | 方案 §11.2 统计了入口，但没给托盘入口的预算差异 |
| **F-A2-18 延迟动作 goroutine 泄漏** | P2 | 方案未提 |
| **§1.2 "45s" 双源** | — | `supervisor.go:94`（异步调和）与 `control_coordinator.go:385`（回滚）是**两个独立的 45s**，方案 §4.4 只写了一个 |

### 3.3 方案说错 / 说过头的地方（独立验证结论）

| # | 方案原文 | 本审计结论 | 证据 |
|---|---|---|---|
| 1 | §11.3-⑤「RPC 默认 deadline **10s**，`CheckHelperService` 的 Ping 就挂在 UI 链上…服务假死会让设置页卡 10s」 | **错**。`ping` 有独立 200ms 预算（`helper_client_windows.go:29-30`）；10s 只对未列举的方法（`shutdown/repair-permission/replace-core-file/install-wintun`，:41-42）生效。真正让 UI 卡住的是 `ensureHelperReadySlow` 的 SCM 启动 + 25 次轮询（`controller.go:1716-1720`，≈5~15s）与 `RecoverHelperServiceForUser`（≈45s 最坏）。**方向对、数字与归因错。** | A |
| 2 | §1.1 #10「`ensureCoreRunningWithDesiredState` = **第三次** EnsureReady」 | **少算一次**。实际 4 次：`controller.go:1770` → `supervisor.go:155` → `controller.go:655` → `runner.go:114`→`setup.go:28`。方案 §11.2 的"7 个调用点"计数正确，但 §0.1 的"2~3 轮"与 §1.1#10 的"第三次"互相矛盾。 | A |
| 3 | §1.2-C「`msg.includes('Wintun')` … **任何一句带 wintun 字样**的错误都被翻译成"缺少驱动"并弹模态」 | **说过头**。`includes` 区分大小写；上游绝大多数文案是小写 `wintun`（`status.go:69` `wintun 仍不可用`、`check.go:236` `wintun.dll 不存在`），**不会**命中该分支，而是落到通用模态分支（`Overview.vue:310-313`）。能命中的只有大写 W 的三处：`controller.go:1778`、`supervisor.go:165`、哨兵 `ErrWintunMissing`=`"wintun_missing"`（`supervisor.go:21`）。→ **真正的病不是"错翻译成缺驱动"，而是"一切错误都弹模态，只是文案不同"。** 这意味着若只按方案 §7#5 删 `includes('Wintun')`，**收益远小于方案预期**，必须同时改通用分支。 | A |
| 4 | §0.1「TUN 只有 25s」+ §4.4 两段式 | 数字正确（`app.go:371`），但**漏了关闭方向**：关 TUN 若系统代理仍开，会走 `RestartRequired`（`supervisor.go:245-249`）→ 完整 stop(≤30s)+start；若不需内核则走 `disableAll`。25s 对关闭路径同样不够，且 stop-core 完全不受 ctx 约束。 | A |
| 5 | §1.1 #7「wintun 永远 `shouldCopy=true`（§1.2-A）→ **每次 EnsureReady 都重拷**」 | 与方案自己的 §11.4-1 修正**自相矛盾**。准确表述：只有 `requirementSatisfied` 为 false 进入修复分支时，wintun 才恒 `shouldCopy=true`（`sync.go:176-188`）。v2 文档里 v1 表述未清理，读者会高估触发频率。应以 §11.4 为准。 | A |
| 6 | §4.2「不在运行中替换…转 `PendingReplace` 队列」被列为"关键规则（现有设计约束）" | 全仓 `grep PendingReplace` **只命中 docs/**，**零实现**。这是纯新增能力（约 1~2 天），不是"补一条规则"；§5.2 的 `ASSET_WINTUN_BUSY` 同样无代码。 | A |
| 7 | §11.6「helper `superviseCore`/`startCoreProcess` 未读，30 min」 | 已读（`main.go:203-217, 578-641`）。设计本身合理（唯一 Wait 调用者 + 不持锁等待），但有三处缺陷：`start-core` 预算错配（F-A2-11）、`install-wintun` 不停内核（F-A2-12）、白名单目录错误（F-A2-03）。方案把它列为"未读"是诚实的，但这块恰恰藏着最硬的 P0。 | A |
| 8 | §5.2 错误码表 `HELPER_WAKE_TIMEOUT` = A 类（无弹窗） | 现状**没有任何"唤醒中"状态可返回**：`EnsureHelperReady` 同步阻塞且无 ctx（`controller.go:1698`），超时只能表现为整体失败 → 一定弹窗。要实现 A 类必须先改函数签名（加 ctx + 返回 phase），只加错误码不够。 | A |

---

## 4 未覆盖与待实测

### 4.1 本审计未覆盖（诚实清单）

| 项 | 状态 | 影响 |
|---|---|---|
| `core/appcore/diagnostics.go`、`backup_service.go`、`subscriptions.go` 全文 | 未读（只读了 `backup_service.go:47` 一处 `DisableSystemProxy`） | F-A2-07 的"误关代理"另有 1 条路径未验证 |
| `core/appcore/updater_orchestrator.go` 全文 | 只读 320-390 | 组件更新与 helper 的交互面未全审 |
| `core/runtimeassets/migrate.go` 全文 | 未读 | 启动迁移对 `core/bin` 的写操作可能触发同款 rename 失败 |
| Wails v3 绑定层错误序列化（`services_v3_generated.go` → 前端 `err.message`） | 未验证 | 前端 `includes('helper_install_required')`（Overview.vue:267）依赖 `error.Error()` 原样透传，需实测 |
| UAC 提权路径（`--repair-permissions` 等入口） | 未审 | 与 F-A2-03/F-A2-04 的权限面互补 |
| helper `install`/`uninstall` 子命令与 UAC 的交互（`main.go:1023-1126`） | 只读未验证 | 服务重建（`helper_client_windows.go:327-339`）依赖它 |

### 4.2 待实测（必须跑应用才能定论，本任务禁止运行）

| # | 待测项 | 方法与预期 | 关联 |
|---|---|---|---|
| 1 | 冷启动一次开 TUN 的**分段耗时分布** | 在 `prepareTunEnable`/`Reconcile`/`EnsureHelperReady`/`waitForTunAdapter` 四处加 `boot.phase` 式埋点（方案 §6.3），取 P50/P95 | F-A2-02、§1.2 |
| 2 | `-v` 在杀软扫描下的超时率 | 采样 `ReadCoreVersion` 的 `err` 与耗时；对照 `check.go:151` 的 1 分钟缓存 | F-A2-02 |
| 3 | `stop-core` 优雅停止的真实耗时分布 | helper 侧在 `stopCoreGraceful` 记录 `grace_used`/`kill_used` | F-A2-05 |
| 4 | `ProbeTunAdapter` 单次 P50/P95（含路由表规模） | 在 `tun_win.go:149` 前后计时，并记录 `len(table.Rows())` | F-A2-22 |
| 5 | helper 白名单是否真的不匹配 | 实机读 `HKLM\SYSTEM\CurrentControlSet\Services\GoclashZHelper\ImagePath`，与 `utils.GetCoreBinDir()` 对比；再手动触发一次内核替换看错误文案 | F-A2-03 |
| 6 | 托盘 ctx 是否在应用退出时立即取消 | 观察 `tray_runtime_windows.go:155` 的父 ctx 生命周期 | F-A2-16 |
| 7 | 25s 到点时前端实际展示的文案 | 注入慢磁盘/慢 AV（进程级延迟），记录 `Overview.vue:310-313` 的 `toUserError` 输出 | F-A2-02、§3.3-3 |
| 8 | watchdog 常驻期实际磁盘读带宽 | PerfMon/ETW 统计进程读字节，验证 ≈1.09 GB/min 的推算 | F-A2-01 |

### 4.3 一句话结论

> 这条链路的问题不在"检查太严"，而在**四轮重复检查共享一个 25s 预算**、**关闭方向有 30s 的不可中断黑洞**、**helper 的越权恢复通道因路径推导错误永久失效**、以及**后台失败既不上 UI 也不落盘**。其中 `main.go:220-233` 与 `paths.go:206-208` 的目录不一致，是本次审计发现的最高性价比修复点（改约 10 行即可恢复整条 helper 替换链路）。