# GoclashZ 全链路审计总报告（v1，2026-11-16）

> 编制：主审 lead；执行：5 个并行审计代理（A1~A5）+ 主审跨切面（A0）
> 分报告：`docs/audit/A0-crosscutting.md`、`A1-assets.md`、`A2-tun-core-helper.md`、`A3-logging.md`、`A4-frontend.md`、`A5-peripheral.md`
> 关联方案：`docs/SELF_HEAL_AND_LOGGING_PLAN.md`（v2.1，含勘误）
> 证据等级：A=代码可确证；B=代码+运行时状态推断；C=假设待实测

---

## 0 结论（三句话）

1. **问题不是"检查太严"，而是"检查没有分层、错误没有语义、日志没有落盘、跨进程契约已经破损"。** 118 条问题（P0 25 / P1 55 / P2 38）可收敛为 5 个结构性根因（§4）。
2. **发现 2 个与体验无关、但必须先修的 P0**：helper 命名管道对交互用户授 `GENERIC_ALL` 且 `start-core` 无路径白名单（本地提权）；helper 白名单目录与真实 dataDir 不一致（内核替换/装驱动/权限修复**永久失败**）。
3. **用户感受到的"第一次必弹、第二次就好"，是 4 层叠加**：冷启动 4 轮全量自检挤在 25s 预算内 → 资产修复必然失败（wintun 被映射） → 60s 冷却把失败固化 → 前端单一模态槽 + 文案匹配把失败表达成"缺驱动"。任何一层单独修都不够。

---

## 1 方法与覆盖面

| 报告 | 范围 | 读了多少 | 问题数 | 覆盖度 |
|---|---|---|---|---|
| A0（主审） | 跨切面：重原语统计、UI 可达重方法、前端重调用点、预算估算 | 全仓 grep + app.go/main.go 结构 + 实机对账 | 7（P0 2） | 横向 100% |
| A1 | 资产自检与自愈（runtimeassets 全量 + 调用方） | 1178 行 + utils 438 行 + 调用方 | 20（P0 4） | 92% |
| A2 | TUN/内核/helper（含 helper 1126 行、controller 2484 行） | 14 个文件全文 + 交叉取证 | 22（P0 4） | 90% |
| A3 | 日志与可观测（logger/dispatcher/sink/diagnostics/main） | 全读 + Wails v3 依赖源码 + 实机核对 | 21（P0 6） | 100% |
| A4 | 前端提示面与错误处理 | 全部 .vue/.ts + 8 语言 locale | 23（P0 6） | 95% |
| A5 | 外围（更新/订阅/备份/配置/提权/托盘/热键/下载器/tasks） | 27 文件 ≈7000 行 + 依赖源码 | 25（P0 3） | 88% |
| **合计** | | **≈11000 行源码 + 实机核对** | **118** | — |

未覆盖（三项，均为低风险）：`sys/uwp_win.go` 的 COM 细节、`core/backup/archive.go` 的 zip 语义、Wails 框架内部实现（只按外部行为取证）。

---

## 2 问题总览

| 严重度 | 数量 | 分布（A0/A1/A2/A3/A4/A5） |
|---|---|---|
| P0 | **25** | 2 / 4 / 4 / 6 / 6 / 3 |
| P1 | **55** | 4 / 13 / 12 / 8 / 8 / 10 |
| P2 | **38** | 1 / 3 / 6 / 7 / 9 / 12 |

---

## 3 P0 清单（去重后 16 组）

### 3.1 安全（必须先修，与体验无关）

