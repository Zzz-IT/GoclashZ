# 严格检查 · 静默自愈 · 可回放日志：全链路审计与改造方案

> 版本：v2.1（2026-11-16，含第二轮审计 §11 + 团队审计勘误）
> 范围：Go 侧 `core/**`、`app.go`、`main.go`、`cmd/goclashz-helper`；前端 `frontend/src/**`
> 性质：**方案文档，不含代码改动**
> 触发案例：开 TUN 首次必定弹「缺少 Wintun 驱动」，重试正常（见 §0.2）

> ⚠️ **v2.1 勘误（团队审计回灌，权威结论见 docs/AUDIT_FULL.md）**
>
> 1. `RepairInvalid` 的 `shouldCopy` 在 **sync.go:176-207**（v1/v2 误写为 check.go:181-188）。
> 2. 一次开 TUN 实为 **4 轮** `EnsureReady`（多一处 `clash.Start → PrepareEnv → setup.go:28`），最坏 8 轮；v2 说 3 轮。
> 3. `RepairRuntimeAssets` **无前端调用方**（死接口），不存在「按钮假死」。
> 4. 前端计数：alert **69**（66 行）/ confirm **17** / toast **6**，另有 **9 处零痕迹 catch**（v2 写 67/18/7 + 28 处 console.error）。
> 5. helper Ping **不是 10s**：预算 200ms + 150ms；10s deadline 只影响 `replace-core-file`（61 MB 替换）。
> 6. **未注册的 Wails 事件不会被丢弃**（照常投递）；注册了但类型不符才会被 cancel。
> 7. 「缺驱动」分支只匹配**大写 `Wintun` 三处 + `wintun_missing`**；小写包装错误走通用模态（v2 §1.2-C 说反了）。
> 8. 启动成本漏算 `MigrateLegacyAssets` 的 **6 次 SHA256（≈91 MB）**，每次启动都跑。
>
> 新增团队发现：helper 管道对 IU 授 `GENERIC_ALL` 且 `start-core` 无白名单（**本地提权**）；helper 白名单目录与真实 dataDir 不一致（**替换/装驱动永久失败**）。详见 `docs/AUDIT_FULL.md` §3.1。

---

## 0. 摘要

### 0.1 一句话诊断

| 维度 | 现状 | 后果 |
|---|---|---|
| 检查 | **重且重复**：一次用户动作会跑 2~3 轮全量资产自检，每轮含 61 MB 内核哈希 + `clash.exe -v`（5s 超时）+ wintun 版本/哈希 + 4 个 geo 文件哈希（合计 ~30 MB） | 冷启动必然吃满预算（TUN 只有 25s），并把"慢"变成"失败" |
| 判定 | **粗**：失败原因用自然语言字符串向上冒泡，前端靠 `msg.includes('Wintun')` 猜语义 | 任何一句带 wintun 字样的错误都被翻译成"缺少驱动"并弹模态 |
| 恢复 | **弱**：修复失败被包级 60s 冷却记住、被 `ensureCoreRunning` 直接升级为硬失败；替换运行中的 DLL/EXE 必然失败 | 一次瞬时故障 → 一次弹窗 + 一分钟不可用 |
| 日志 | **空**：仅 500 条内存环，`HH:MM:SS`，无文件、无 code、无耗时、无关联 ID；`data\logs` 从不写入；启动早期事件被丢弃 | 事后**无法复盘**——这正是本次"为什么判成 missing"查不下去的根本原因 |

### 0.2 事故复盘的证据链（为什么要先修日志）

- `D:\GoclashZ\data\core\bin\wintun.dll` 存在，427,552 B，SHA256 `e5da8447…afce`，与 seed 完全一致 → **文件没缺**。
- 该文件 `CreationTime = LastWriteTime = 2026/9/22 14:20:07` → **从未被替换成功过**。
- 但 `core\bin` 目录的 `LastWriteTime = 2026/11/16 19:00:33`（= 首次点 TUN 的瞬间）→ **当时确实在目录里建/删过文件**（`wintun.dll.tmp` 写完、rename 失败、tmp 被删）。
- 火绒病毒库 18:57:53 更新，软件 18:59:34 启动 → 冷启动首访 61 MB 内核 + DLL 时正是杀软扫描压力最大的窗口。
- 日志为空 → 上述链条只能靠文件系统时间戳反推，**拿不到"哪一行代码判了 missing"**。

结论：**先把"过程可回放"补上，再改判定逻辑**。否则每次优化都只能靠猜。

### 0.3 方案骨架

1. **检查分级**（L0 快检 / L1 中检 / L2 深检）＋ 深检后台化 ＋ 结果落盘缓存 → 用户路径 800ms 内必有"确定状态或明确的进行中"。
2. **AssetGuard 自愈管道**：探针 → 计划 → 单飞执行 → 复核 → 发布；分资产冷却；运行中不替换被映射文件（改 PendingReplace 队列）。
3. **错误码化 + 三级 UI 策略**：A 静默重试 / B 非阻塞提示（toast、内联状态、日志角标）/ C 才允许弹模态。**删除一切文案匹配**。
4. **结构化落盘日志**（JSONL + 轮转 + 崩溃标记 + 诊断包），主进程与 helper 都要写。
5. 分 P0/P1/P2 三阶段落地，P0 半天到一天即可把"弹窗误报 + 无日志"这两个致命点堵住。

> **第二轮审计**（方法、覆盖面矩阵、调用点统计、新增 15 项发现、证据等级、未验证清单）见 **§11**。

---

## 1. 现状审计

### 1.1 检查链路清单（用户动作 → 实际跑了什么）

