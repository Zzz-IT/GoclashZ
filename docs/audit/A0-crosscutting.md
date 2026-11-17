# A0 跨切面审计（主审自留）

> 生成：2026-11-16；范围：不属于 A1~A5 任一子系统的横向统计与方法论
> 证据等级：A=代码可确证；B=代码+运行时状态推断；C=假设待实测

## 0 方法与口径

- 手段：全仓 `grep` 调用点计数 → 结构体/函数逐段读 → 实机文件/进程/服务对账（`Get-ChildItem`/`Get-FileHash`/`clash.exe -v` 实测耗时）→ 前端调用点统计。
- 统计口径：只统计仓库内源文件（含 `cmd/`、`core/`、`app.go`、`main.go`、`frontend/src/`）；不含 `build/`（构建脚本）与 `node_modules`。
- 实测基准（本机 D 盘 NVMe，warm cache）：`Get-FileHash clash.exe(61 MB) = 0.06 s`；`clash.exe -v = 0.08 s`。冷缓存/杀软首扫的放大倍数**未测**（C 级）。

## 1 全仓“重原语”统计

| 原语 | 次数 | 热点文件（次数） |
|---|---|---|
| `exec.Command` / `CommandContext` | 26 | build/ios(10)、build/android(4)、helper(2)、clash/runner(2)、utils/data_migration(2)、sys/startup(2)、sys/uwp(2)、clash/setup(1) |
| SHA256 | 12 | runtimeassets/check(5)、migrate(4)、helper(1)、sync(1)、downloader/utils(1) |
| `context.WithTimeout` | 23 | app.go(4)、controller(2)、delay_manager(2)、outbound_ip(2)、supervisor(2)、updater(2)、uwp(2) |
| `time.Sleep` | 23 | app.go(3)、helper(3)、helper_client(3)、helper_scm(3)、sys/win(3)、component_replace(2)、controller(2)、runner(2) |
| `go func()` | 45 | geo_update(5)、outbound_ip(5)、app.go(4)、helper(4)、app_update(3)、controller(3)、delay(3) |
| `singleflight` | 5 | controller(3：helper-ready)、outbound_ip(2) |

> 说明：`time.Sleep` 有 9 处在 `core/sys` 的 SCM/提权路径上，属于“固定等待”型阻塞，无法取消、无法观测——改造时统一换成带 ctx 的轮询。

## 2 UI 可达的同步重方法（app.go，共 17 个）

| 方法 | 行 | 重操作 | 前端调用点 |
|---|---|---|---|
| `CheckTunEnv` | 727 | GetStatus + CheckHelperService(Ping≤10s) + CoreSupportsMips(再起一次 -v) | App.vue:641（启动）+ SettingsTun:118 |
| `GetRuntimeAssetStatus` | 897 | GetStatus（全量哈希+ -v） | SettingsTun:109、SettingsUpdate:73 |
| `GetDataDirInfo` | 663 | GetStatus + GetSeedCatalogStatus（对 seed 再跑一次 -v） | SettingsAbout:446 |
| `GetWintunVersion` | 744 | GetStatus | 无（死接口，但仍在 108 方法服务面上） |
| `GetComponentFileInfo` | 756 | GetStatus | 无（死接口） |
| `GetCoreVersion` | 885 | GetStatus | 无（死接口） |
| `RepairRuntimeAssets` | 901 | EnsureReady(RepairForce) → 重写 6 资产 ≈95 MB | **无前端调用方**（A1 独立 grep 0 命中），死接口 |
| `GetHelperServiceStatus` | 617 | CheckHelperService → Ping（默认 10s deadline） | SettingsUpdate:81 |
| `RestartHelperService` | 652 | Stop → sleep 300ms → Start（同步） | SettingsUpdate:88 |
| `ToggleTunMode` | 370 | 25s ctx 全链路 | Overview / SettingsTun / 托盘 |
| `ToggleSystemProxy` | 364 | 15s ctx + WinINet 读写 | Overview |
| `SelectProxy` | 392 | 3s ctx + 内核 API | Proxies |
| `TestProxy` | 1235 | 8s 测速 | Proxies |
| `UninstallHelperService` | 644 | SCM 卸载（阻塞） | SettingsUpdate |
| `runStartupRuntimeAssetMaintenance` | 166 | 2×EnsureReady + 迁移 + 清理（同步，UI 前） | 启动 |
| `ServiceStartup` | 196 | 上述 + Bootstrap + 注册 helper | 启动 |
| `safeQuit` | 1456 | sleep + 清理 | 退出 |

**关键**：其中 6 个是“打开页面就跑全量哈希”的接口；3 个是**前端根本没用**的死接口（仍在服务面/绑定里，未来误用风险）。

## 3 前端重调用点（实测代码）

| 页面 | 代码 | 并发度 | 后果 |
|---|---|---|---|
| 设置→TUN | `Promise.all([refreshRuntimeAssets(), App.CheckTunEnv(), App.GetTunConfig()])`（SettingsTun:118） | **2 个全量 GetStatus 并发** | 打开页面 = 2×（61 MB 哈希 + 起进程）+ 适配器/服务探测 |
| 设置→组件与库更新 | `Promise.all([refreshRuntimeAssets(), refreshHelperStatus()])`（SettingsUpdate:82） | 1 个全量 GetStatus + 1 次 helper Ping | 页面打开可能卡到 Ping 的 10s deadline |
| 设置→关于 | `App.GetDataDirInfo()`（SettingsAbout:446） | 1 个 GetStatus + 1 次 seed 版 -v | 诊断接口本身很贵 |
| 启动 | `App.CheckTunEnv()`（App.vue:641） | 与首屏其他事务并行 | 首屏“状态未知”窗口 |
| 组件更新完成事件 | `driver-install-success` → refreshRuntimeAssets（SettingsTun:187 / SettingsUpdate:99） | 每次装驱动后再来一轮 | 装完驱动卡一下 |