| # | 问题 | 位置 | 证据 | 来源 |
|---|---|---|---|---|
| S1 | helper 管道 SDDL 对 **Interactive Users 授 GENERIC_ALL**，而 `start-core` 只校验"绝对路径 + 文件存在"，不校验目录 → 任意登录用户可让 SYSTEM 服务启动任意 exe = **本地提权** | `cmd/goclashz-helper/main.go:1090`、`:540-571` | A | A2-04 |
| S2 | helper 白名单目录 = `<appDir>/core/bin` 与 `<appDir>/data`；真实 dataDir 由 `install-profile.json` 决定（实测 `%LOCALAPPDATA%\GoclashZ` 与 `D:\GoclashZ\data` 两种布局）→ 内核替换 / 装驱动 / 权限修复**永久失败** | `main.go:220-233`、`:827-840`、`:903-908` | A/B | A2-03 |
| S3 | 任务框架 goroutine 无 `recover`：更新/下载任务内任何 panic 直接杀进程，无栈无日志 | `core/tasks/manager.go:45-68, 88-110` | A | A5-01 |
| S4 | 提权重启在库函数里 `os.Exit(0)`：跳过 `DisableAll` 与系统代理清理 | `core/sys/admin_win.go:69, 95` | A | A5-03 |

### 3.2 可用性（用户直接可感）

| # | 问题 | 位置 | 证据 | 来源 |
|---|---|---|---|---|
| U1 | 一次开 TUN 触发 **4 轮** `EnsureReady`（prepare / reconcile / ensureCore / `clash.Start→PrepareEnv`）共享 25s ctx，每轮含 95 MB 哈希 + `-v` → 冷启动**结构上必然超时** | `controller.go:1770/655`、`supervisor.go:155`、`setup.go:28`、`runner.go:114` | A | A1-10 / A2-02 |
| U2 | wintun 在 `RepairInvalid` 下恒 `shouldCopy=true`，`CopyFile` 无 `WaitFileReleased`/重试 → TUN 运行中 rename 必失败 → 60s 冷却 → `controller.go:655` 升级为硬失败 | `sync.go:176-207`、`atomic.go:59-99`、`sync.go:110-134` | A | A1-02 |
| U3 | 自愈并发失控：冷却判断非原子、无单飞、`CopyFile` 固定 `.tmp` 名 → 两个 goroutine 可互踩出**半截文件** | `sync.go:118-134`、`atomic.go:70` | A | A1-03 |
| U4 | 前端**单一模态槽**：`showAlert` / 直写 modal 会顶掉 pending 的 `showConfirm` → Promise 永不 settle → `tunPending` 恒 true → **TUN/系统代理开关永久点不动** | `store.ts:474-507`、`App.vue:168`、`Overview.vue:314` | A | A4-01 |
| U5 | `clash.Start` 起的子进程 stdout/stderr 被丢弃（helper 重定向 NUL，直连继承无效句柄），`Wait()` 丢弃退出码 → 内核启动失败**原因永久消失** | `helper:607-613`、`runner.go:319-323` | A | A3-04 / A3-13 |
| U6 | 「更新全部订阅」吞掉每个 err 恒返回 nil → 全失败也发 `subs-update-success` | `subscriptions.go:54-66` | A | A5-02 |

### 3.3 可观测（决定"能不能复盘"）

| # | 问题 | 位置 | 证据 | 来源 |
|---|---|---|---|---|
| O1 | 全仓**零落盘**：日志只在 500 条内存环，进程退出即归零；`data/logs` 只建不写，迁移还主动跳过 | `logger/buffer.go:26-82`、`paths.go:61`、`data_migration.go:230` | A | A3-01 |
| O2 | 资产自检/自愈的 10 处 `log.Printf` 打到 GUI 进程**不存在的 stderr**（实测 Subsystem=2 无控制台），也不进内存环 | `check.go:83/86/222`、`sync.go:166-216` | A | A3-03 |
| O3 | 崩溃/异常退出路径日志必丢：不抓栈、无退出标记、`os.Exit` 前只写内存 | `main.go:35-43`、`admin_win.go:69` | A | A3-02 |
| O4 | 用户能看到的每条错误通知，日志里都不存在（`notify` 只 Emit） | `controller.go:1362-1375` | A | A3-05 |

### 3.4 一致性与"静默错误"