| # | 链路 / 入口 | 位置 | 严格度 | 单次成本 | 阻塞用户 | 误报面 | 频率 |
|---|---|---|---|---|---|---|---|
| 1 | `runtimeassets.GetStatus` | `core/runtimeassets/status.go:19-42` | 全量 | PE 校验 + 61 MB SHA256 + `-v`(≤5s) + DLL 版本/哈希 + 4×geo SHA256 | 是（同步） | 任何一次 IO/进程异常 → 全链路降级 | 每次 EnsureReady |
| 2 | `CheckCore` → `ReadCoreVersion` | `core/runtimeassets/check.go:91-197` | 严（跑进程） | 进程 spawn + 5s 超时；冷启动杀软扫描常超时 | 是 | 超时 → `VersionProbeOK=false` → 触发 61 MB 重拷 | 每次 GetStatus |
| 3 | `CheckWintun` | `core/runtimeassets/check.go:199-273` | 严（PE+版本+哈希） | 427 KB ×3 次 IO + version API | 是 | §1.2-A 的 `VersionProbeOK` 缺陷 | 每次 GetStatus |
| 4 | `adoptAssetFromFallback` | `core/runtimeassets/check.go:65-89` | — | 同步 `CopyFile`（最多 61 MB） | **是** | 在"检查"里做"写入"，检查本身变重 | 每次 GetStatus |
| 5 | `EnsureReady` | `core/runtimeassets/status.go:45-73` | 严 | 1×GetStatus，修复路径 2~3× | 是 | 修复路径把耗时再翻倍 | 每层调用 |
| 6 | `RepairFromSeed` 冷却 | `core/runtimeassets/sync.go:107-134` | — | 失败后**包级 60s** 内直接返回旧错误 | 是 | 一次瞬时失败 → 一分钟内连"重试机会"都没有 | 全局 |
| 7 | `RepairInvalid` 的 shouldCopy | `core/runtimeassets/sync.go:176-207` | 逻辑缺陷 | 每次自愈都重拷 wintun | 是 | **wintun 永远 shouldCopy=true**（§1.2-A） | 每次 EnsureReady |
| 8 | `prepareTunEnable` | `core/appcore/controller.go:1760-1787` | 严 | 1×(helper ping + EnsureReady) | 是 | 只对 `ErrMissing` 硬拦（设计正确），但文案被前端曲解 | 每次开 TUN |
| 9 | `supervisor.Reconcile` | `core/appcore/supervisor.go:142-221` | 严 | **又是 1×EnsureReady**（watchdog 每 5s 也跑） | 是 | TUN 开着的整个生命周期每 5s 一次全量自检 | 5s / 事件 |
| 10 | `ensureCoreRunningWithDesiredState` | `core/appcore/controller.go:626-662` | 最严 | **第三次 EnsureReady**，且**任何**修复错误 → `运行组件不可用` 硬失败 | 是 | 一次 tmp 写失败 = 开 TUN 失败 | 每次起内核 |
| 11 | `EnsureHelperReady` | `core/appcore/controller.go:1696-1750` | 中 | singleflight 无超时；SCM 启服务 + 25×200ms 等待；非管理员直接返回引导错误 | 是 | 冷启动最坏 ≈5s+ | 每次 TUN |
| 12 | `waitForTunAdapter` | `core/appcore/controller.go:1074-1123` | 中 | 20s 轮询 ×200ms（每次枚举适配器+路由表） | 是 | 冷启动 wintun 适配器常 >10s（代码注释自认） | 每次开 TUN |
| 13 | `clash.ValidateConfig`（`-t`） | `core/clash/runner.go:249-279` | 严 | 每次起内核前 spawn 一次，3s 超时；失败直接回显内核 stdout | 是 | 配置没变也每次都跑 | 每次起内核 |
| 14 | `CoreSupportsMips` / `GetLocalCoreVersion` | `core/clash/setup.go:64-129, 432-450` | 严 | 又一次 `-v`（另一份缓存） | 前端首屏链上 | 首屏多等一轮 | 首屏 + 设置页 |
| 15 | `CheckTunEnv` | `app.go:727-738` | 严 | GetStatus 全量 + `CheckHelperService`（Ping 默认 10s deadline）+ `CoreSupportsMips` | 前端 `App.vue:641` 启动即调 | 首屏状态长时间"未知" | 每次启动 |
| 16 | 启动资产维护 | `app.go:166-191` | 严 | Migrate + EnsureReady(RequireTun) + EnsureReady(RequireGeo) + Cleanup，**同步串行且早于 UI** | **是（启动）** | 种子目录异常 → 启动被拖慢 | 每次启动 |
| 17 | 启动恢复 | `core/appcore/controller.go:380-444` | 中 | 2/5/10/20/40s 退避 ×5，每次全量 Reconcile | 后台（但会 notify） | 冷启动未就绪 → 反复重试 → 最后弹通知 | 每次启动 |
| 18 | `clash.PrepareEnv` | `core/clash/setup.go:26-42` | 中 | 又一次 EnsureReady(RequireCoreOnly) | 是 | 与 #5/#10 重复 | 每次起内核 |
| 19 | geo 在线补齐 | `core/appcore/env.go:15-51` | 中 | 缺失时同步下载（200 MB 上限） | **是** | 断网 → 起内核被拖住 | 起内核 |
| 20 | 前端错误翻译 | `frontend/src/utils/errors.ts:10-20`、`Overview.vue:291` | **最危险** | — | — | 文案猜语义 | 任意错误 |

### 1.2 三个结构性缺陷（决定了"严格检查"必然变成"误报"）

**A. wintun 的 `VersionProbeOK` 永远是 false → 每次自愈都重拷 DLL**

```go
// sync.go:176-207（v1 误写为 check.go:181-188，已按 A1 审计修正）
case RepairInvalid:
    if isCoreAsset(key) {          // core 和 wintun 都算
        shouldCopy = !runtimeHealth.Ready || !runtimeHealth.VersionProbeOK
    }
```

`VersionProbeOK` 只在 `checkCoreByPath` 里被置位（`check.go:140`），`checkWintunByPath` 从不设置（`check.go:230-273`）。
于是只要走到修复分支，wintun **必然**被重拷：写 `wintun.dll.tmp` → `os.Rename` 覆盖。
- TUN 开着时 DLL 被 clash.exe 以 image 方式映射 → rename 必失败 → 产生一次"文件在、修复失败"的假故障；
- 于是 `ensureCoreRunningWithDesiredState:655-661` 把这次失败升级成 `运行组件不可用` 硬失败。

**B. 检查里夹带写入与失败记忆**

`adoptAssetFromFallback`（`check.go:65-89`）在 `CheckWintun` 里同步复制文件；`RepairFromSeed` 的 60s 冷却是**包级变量**（`sync.go:112-134`），一次失败污染所有后续调用，包括用户刚点的 TUN。

**C. 判定与文案脱钩**

```ts
// Overview.vue:291
} else if (msg.includes('wintun_missing') || msg.includes('Wintun')) {
```

Go 侧只有两处会产生这种文案（`controller.go:1778`、`supervisor.go:167`），触发条件都是"文件真的不存在"；一旦上游任何一层把别的错误包装进来（`运行组件不可用: %w`、`runtime 事务失败于阶段 %s`），前端就无法区分"缺驱动"和"驱动被占用"，统一弹模态。

### 1.3 日志系统审计