> 结论（修正此前表述）：这些接口**没有定时轮询**（全仓只有 1 处 `setInterval`，在 `trafficWaveState.ts:140`，与资产无关）。所以影响是“**每次进入页面/每次事件后卡一次**”，不是“持续烧 IO”。

## 4 单次用户动作的 IO / 进程预算（估算，B 级）

| 动作 | GetStatus 次数 | 哈希读取量 | 进程 spawn |
|---|---|---|---|
| 开 TUN（正常路径） | 3（prepare+reconcile+ensureCore） | ≈270 MB | 3×`-v` + 1×`-t` + 1×`clash.exe` |
| 开 TUN（触发修复） | 最多 9 | ≈810 MB | 9×`-v` + 1×`-t` + 1×`clash.exe` + 若干 CopyFile |
| 失败 + 回滚的配置切换 | 4~6 | ≈450 MB | 4~6×`-v` + 2×`-t` + 2×`clash.exe` |
| 打开“设置→TUN” | 2 | ≈180 MB | 2×`-v` + 2×(版本 API) |
| 启动（资产维护+首屏） | 3~4 | ≈270 MB | 3~4×`-v` |

> warm 缓存下这些数字都不致命（~0.1s/90MB）；**致命的是 |cold page-in| 与 |杀软首扫| 这两个乘数**，而它们恰好都作用在进程 spawn 与首次读取上。这也是为什么“冷启动第一次”必然最慢、第二次就正常。该乘数未实测（C 级），P0 埋点后即可量化。

## 5 跨切面问题（F-A0-xx）

| 编号 | 问题 | 位置 | 严重度 | 证据 |
|---|---|---|---|---|
| F-A0-01 | UI 接口直接暴露“全量深检”，无缓存层 | `app.go:663,727,744,756,885,897` | P0 | A |
| F-A0-02 | 同一页面并行跑 2 个全量 GetStatus | `SettingsTun.vue:118` | P1 | A |
| F-A0-03 | 6 个重接口中 3 个是死接口，仍在服务面 | `app.go:744,756,885` | P2 | A |
| F-A0-04 | `RepairForce` 全量重写无进度/无取消 | `app.go:901`、`runtimeassets/sync.go:136-246` | P1 | A |
| F-A0-05 | SCM/提权路径用固定 `time.Sleep` 轮询（9 处），不可取消不可观测 | `core/sys/helper_scm_windows.go`、`helper_client_windows.go`、`win.go` | P1 | A |
| F-A0-06 | 单次动作的进程 spawn 数无上限（修复路径 9+） | 同 §4 | P1 | B |
| F-A0-07 | 首屏与设置页共用同一批重接口，没有“轻量快照”通道 | `App.vue:641`、`Settings*` | P0 | A |

## 6 待验证假设（不写进结论）

1. 19:00:33 的 `core\bin` 目录变更来自 `utils.CopyFile` 的 tmp 文件（B 级）→ 需落盘日志复现。
2. 冷启动“卡一会”的主因是进程 spawn + 首扫，而非哈希（C 级）→ 需 `boot.phase`/`asset.probe` 耗时埋点。
3. 火绒首次扫描对 61 MB 内核的放大倍数（C 级）→ 可做一次对照实验（临时加排除项对比）。

## 6.5 修正记录（子审计回灌，v2）

| 项 | 原表述 | 修正后 | 来源 |
|---|---|---|---|
| shouldCopy 位置 | check.go:181-188 | **sync.go:176-207** | A1 |
| 开 TUN 的 EnsureReady 次数 | 3 | **4**（多一处 `clash.Start` → `PrepareEnv` → `setup.go:28`） | A1/A2 |
| RepairRuntimeAssets 调用方 | SettingsUpdate 按钮 | **前端 0 调用（死接口）** | A1/A4 |
| 前端计数 | alert 67 / confirm 18 / toast 7 | **alert 69 调用(66 行) / confirm 17 / toast 6**；另有 9 处零痕迹 catch | A4 |
| helper Ping 成本 | 默认 10s → 可能卡 10s | **10s 只是 deadline 上限；正常 ~200ms**，只有服务假死时才会逼近上限 | A2/A4 |
| 前端误报机理 | 任何含 wintun 字样的错误都会命中缺驱动分支 | **只有大写 `Wintun` 三处与 `wintun_missing` 命中**；小写包装错误走通用模态 | A2/A4 |

> 以上修正已同步到 docs/SELF_HEAL_AND_LOGGING_PLAN.md（v2.1）。后续以合并报告 docs/AUDIT_FULL.md 为准。

## 7 与主方案的关系（修正后）

- §7 清单里的 #16/#17/#20/#31/#33 与本报告的 F-A0-01/04 对应；
- 新增：F-A0-02（并行重调用）、F-A0-03（死接口收敛）、F-A0-05（固定 Sleep 轮询）、F-A0-07（缺轻量快照通道）；
- 修正：主方案曾暗示“哈希是主要成本”，实测 warm 下哈希很便宜，**成本在进程 spawn 与冷首扫**。