| # | 问题 | 位置 | 证据 | 来源 |
|---|---|---|---|---|
| C1 | `RepairInvalid` 的 `Ready` 与 `VersionProbeOK` 解耦：损坏内核 / 0 字节 geo 在快路径上永不触发自愈 | `check.go:97-142`、`sync.go:181-182` | A | A1-05 |
| C2 | 资产 `Ready` 判定乐观：`CheckWintun` 的 alt 目录回退把"不在运行目录"的 DLL 判成 Ready；且与 `CleanupLegacyAssets` 互拆 | `check.go:199-228`、`migrate.go:144-171` | A/B | A1-07 |

---

## 4 五层根因模型（把 118 条收敛成 5 个结构性问题）

| 层 | 根因 | 典型证据 | 覆盖问题数 |
|---|---|---|---|
| L1 分层缺失 | 检查 = 深检 = 阻塞；无快检/深检/缓存三层 | 95 MB 哈希 × 4 轮（U1）、6 个 UI 方法直连 `GetStatus`（A0-01）、watchdog 每 5s 全量（A1-01） | ~30 |
| L2 错误无语义 | 错误用自然语言字符串冒泡；无 code/class/retryable；前端靠 `includes` 猜 | 只有大写 `Wintun` 三处命中缺驱动（A2/A4）、正文与 title 各塞一遍（A4-13）、3 个永远匹配不到的字符串（A4-15） | ~22 |
| L3 可观测为零 | 不落盘、丢 stderr、丢内核输出、丢弃计数静默 | O1~O4、F-A3-09~21 | ~21 |
| L4 契约脆弱 | 单模态槽、两套 TUN 实现、8 处自建模态、10 处直写 modal | U4、F-A4-02/03/07 | ~20 |
| L5 跨进程破损 | helper 白名单/管道权限/替换路径/SCM 时序与真实布局脱节 | S1、S2、F-A5-12（10s deadline 罩住 61MB 替换）、F-A0-05（9 处固定 Sleep） | ~25 |

---

## 5 与主方案（`SELF_HEAL_AND_LOGGING_PLAN.md`）的差异

### 5.1 方案需要修正的 8 处（已在 v2.1 勘误）

| # | 方案原表述 | 实际 | 来源 |
|---|---|---|---|
| 1 | `shouldCopy` 在 `check.go:181-188` | **`sync.go:176-207`** | A1 |
| 2 | 一次开 TUN 跑 3 轮资产自检 | **4 轮**（含 `clash.Start→PrepareEnv`），最坏 8 轮 | A1/A2 |
| 3 | `RepairRuntimeAssets` 有"恢复内置组件"按钮 | **前端 0 调用**（死接口） | A1/A4 |
| 4 | 前端 alert/confirm/toast = 67/18/7 | **69（66 行）/17/6**，另有 9 处零痕迹 `catch` | A4 |
| 5 | helper Ping 默认 10s，设置页可能卡 10s | **Ping 预算 200ms+150ms**；10s 只影响 `replace-core-file` | A2/A3/A5 |
| 6 | 未注册的 Wails 事件会被丢弃 | **未注册照常投递**；注册了但类型不符才被 cancel | A3/A5 |
| 7 | 任何含 wintun 字样的错误都会命中"缺驱动" | **只有大写 `Wintun` 三处 + `wintun_missing` 命中**，其余走通用模态 | A2/A4 |
| 8 | 启动成本只算 2 次 EnsureReady | 还漏了 `MigrateLegacyAssets` 的 **6 次 SHA256（≈91 MB）** | A5 |

### 5.2 方案已覆盖但深度不足的部分

- 方案 §7 只到"改成 L0/L1/L2 + AssetGuard"；A1 给出更硬的约束：**自愈必须跨进程单飞 + 固定 tmp 名必须改随机名 + 冷却必须原子**，否则并发自愈会产出半截 DLL。
- 方案 §6 只讲"结构化落盘"；A3 补充：**必须以 boot 段 + 崩溃标记 + dropped 计数 + helper 日志 + 内核 stdout/stderr 采集**为最小闭环，否则仍然无法复盘本次事故。

### 5.3 方案未覆盖的新增项（按优先级）