| 项 | 现状 | 位置 | 问题 |
|---|---|---|---|
| 存储 | 全局单例 `RingBuffer(500)`，纯内存 | `core/logger/buffer.go:26-82` | 进程退出即丢；500 条被 core 日志冲爆 |
| 时间 | `time.Now().Format("15:04:05")` | `buffer.go:96` 等 4 处 | 无日期、无毫秒、无时区 → 跨天/跨次启动无法排序 |
| 级别 | `NormalizeLevel` 四档 | `core/logger/level.go` | 无"类别/子系统"维度 |
| 双通道 | `AddApp` 按用户级别过滤后 emit；`AddCore` **不过滤** | `core/appcore/log_dispatcher.go:30-66` | core 日志任何级别都推 UI，且与 app 共用一个环 |
| 早期事件 | `ctx == nil` 直接 return | `wails_event_sink.go:37-39` | 启动阶段（最需要证据）的事件**完全没记录** |
| 通知去重 | 30s 窗口内重复 error 直接 return | `core/appcore/controller.go:1362-1365` | **连日志都不写**，时间线断裂 |
| 落盘 | 无。`data/logs` 由 `core/utils/paths.go:61` 创建但从不写入（实测空目录） | — | 无法事后复盘 |
| 崩溃 | `recover` 只写内存环 | `main.go:35-43` | GUI 子系统无 stderr，panic 后无痕 |
| helper | 完全黑盒 | `cmd/goclashz-helper` | 停止确认/替换文件失败无法归因 |
| 诊断 | 只有单点快照 | `core/appcore/diagnostics.go:49-97` | 无时间线、无最近日志、无适配器历史 |

### 1.4 前端提示面审计

| 位置 | 当前行为 | 问题 |
|---|---|---|
| `Overview.vue:252-319` | 任何 TUN 错误 → 三分支（helper / wintun / 通用）→ **模态** | 瞬时错误也弹；wintun 分支靠文案 |
| `SettingsTun.vue:142-156` | 未装驱动 → 模态；错误 → 模态回显原文 | 与 Overview 两套逻辑，行为不一致 |
| `store.ts:749-777` | 通知只处理 `tray/startup/deferred-action` 且 level=error → **模态** | 后台自愈失败也弹 |
| `store.ts:474-507` | `showAlert`/`showConfirm` 两套模态 | 已有 `showToast`（:907-930），但错误路径几乎不用 |
| `errors.ts:10-20` | 正则猜语义（order 敏感） | 新增文案极易归错类 |
| `App.vue:698-700` | `tun-mips-fallback` → 模态 | 属"可自动降级"事件，不该弹 |

### 1.5 结论：要改的不是"检查太严"，而是"检查的位置、时机与失败语义"

严格检查本身是对的（防损坏、防投毒）。问题是：

1. 它发生在**用户按下开关的同步路径**上；
2. 它**重复执行**（同一次动作 3 轮）；
3. 它**用同一种方式表达一切失败**（错误字符串 → 模态）。

---

## 2. 目标与原则

### 2.1 用户可见契约（硬指标）

| 场景 | 期望 |
|---|---|
| 点任意开关 | **≤100ms** 界面有反应（状态变为"正在开启/关闭"），按钮不假死 |
| 冷启动首次开 TUN | **≤2s** 给出三种确定状态之一：已开启 / 正在建立适配器（继续后台）/ 需要你处理（此时才允许弹窗） |
| 自愈动作（重拷资产、唤醒 helper、重建适配器） | 全程**零弹窗**；进度进状态条 + 日志；失败自动重试 ≥2 次 |
| 真正不可恢复（seed 也缺 / 无权限 / 配置语法错 / 磁盘只读 / 端口被占） | 弹一次模态，带 code，可"复制诊断" |
| 任何一次失败 | **5 分钟内**可从 jsonl 还原完整因果链（谁判的、判据、耗时、重试次数） |

### 2.2 五条原则

1. **检查严，时机后移**：严格检查保留，但从"同步门禁"变成"后台巡检 + 门禁只看 L0 快检"。
2. **一切失败先自愈再说话**：失败 → 静默重试（退避 200ms/1s/3s）→ 仍失败才升级为用户可见。
3. **错误必须可编程**：一律 `code + class + 结构化 detail`，前端按 code 决策，禁止文案匹配。
4. **弹窗是最后的资源**：只有"用户必须做选择或必须知情且无法自愈"才用模态；其余用 toast/内联/日志角标。
5. **可回放优先**：任何优化前先保证"能看到上一次为什么失败"。

---

## 3. 检查分级模型（Check Ladder）

### 3.1 三级定义

| 级别 | 预算 | 内容 | 允许出现在用户同步路径 | 缓存 |
|---|---|---|---|---|
| **L0 快检** | ≤5 ms | `os.Stat`(size/mtime)、进程存活、pipe 可达、适配器 Up、desired 状态 | 是唯一门禁 | 无需 |
| **L1 中检** | ≤100 ms | PE 头（MZ+大小）、`GetFileVersion`、配置语法（纯解析，不跑进程）、seed manifest 存在性 | 可 | 内存 30s |
| **L2 深检** | 秒级 | `SHA256`、`clash.exe -v`、`clash.exe -t`、seed 逐字节对账、权限探测 | 仅后台 | 落盘缓存（3.3） |

### 3.2 现有检查的重新归类

| 现有检查 | 新级别 | 说明 |
|---|---|---|
| `CheckCore` 的 stat/PE 头 | L0+L1 | 只读 2 字节 + size，够快，保留 |
| `CheckCore` 的 SHA256 | **L2** | 仅在"准备替换/seed 对账"时算 |
| `ReadCoreVersion`(`-v`) | **L2** | 结果按 (path,size,mtime) 落盘缓存 7 天；后台刷新 |
| `CheckWintun` PE+版本 | L1 | `GetFileVersion` 失败不再等同"不可用"，只降级 Version 文案 |
| wintun/geo SHA256 | **L2** | 同上 |
| `ValidateConfig`(`-t`) | **L2** | 只在配置/TUN/DNS 变更后的首次启动跑；按 configHash 缓存 |
| `EnsureHelperReady` SCM 启动 | L0 快 ping（800ms）+ 后台 | ping 失败先返回"正在唤醒后台服务"，后台启服务 |
| `waitForTunAdapter` 20s | **后台** | 同步段只做 1 次探测；未 Up 返回"正在建立"，watchdog 收敛 |
| `CheckTunEnv`（首屏） | L0+L1 | 只读缓存结果，不触发深检 |

### 3.3 缓存设计

`%DATA%\runtime\asset-cache.json`（原子写，0600）：

```json
{
  "version": 1,
  "entries": {
    "core":   {"path":"...clash.exe","size":61304320,"mtime":1792...,"sha256":"926b...","ver":"v1.19.33","verAt":1792...},
    "wintun": {"path":"...wintun.dll","size":427552,"mtime":1792...,"sha256":"e5da...","ver":"0.14.1"}
  },
  "configHash": "sha256(runtime config + tun/dns 设置)",
  "checkedAt": 1792...
}
```

规则：**stat 命中 (path,size,mtime) 就直接复用结论**；mtime 变化才重算。冷启动时 L0/L1 全命中 → 用户路径 0 次哈希。

---

## 4. 静默自愈管道（AssetGuard）

### 4.1 结构

```
            ┌────────────┐   L0/L1    ┌──────────┐
 触发源 ──▶ │  Prober    │ ─────────▶ │ Planner  │ ──▶ Ready / Degraded
 (启动/开关/  └────────────┘            └────┬─────┘
  watchdog)                                  │ needRepair
                                    ┌────────▼────────┐
                                    │ Executor(单飞+锁)│ ── PendingReplace / CopyFromSeed / WakeHelper
                                    └────────┬────────┘
                                    ┌────────▼────────┐
                                    │ Verifier(L0/L1) │ ──▶ Ready / Failed(可重试 n 次)
                                    └────────┬────────┘
                                    ┌────────▼────────┐
                                    │ Publisher       │ ──▶ 事件 / 日志 / 状态条
                                    └─────────────────┘
```

### 4.2 关键规则

| 规则 | 具体做法 | 解决的问题 |
|---|---|---|
| **单飞** | 进程内 `singleflight`（key=asset 名）+ 跨进程文件锁 `%DATA%\runtime\.asset.lock` | 主进程/helper/watchdog 并发替换同一文件 |
| **分资产冷却** | 每个资产独立 `nextRetryAt`、独立退避（200ms→1s→3s→10s，上限 30s），**不再用包级 60s** | 一次失败污染全局 |
| **不在运行中替换** | 目标被映射（clash.exe/wintun.dll 正被内核使用）→ 不 rename，转 `PendingReplace` 队列，**下次内核停止时原子 apply** | 消除"文件在、修复失败"的假故障 |
| **检查不写入** | 所有 `CopyFile` 从 `Check*` 移出，只由 Executor 做 | 检查变纯、可并发、可缓存 |
| **修复不改判定** | 修复失败只把资产标为 Degraded，**不再让上层 `EnsureReady` 返回错误** | 修复失败不再升级为开关失败 |
| **限时** | Executor 单次 ≤3s（大文件转后台任务），超出返回"进行中" | 不阻塞 UI |
| **可观测** | 每次 Probe/Repair/Verify 写结构化日志（dur_ms、attempt、blockedBy） | 可回放 |

### 4.3 状态机

```
Unknown ──probe──▶ Ready
   │                 ▲
   ├──probe──▶ Degraded ──repair──▶ Repairing ──verify ok──▶ Ready
   │                                  └──verify fail(n<3)──▶ Degraded
   │                                  └──verify fail(n≥3)──▶ Unrecoverable(需用户)
   └──probe──▶ Unrecoverable   （仅：文件不存在且 seed 也缺 / 无权限 / 磁盘只读）
```

### 4.4 TUN 开启的"两段式"

```
ToggleTunMode(true)
  ├─ 同步段（≤1.5s）：L0/L1 快检 → 提交 desired → 返回 accepted + phase
  │    phase = "starting" | "repairing" | "needs-action"
  └─ 异步段（后台，≤45s）：AssetGuard → helper 就绪 → 写配置 → 起内核 → 等 API → 等适配器
       → 事件 tun-phase / tun-phase-detail 持续推给 UI
```

UI 用 `tunPhase` 呈现：`idle / checking / repairing(带进度) / starting / on / failed(code)`。
失败只在 `needs-action`（Unrecoverable）时弹模态，其余静默 + 状态条 + 日志。

> 兼容性：保留旧同步路径，用 `behavior.AsyncTunEnable`（默认 true）灰度，可即时回退。

---

## 5. 错误分级与 UI 策略

### 5.1 三分类

| Class | 含义 | 处理 | UI |
|---|---|---|---|
| **A 瞬时可自愈** | 占用、超时、冷启动未就绪、适配器晚到、pipe 暂不可达 | 静默重试 + AssetGuard | 无弹窗；状态条"正在…"；>5s 可 toast |
| **B 需干预但不阻塞** | helper 未安装、驱动版本旧、seed 缺失但可在线补、需重载 | 引导 + 一键动作 | toast + 内联按钮 + 日志角标 |
| **C 不可恢复** | 配置语法错、无权限且无法提权、磁盘只读、端口被占、seed 与 runtime 双缺失 | 停止自动重试 | 模态（code + 复制诊断） |

### 5.2 错误码表（初始集合）

| code | class | 触发条件 | 自愈动作 | UI |
|---|---|---|---|---|
| `ASSET_WINTUN_MISSING` | C | runtime 缺 + seed 也缺 | 无（引导安装/联网） | 模态 |
| `ASSET_WINTUN_BUSY` | A | rename 失败/文件被映射 | 转 PendingReplace，停止时 apply | 无 |
| `ASSET_CORE_PROBE_TIMEOUT` | A | `-v` 超时 | 用缓存版本，后台重试 | 无 |
| `ASSET_SEED_UNAVAILABLE` | B | seed 目录不存在/清单坏 | 在线补齐提示 | toast |
| `HELPER_NOT_INSTALLED` | B | 服务未注册 | 引导安装（一次性） | 模态（仅首次） |
| `HELPER_WAKE_TIMEOUT` | A | SCM 启动超时 | 后台重试 + watchdog | 无 |
| `TUN_ADAPTER_PENDING` | A | 探测未 Up（20s 内） | watchdog 收敛 | 状态"正在建立" |
| `CONFIG_INVALID` | C | `-t` 失败 | 回滚上一份可用配置 | 模态（带行号） |
| `PORT_IN_USE` | C | API/代理端口占用 | 建议换端口 | 模态 |
| `PERMISSION_DENIED` | C | 无写权限且提权失败 | 无 | 模态 |
| `DISK_READONLY` | C | 写测试失败 | 无 | 模态 |

> 实施要点：Go 侧返回 `*apperr.Error{Code, Class, Msg, Detail, Retryable}`；在不改 Wails 绑定协议的前提下，message 统一前缀 `CODE:<code>|`，前端 `parseCode(msg)` 兜底；若允许改协议则直接结构化传。