1. helper 管道权限与 `start-core` 白名单（安全，独立于体验改造）；
2. helper 白名单目录必须从 `install-profile.json` 解析，而不是猜 `<appDir>/data`；
3. 任务框架 `recover` + 任务错误日志（A5-01/06）；
4. 「更新全部订阅」的错误聚合（A5-02）；
5. 提权 `os.Exit` 清理（A5-03）；
6. 单模态槽改造为"模态队列/带 owner 的模态"（A4-01）；
7. 内核输出采集（A3-04）；
8. 迁移阶段每次启动的 91 MB 哈希（A5-14）。

---

## 6 修复序列（建议两批）

### Batch-0（半天，安全 + 止血，可独立发版）

1. 收紧 helper 管道 SDDL（去掉 `IU` 的 `GA`，改为最小权限 + 调用方 SID 白名单）；`start-core` 增加路径白名单（core 必须位于 `<dataDir>/core/bin` 且 PE 合法）。
2. helper 白名单改由 `install-profile.json` 解析 dataDir。
3. 任务框架加 `recover` + 结构化日志。
4. 日志最小闭环落盘（app + helper + 崩溃标记 + `notify` 也写日志）。
5. 前端误报面：删文案匹配、单模态槽加 owner/队列。

### Batch-1（3~5 天，结构改造）

6. Check Ladder + 落盘缓存 + UI 只读快照接口（`GetStatusCached`）。
7. AssetGuard：跨进程单飞、随机 tmp 名、分资产退避、PendingReplace。
8. 两段式 TUN + phase 事件；4 轮 EnsureReady 收敛为 1 轮快检 + 1 轮后台深检。
9. 内核 stdout/stderr 采集 + 退出原因进状态。
10. 订阅/备份/任务错误的可见性统一（code + class + 日志）。

### Batch-2（3~5 天，体验与可观测）

11. 诊断包（含日志/缓存/脱敏配置/系统信息）。
12. 日志面板：搜索/级别/暂停/导出/打开目录；前端批量合并日志事件。
13. 8 处自建模态收敛；两套 TUN 开关合并。
14. i18n 槽位与死键清理。

---

## 7 待实测清单（所有报告合并，共 9 项）

| # | 待验证 | 方法 | 影响 |
|---|---|---|---|
| 1 | 19:00:33 `core\bin` 的 tmp 文件归属 | Batch-0 落盘后复现一次 | 确认 U2 的现场 |
| 2 | 启动各阶段耗时分布 | `boot.phase` 埋点 | 定位"双击没反应" |
| 3 | 冷启动 4 轮自检的真实耗时 | `asset.probe` 埋点（level/dur/cacheHit） | 验证 §0 结论 3 |
| 4 | 火绒首扫对 61 MB 内核的放大 | 临时排除项对照实验 | 决定是否提示用户加白 |
| 5 | `-v` 超时率 | 日志统计 `ASSET_CORE_PROBE_TIMEOUT` | 决定 TTL |
| 6 | helper 白名单在两种安装布局下的失败率 | 构造 standard/portable 两套布局 | 验证 S2 |
| 7 | 管道权限利用面 | 安全评审（不要实机利用） | 验证 S1 |
| 8 | 模态槽卡死复现 | 构造 showAlert 打断 showConfirm | 验证 U4 |
| 9 | 订阅全失败场景 | 断网更新全部 | 验证 U6 |

---

## 8 索引

- 全部 118 条：见各分报告 §2（A1 20 条、A2 22 条、A3 21 条、A4 23 条、A5 25 条、A0 7 条），编号 `F-A{n}-xx` 全局唯一。
- 本报告只做收敛与排序，**不替代**分报告中的 file:line 与复现建议。
- 方案侧改动见 `docs/SELF_HEAL_AND_LOGGING_PLAN.md` §7（30 条）+ §11.7（10 条）+ v2.1 勘误。

---

*审计结论以本报告 + 6 份分报告为准；所有结论均可在仓库中按 file:line 复核。*