### 5.3 前端改造点

| 位置 | 改法 |
|---|---|
| `Overview.vue:252-319` | 删除 `includes('Wintun')`；仅 `code===ASSET_WINTUN_MISSING`（C）才 showConfirm；A/B 走 toast/状态条 |
| `SettingsTun.vue:142-156` | 与 Overview 共用 `useTunToggle()`，禁止两份逻辑 |
| `store.ts:749-777` | 按 `notification.code` 的 class 路由：C→模态，B→toast+入口，A→只进日志/状态条 |
| `store.ts:907-930` | 已有 toast，补齐 `action` 按钮（"重试"/"查看日志"） |
| `errors.ts` | 删除正则猜语义，改 `code → i18n key`；无 code 时显示"未知错误（已记录日志）" |
| `App.vue:698-700` | mips fallback 改 toast |

---

## 6. 日志系统改造

### 6.1 目标形态

- **双写**：内存环（UI 实时面板）+ JSONL 文件（复盘/诊断包）。
- **分级配额**：内存环 2000 条，其中 error/warn 至少保 500 条（core 日志不得挤掉 app 错误）。
- **两进程**：`app-YYYYMMDD.jsonl`、`helper-YYYYMMDD.jsonl`。
- **轮转**：单文件 8 MB / 保留 7 天 / 总量 ≤80 MB；异步 channel（4096）+ 满则丢 debug 并计数。

### 6.2 记录结构（JSONL 一行一条）

```json
{"ts":"2026-11-16T19:00:33.412+08:00","lvl":"warn","cat":"asset","event":"asset.repair.failed",
 "code":"ASSET_WINTUN_BUSY","corr":"tun-toggle-8f3a","pid":6868,"role":"main","attempt":1,"dur_ms":182,
 "assets":{"wintun":{"ready":true,"path":"D:\\GoclashZ\\data\\core\\bin\\wintun.dll"}},
 "err":"replace ... failed: Access is denied.","extra":{"tmp":"wintun.dll.tmp","mappedBy":"clash.exe"}}
```

| 字段 | 说明 |
|---|---|
| `ts` | ISO8601 带毫秒与时区 |
| `lvl` | debug/info/warn/error |
| `cat` | asset / helper / core / tun / proxy / update / ui / boot / config |
| `event` | 稳定事件名（6.3），不要用自由文本 |
| `code` | 错误码（仅失败） |
| `corr` | 一次用户动作/事务的关联 ID |
| `role` / `pid` | main / helper |
| `dur_ms` / `attempt` | 耗时与第几次尝试 |
| `extra` | 结构化补充（路径、哈希、适配器快照） |

### 6.3 必埋事件（复盘五件套）

| event | 何时 | 关键字段 |
|---|---|---|
| `boot.phase` | 启动各阶段（迁移/资产/helper/UI 就绪/恢复） | phase, dur_ms, result |
| `asset.probe` | 每次 L0/L1/L2 检查 | asset, level, ready, code, dur_ms, cacheHit |
| `asset.repair.start/end` | 每次自愈 | asset, from, to, sha, attempt, blockedBy, dur_ms |
| `ui.action` | 每个开关/保存/重试 | action, from, to, corr, result, dur_ms |
| `err.raise` | 每个错误（含被 UI 抑制的） | code, class, msg, willShowUi, corr |
| `helper.rpc` | 每次 pipe 调用 | method, ok, dur_ms, err |
| `core.proc` | 内核启停 | pid, tun, exitCode, dur_ms |
| `tun.phase` | TUN 阶段变化 | from, to, adapter, dur_ms |

### 6.4 生命周期与崩溃

- 启动写 `boot.start`（版本、路径、权限、helper 状态、seed 目录、上次是否异常退出）。
- 正常退出写 `boot.exit{clean:true}`；下次启动若未见 clean → `boot.previousRunUnclean` + 常驻 warn。
- `main.go:35-43` 的 panic 分支：先 `Flush()` 再 `ClearOwnedSystemProxy()`，栈写文件。
- `WailsEventSink`：`ctx==nil` 时不再丢弃，改 pending 队列（≤256）+ 落盘，`SetContext` 后补发。
- `notify()` 去重时**仍然写日志**（`suppressed:true`），保证时间线完整。

### 6.5 诊断包（一键）

`设置 → 关于 → 导出诊断包` 产出 `GoclashZ-diag-YYYYMMDD-HHMMSS.zip`：

```
diagnostics.json        （GetDiagnosticInfo + 适配器快照 + helper 状态 + 缓存）
logs/app-*.jsonl        （最近 3 天）
logs/helper-*.jsonl
asset-cache.json / asset-state.json / seed manifest
runtime/config.yaml     （脱敏：订阅 URL、uuid/password、节点地址）
system.txt              （OS 版本、网卡、TUN 路由摘要、服务状态）
```

脱敏集中在 `core/utils/redact.go`：`(password|uuid|token|secret|url|host)` 统一替换，保留可定位前后缀。

### 6.6 性能预算

| 项 | 预算 |
|---|---|
| 单条日志序列化 + 入队 | ≤50 µs（异步，不阻塞调用方） |
| 每日写入量（info 级） | ≤5 MB |
| 诊断包 | ≤3s，≤10 MB |
| 对开关延迟的影响 | 0（全异步；队列满丢 debug） |

---

## 7. 逐条改造清单

> P0 = 立即止血（0.5~1 天）；P1 = 结构改造（3~5 天）；P2 = 体验与可观测（3~5 天）。

| # | 文件:行 | 现状 | 改法 | 优先级 |
|---|---|---|---|---|
| 1 | `core/logger/*` | 只有内存环 | 新增 `file.go`（JSONL+轮转+Flush）、`fields.go`、`redact.go`；保留环作 UI 源 | **P0** |
| 2 | `log_dispatcher.go:30-66` | core 不过滤；共用一个环 | core 按级别过滤；内存环分级配额；两条流都落盘 | **P0** |
| 3 | `controller.go:1362-1375` | 去重即 return | 去重时写 `err.raise{suppressed:true}` | **P0** |
| 4 | `wails_event_sink.go:37-39` | 早期事件丢弃 | pending 队列 + 落盘，SetContext 后补发 | **P0** |
| 5 | `Overview.vue:291` | `includes('Wintun')` | 改 code 判定；A/B 走 toast/状态条 | **P0** |
| 6 | `errors.ts:10-20` | 正则猜语义 | code→i18n 表；未知 code 显示"已记录日志" | **P0** |
| 7 | `controller.go:655-661` | 任何修复失败→硬失败 | 降级为 warn + 后台修复 + 返回 repairing | **P0** |
| 8 | `sync.go:176-207` | wintun 永远 shouldCopy | wintun 用"PE 有效 + 记录版本"判定；shouldCopy 只看 Exists/Valid | **P0** |
| 9 | `sync.go:107-134` | 包级 60s 冷却 | 分资产冷却 + 退避；失败不返回给用户路径 | **P0** |
| 10 | `check.go:65-89` | 检查里 CopyFile | 移除；迁移逻辑只在 Executor 做 | **P1** |
| 11 | `status.go:45-73` | EnsureReady 返回 error | 拆 `Probe()`（无副作用）+ `Ensure()`（后台修复） | **P1** |
| 12 | `supervisor.go:154-175` | watchdog 每 5s 全量 | 改 L0 快检 + 状态变化才深检 | **P1** |
| 13 | `controller.go:1760-1787` | 同步 EnsureReady | 只跑 L0/L1，深检交后台 | **P1** |
| 14 | `controller.go:626-662` | 第三次 EnsureReady | 删除（统一到 prepare 阶段） | **P1** |
| 15 | `runner.go:249-279` | 每次跑 `-t` | 按 configHash 缓存 | **P1** |
| 16 | `check.go:309-321` | 每次 SHA256 | 落盘缓存 (size,mtime) | **P1** |
| 17 | `check.go:144-197` | 每次 `-v` | 落盘缓存 7 天 + 后台刷新 | **P1** |
| 18 | `app.go:166-191` | 启动同步阻塞 | 后台化，UI 先出 | **P1** |
| 19 | `app.go:370-374` | 25s 同步 | 两段式：同步 1.5s + 异步 45s + phase 事件 | **P1** |
| 20 | `app.go:727-738` | CheckTunEnv 全量 | 只读缓存 + L1 | **P1** |
| 21 | `controller.go:1696-1750` | singleflight 无超时 | 快 ping 800ms；SCM 启动后台化 | **P1** |
| 22 | `controller.go:1074-1113` | 20s 同步轮询 | 同步 1 次探测；未 Up 返回 `TUN_ADAPTER_PENDING`(A) | **P1** |
| 23 | `control_coordinator.go:128-141` | 失败即回滚 desired | 仅 C 类回滚；A 保留 intent 后台重试 | **P1** |
| 24 | `diagnostics.go:49-97` | 单点快照 | 打包 zip（日志/缓存/脱敏配置） | **P2** |
| 25 | `App.vue:676-693` | 日志面板 500 条 | 分级筛选 + 搜索 + 打开日志目录 | **P2** |
| 26 | `main.go:35-43` | panic 只写内存 | Flush + 栈落盘 + previousRunUnclean | **P0** |
| 27 | `cmd/goclashz-helper/*` | 无日志 | helper 落盘 jsonl（含每次 RPC） | **P1** |
| 28 | `SettingsTun.vue:142-156` | 与 Overview 两套 | 抽 `useTunToggle()` | **P2** |
| 29 | 全站 | 无错误 code | 引入 `apperr` 包，替换用户可见文案 | **P1** |
| 30 | 全站 | 无 feature flag | `AsyncTunEnable` / `SilentSelfHeal` 灰度开关 | **P1** |

---

## 8. 落地节奏

### P0（止血，0.5~1 天）

1. 日志落盘（#1 #2 #3 #4 #26）——没有它，后续优化都无法验证。
2. 误报面（#5 #6）——删文案匹配，模态只在 C 类。
3. 硬失败降级（#7 #8 #9）——让"修复失败"不再等于"开关失败"。

**验收**：注入"wintun.dll 被占用"，点 TUN：无弹窗、状态条显示"正在准备驱动"、日志里有 `asset.repair.failed(ASSET_WINTUN_BUSY)` 与 `tun.phase` 时间线；30s 后自动成功。

### P1（结构，3~5 天）

4. Check Ladder（#10~#13 #16 #17 #20）。
5. AssetGuard + PendingReplace + 分资产冷却（#9 #10 #11）。
6. 两段式 TUN + phase 事件（#19 #22 #23）。
7. helper 日志 + 超时改造（#21 #27）。
8. 启动异步化（#18）。

**验收**：冷启动首开 TUN，UI 1s 内响应，P95 ≤2s 进入"已开启/正在建立"，全程零弹窗；同一次动作只跑 1 轮 L0/L1 + 后台 1 轮 L2。

### P2（体验，3~5 天）

9. 诊断包（#24）、日志面板（#25）、toast 动作按钮、`useTunToggle()` 收敛（#28）、脱敏（#29）。

**验收**：一键导出诊断包；任意 error 可按 code 筛选；诊断包 ≤10 MB 且无敏感信息。

### 风险与回滚

| 风险 | 缓解 |
|---|---|
| 两段式改变 TUN 语义 | `AsyncTunEnable` 开关，默认灰度，可即时回退 |
| 缓存导致"文件已坏却判 Ready" | L0 命中仍需 L2 后台复核（启动 30s 内）；失败自动重检 |
| PendingReplace 忘记 apply | 每次内核停止/下次启动检查队列；超 24h 记 warn |
| 日志写盘影响性能 | 全异步 + 队列满丢 debug + 8MB 轮转 |

---

## 9. 验收标准与回归用例

### 9.1 指标

| 指标 | 目标 |
|---|---|
| 正常运维下的模态弹窗数 | 0（除 C 类） |
| TUN 首开 UI 响应 | ≤100 ms |
| TUN 首开确定状态 | P95 ≤2 s |
| 单次开关的 L2 深检次数 | 稳态 0（缓存命中），冷启动 ≤1 |
| 失败可复盘率 | 100%（jsonl 含 code/corr/dur/attempt） |
| 诊断包 | ≤10 MB，≤3 s |

### 9.2 故障注入用例（必须自动化）

| 用例 | 注入方式 | 期望 |
|---|---|---|
| DLL 被占用 | 独立进程 LoadLibrary 锁住 wintun.dll | 无弹窗；PendingReplace；内核停止后自动替换 |
| seed 缺失 | 重命名 seed 目录 | B 类 toast；不阻塞已就绪的运行 |
| seed + runtime 双缺失 | 同时删两处 | C 类模态，文案含 code |
| helper 未安装 | 卸载服务 | 首次引导模态，之后 toast |
| helper 唤醒慢 | 服务设延迟启动 | A 类，后台重试，不弹 |
| 适配器晚到 30s | 延迟驱动/限速 | 状态"正在建立"，watchdog 收敛，不弹 |
| 磁盘只读 | 挂只读卷 | C 类模态 |
| 配置语法错 | 写入坏 yaml | C 类模态带行号，回滚上一份 |
| 日志队列打满 | 灌 10k 条 debug | 丢 debug、计数、不阻塞、不丢 error |
| 进程崩溃 | 断言失败注入 | 下次启动 previousRunUnclean + 栈在文件里 |

---

## 10. 附录

### 10.1 错误分类速查

- **A（静默）**：占用/超时/未就绪/唤醒慢/适配器晚到/pipe 暂不可达。
- **B（提示不阻塞）**：helper 未安装、驱动旧、seed 可在线补、需重载。
- **C（阻塞）**：双缺失、权限、只读、端口占用、配置错误。

### 10.2 事件命名规范

`<域>.<对象>.<动作>`：`asset.repair.start`、`asset.probe`、`tun.phase`、`helper.rpc`、`ui.action`、`boot.phase`、`err.raise`。

### 10.3 与现有事件的衔接

现有 62 个 typed event（`v3_events.go`）保持不变；新增事件（`tun-phase`、`asset-repair-progress`、`asset-state`）必须在 `v3_events.go` 注册，否则会被 v3 的类型校验丢弃。

### 10.4 一句话回顾

> **检查要严，但只该严在后台；错误要多，但只该多进日志。**

---

## 11. 第二轮审计（v2 增补）

> 第一版只深挖了"资产自检 / TUN 启停 / helper / 日志"四条主链，覆盖约 40%。本节补齐调用点统计、UI 直连深检、任务框架、配置事务回滚、前端提示面，并列出**没查到/没验证**的部分。

### 11.1 方法与证据等级

- 手段：全仓 grep 调用点计数 → 逐文件读关键函数 → 与实机文件系统/进程/服务状态对账 → 前端调用点统计。
- 证据等级：**A** = 代码可确证；**B** = 代码 + 运行时状态一致推断；**C** = 假设，待实测。

| 子系统 | 覆盖度 | 等级 | 说明 |
|---|---|---|---|
| 资产自检与自愈 | 92% | A | runtimeassets 全量读完 |
| TUN 启停 / 适配器探测 | 90% | A | controller 相关函数全读 |
| helper RPC / SCM | 75% | B | 未读 `superviseCore` / `startCoreProcess` 细节 |
| 系统代理 | 80% | A | 未读 `proxy_win.go` 的 WinINet 细节 |
| 配置切换 / 回滚 | 85% | A | switchConfigLocked + rollback 全读 |
| 后台任务框架 | 100% | A | `core/tasks/manager.go` 仅 122 行 |
| 更新（app/core/geo/wintun） | 60% | B | downloader 未逐行 |
| 订阅 / 规则 / 备份 / YAML | 40% | C | 只审调用边界 |
| 日志系统 | 100% | A | logger + dispatcher + sink 全读 |
| 前端提示面 | 95% | A | 67 处 alert 已计数，未逐条读实现 |
| 提权 / UWP / 开机自启 | 50% | C | 抽查 `--repair-permissions` 等入口 |

### 11.2 调用点统计（这些数字本身就说明问题）

| API | 调用点 | 位置 |
|---|---|---|
| `runtimeassets.EnsureReady` | **7** | `app.go:170/181/902`、`core/clash/setup.go:28`、`controller.go:655/1770`、`supervisor.go:155` |
| `runtimeassets.GetStatus` | **9** | `status.go:47/56/60` 内部 3 处 + **UI 方法 6 处**（11.3-①） |
| `c.notify` | 9 | 见 §1.4 与 `control_coordinator.go:331` |
| `c.setLastError` | 10 | controller ×4、coordinator ×2、supervisor ×3、updater ×1 |
| `sys.ProbeTunAdapter` | 3 | `controller.go:1056/1159/1228` |
| 前端 `showAlert` | **67** | 全站 |
| 前端 `showConfirm` | 18 | 全站 |
| 前端 `showToast` | **7** | 非阻塞通道几乎是空的 |
| 前端 `console.error` 吞错 | **28** | 用户看不见、日志也没有 |
| 前端错误文案匹配 `.includes(` | 6 | `Overview.vue:233/267/289/291`、`Proxies.vue:696` |

### 11.3 新增发现（第一轮漏掉、同样"严/重/误报"）

| # | 位置 | 现象 | 影响 | 改法 |
|---|---|---|---|---|
| ① | `app.go:667/728/745/757/886/898` | **6 个 UI 方法直接调 `GetStatus`**：`GetDataDirInfo`、`CheckTunEnv`、`GetWintunVersion`、`GetComponentFileInfo`、`GetCoreVersion`、`GetRuntimeAssetStatus` | 打开「组件与库更新 / TUN / 关于 / 数据目录」任意一页 = 一次 61 MB 哈希 + 一次 `-v`；`GetDataDirInfo` 还额外跑 seed 的 `-v`（`app.go:681` → `GetSeedCatalogStatus` → `checkCoreByPath(seed)`） | 新增 `GetStatusCached()`（只读缓存/L0），UI 一律用它；深检只由后台触发 |
| ② | `app.go:901-907` | `RepairRuntimeAssets` 用 `RepairForce` 强制重写 6 个资产（≈95 MB） | **前端无任何调用方**（A1/A4 独立 grep 0 命中），属已暴露但无入口的死接口；一旦接按钮就是同步重写 + 无进度 | 接入口前先异步化 + 进度事件 + 可取消 |
| ③ | `core/tasks/manager.go:56-67, 98-109` | 任务失败只 `Emit(name+"-error")`，**不写日志** | core 更新 / geo 更新 / 订阅更新失败在日志里完全不存在 | 在 `-error` 分支加结构化日志（code/dur/attempt） |
| ④ | `control_coordinator.go:246-346, 362-407` | 一次失败的配置切换 = switchConfig 1 次 + ensureCore 1 次 + rollback 1 次 + rollback 内 ensureCore 1 次 ⇒ **4~6 次 EnsureReady**；自愈回滚用独立 45s ctx（:385） | 订阅更新失败可能让一次点击跑 90s+ | 回滚路径复用 L0 判定；回滚成功/失败都要有事件与日志 |
| ⑤ | `core/sys/helper_client_windows.go:36-43` | RPC 默认 deadline **10s** | `CheckHelperService` 的 Ping 就挂在 UI 链上（`CheckTunEnv`/`GetDataDirInfo`），服务假死会让设置页卡 10s | Ping 单独 800ms；UI 路径禁止 10s 级调用 |
| ⑥ | `app.go:656-660` | `RestartHelperService` = Stop → sleep 300ms → Start（同步） | 按钮同步阻塞，失败只回一句 err | 异步 + 进度 + 结果事件 |
| ⑦ | `Overview.vue:252-319` / `SettingsTun.vue:142-156` | **两套 TUN 开关**：前者"缺驱动→确认→装驱动→自动重试"，后者只有一句 missingDriverMsg | 同一台机器两种行为；修一处漏一处 | 抽 `useTunToggle()` 单一实现 |
| ⑧ | `Overview.vue:233` vs `:289` | 系统代理分支与 TUN 分支各自复制了一份 `includes('no active config')` 判定 | 判定逻辑复制粘贴，无法收敛成 code 表 | 统一 `code → 处理策略` 映射 |
| ⑨ | `Proxies.vue:696` | 连续三处 `msg.includes()` 串联：`DELAY_TEST_BUSY` / `已有测速任务` / `busy` | 任何含 busy 的错误都显示"已有测速任务" | 同上，改 code |
| ⑩ | 28 处 `console.error` | 见 11.5 清单 | 这些 catch 既不提示也不记录 → **全盲区** | 统一 `reportError(e, ctx)`：落日志 + 按 class 决定 UI |
| ⑪ | `app.go:910` | `GetProxyDelay(..., 8000)` | UI 测速最长 8s 无反馈 | 加超时提示 + 取消 |
| ⑫ | `log_dispatcher.go:63-65` + `App.vue:681-691` | core 日志**逐条** emit 到前端并逐条 push + 自动滚动 | 默认 `log-level=error` 尚可；用户切 info/debug 会把 UI 打满，并把 500 条环冲掉 | 前端批量合并（16ms 帧）+ 分级配额 |
| ⑬ | `check.go:144-155` | 已有 (size,mtime) 命中的 60s 内存版本缓存 | 证明"按 stat 缓存 + 落盘延长 TTL"与现有设计同源，风险低 | 直接把 TTL 提到 7 天并落盘 |
| ⑭ | `updater_orchestrator.go:278-312` | `EvaluateCoreComparison`：版本未知时**主动不提示**并自动关闭提示开关 | 这是本项目"严格但不打扰"的**正确样板** | 把同一范式推广到 §5 的所有 code |
| ⑮ | `main.go:56` → `app.go:204` → `Controller.Bootstrap` | 迁移 → 资产维护 → Bootstrap，全部在窗口出现前或紧邻 | 启动期任何一次重 IO 都直接表现为"双击没反应" | 全部后台化，UI 先出，`boot.phase` 上报进度 |

### 11.4 对 v1 的两处修正

1. **"每次 EnsureReady 都重拷 wintun"** → 更准确：只有 `requirementSatisfied` 为 false（某资产不 Ready）时才进入修复分支；一旦进入，wintun 因 `VersionProbeOK` 恒为 false 而**必然** `shouldCopy=true`。这正好解释"平时不拷、出事就拷、拷就失败"。
2. **19:00:33 的 tmp 文件** → 属**推断（B 级）**，不是既成事实。验证方式：落盘日志 + 复现一次，看 `asset.repair.*` 是否在 19:00:33 附近出现且带 `blockedBy`。

### 11.5 28 处吞错清单（全部要接入 `reportError`）

```
App.vue:566,643,653            store.ts:197,741
Connections.vue:300            Proxies.vue:581,645,700
Rules.vue:293                  Settings.vue:185,204,221
SettingsAbout.vue:405          SettingsDns.vue:224
SettingsLan.vue:240,269        SettingsNetwork.vue:322,339
SettingsTun.vue:130,173        SettingsUpdate.vue:81,82
Subscriptions.vue:272,376,398,474
UpdateTaskPanel.vue:34
```

### 11.6 未覆盖 / 待实测（诚实清单）

| 项 | 状态 | 补齐成本 |
|---|---|---|
| 19:00:33 到底谁写了 tmp | 推断 B | 落盘日志 + 1 次复现 |
| 启动到可交互的耗时分布 | 未测 | `boot.phase` 埋点（P0 顺带做） |
| helper `superviseCore` / `startCoreProcess` | 未读 | 30 min |
| downloader 重试/续传/代理 | 未逐行 | 2 h |
| 订阅 / 规则 / YAML 编辑器事务边界 | 只审调用点 | 2 h |
| 提权 UAC / UWP / 开机自启 | 抽查 50% | 1 h |
| 8 语言文案与 code 对齐 | 未审 | 0.5 h（P2） |
| System proxy WinINet 细节 | 未读 | 30 min |

### 11.7 由此新增的改造项（并入 §7）

| # | 文件:行 | 改法 | 优先级 |
|---|---|---|---|
| 31 | `app.go:667/728/745/757/886/898` | 新增 `GetStatusCached()`，UI 方法全部改走它（只读 + L0/L1 缓存） | **P0** |
| 32 | `core/tasks/manager.go:56-67,98-109` | `-error` 分支补结构化日志 | **P0** |
| 33 | `app.go:901-907` | `RepairRuntimeAssets` 无调用方：要么补入口并异步化，要么收敛接口 | **P1** |
| 34 | `Proxies.vue:696` | 去 `includes('busy')`，改 code | **P0** |
| 35 | `Overview.vue:252-319` / `SettingsTun.vue:142-156` | 合并为 `useTunToggle()` | **P1** |
| 36 | `app.go:681` | `GetDataDirInfo` 不再对 seed 跑进程探测，改用 manifest 版本 | **P0** |
| 37 | 前端 28 处 `console.error` | 统一 `reportError()`（日志 + class 路由） | **P0** |
| 38 | `App.vue:681-691` | core 日志批量合并 + 分级配额 | **P1** |
| 39 | `core/sys/helper_client_windows.go:36-43` | Ping 单独 800ms；UI 路径禁 10s 调用 | **P0** |
| 40 | `check.go:144-155` | 版本缓存 TTL 60s → 7 天 + 落盘 | **P1** |

### 11.8 一句话

> 第一版证明了"资产检查太重"；第二版补齐了**"重的不止资产"**——UI 方法本身就在跑深检，任务框架、配置回滚、吞错 catch 都在这条误报链上。

---

*本文档为方案，不含代码改动。落地时按 §7 编号逐条提交，每条独立可回滚。*
