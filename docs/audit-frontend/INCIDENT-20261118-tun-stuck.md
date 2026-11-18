# 事故复盘：TUN 开启后界面卡在「启动中...」46 秒（2026-11-18）

> 来源：用户反馈「有时候这个状态卡住很久无反应，取消再点按就好了」+ 截图（服务状态=启动中...，TUN 卡片=已挂载）。
> 结论：不是"启动慢"，是一个**卡死的中间态** —— 三个缺陷叠加。证据来自用户本机日志 `D:\GoclashZ\data\logs\app-20261118.jsonl`。

---

## 1 证据时间线（用户本机，2026-11-18）

| 时间 | 日志 | 含义 |
|---|---|---|
| 23:12:19 | 读取内核代理组失败: 127.0.0.1:9090 拒绝连接 | 内核刚重启/未起，前一轮内核还在退出 |
| 23:13:19 | Get connections 失败: 9090 拒绝连接 | 同上，**内核反复启动** |
| 23:14:08 | Helper 未在 800ms 内就绪 → 后台唤醒 → 就绪 | 用户点开 TUN，进入启动流程 |
| **23:14:30** | `TUN 适配器在 20s 内未就绪: tun_adapter_not_ready: device="本地连接" desc="Wintun Userspace Tunnel" ifIndex=21 up=false routes=0` | **关键**：命中的不是我们的适配器 |
| 23:14:30 | 控制动作遇到瞬时失败，保留用户意图并转入后台重试 | 因为错误码是 not_ready，被归为"可等" |
| 23:14:30–23:14:50 | 每 5s 重复 `Reconcile(watchdog) failed: tun_adapter_not_ready ... device="本地连接"` | **空转 20s 没有换策略** |
| 23:14:50 | TUN 适配器 15s 内未就绪，开始后台有界等待（L1） | 静默阶梯终于触发 |
| **23:14:53** | 内核已通过 Helper 服务启动 (TUN) | L1 触发内核重启（第二次启动） |
| **23:14:54** | `TUN 适配器已就绪: device="GOCLASHZ" desc="Meta Tunnel" ifIndex=43 up=true routes=10` | 这次终于建出**本程序**的适配器 |

**点击 → 就绪 = 46 秒**，期间界面恒定「启动中...」，无原因、无入口。

对照历史成功记录（11-16 / 11-17）：正常都是 `device="GOCLASHZ" desc="Meta Tunnel"`，up=true，1 秒内就绪。

---

## 2 根因（三个缺陷叠加）

### 缺陷 A（后端，触发剂）：探测把「别人的 Wintun 适配器」认成了自己的

`core/sys/tun_win.go` 的匹配顺序是：设备名精确 → 设备名前缀 → **描述关键字**。描述关键字里除了强信号 `"meta tunnel"`（mihomo/sing-tun 固定写死），还留了泛化的 `"wintun"`。

本次系统里残留了一个无关适配器：名字 `本地连接`、描述 `Wintun Userspace Tunnel`、Down、routes=0。名字匹配组为空 → 落到描述组 → 命中 `"wintun"` → 被当成"我们的 TUN 还没起来"，返回 **`ErrTunAdapterNotReady`**。

这个错误的语义是"等一下就好"，于是：
- `isTransientControlError` 判为瞬时 → 保留用户意图、后台重试（`control_coordinator.go:175`）；
- watchdog 每 5s 只重复探测同一个错误，**永远不会推进到"内核没建出设备 → 重启内核"**；
- 只能靠 15s 的 L1 静默阶梯兜底，多花 20s。

反向风险更严重：万一那个外来适配器是 **Up** 的，探测会直接成功 → `ActualTun=true` **假阳性**（界面显示已接管，实际流量没走 TUN）。

### 缺陷 B（后端，放大器）：适配器"缺失"与"未就绪"没有区分处理

- `ErrTunAdapterNotFound`（设备不存在）意味着内核这次根本没建出来 → 应当尽快重启内核；
- `ErrTunAdapterNotReady`（设备存在但 Down）才是"再等等"。

两者现在都被当成"可等"，所以缺设备的情况要等满 15s 阶梯。

### 缺陷 C（前端，用户可见的那一半）：启动态没有超时降级

`Overview.vue` 的状态计算：`wantTun && !actualTun && (isRunning || tunPending)` → 恒返回「启动中...」，**没有任何超时分支**，也没有"重试/取消"入口。

更糟的是上一轮「提示策略」返工里，我把后端 L3 的 `TUN_NOT_READY`（60s 自愈失败的提示）改成了静默 —— 等于把这个窗口期唯一的反馈也删掉了。

---

## 3 为什么"取消再点一次就好了"

取消会把意图切回关闭 → 内核被停（`stop-core`）、`ActualTun` 清零；再点开启时内核是**全新启动**，上一次内核持有的 Wintun 会话/适配器句柄已经释放，所以这次能正常创建 `GOCLASHZ` 适配器。

也就是说：真正需要的是"重启内核"这个动作，而缺陷 A 让它没有被及时触发 —— 用户手动取消+重开等于亲手做了这件事。

---

## 4 已修（本轮）

| 项 | 改动 | 文件 |
|---|---|---|
| A | 描述匹配**只接受强信号** `Meta Tunnel`；泛化 `Wintun` 命中一律返回 `ErrTunAdapterNotFound`，并在错误里附上被忽略的外来适配器（保留可诊断性）。同时消除了 Up 状态下的假阳性风险 | core/sys/tun_win.go |
| C-1 | 启动态加 25s 超时降级：超过后显示「启动尚未完成，正在后台自动重试 · 点击立即重试」，点击会把当前意图幂等重放一次（不反转） | Overview.vue（+ 8 语言新 key） |
| C-2 | `TUN_NOT_READY` 从 silent 恢复为 **toast**（自愈失败需要知情，属于"重试无用/需要用户处理"，不是瞬时可自愈） | store.ts |

## 4.5 「最快最无感」的快速自愈（本轮已实现）

目标：把 46s 压到 ~10s，且**全程不打扰用户**（开机自启尤其不能弹东西）。

| 改动 | 位置 | 效果 |
|---|---|---|
| 探测结论二分：`tunProbeMissing` 记录"设备根本不存在" | controller.go | 「缺失」与「存在但 Down」从此走不同策略 |
| 同步等待遇到缺失**提前收尾**（5s 而非硬等 20s） | `waitForTunAdapter` | 省掉 15s 的无效等待 |
| 静默阶梯新增"缺失档"：5s 就重启内核（一次） | `escalateTunReadinessIfNeeded` + `restartCoreForMissingTun` | 不再等 15s 有界等待 / 40s L2 |
| 重启只做一次（`tunLadderStage=2` 闸门） | 同上 | 不会重启风暴；失败后仍有 L3 提示兜底 |

预期时间线（同一次事故重演）：点击 → 5s 判定"设备缺失" → 下一次 watchdog tick（≤5s）→ 重启内核 → ~4s 就绪 ≈ **10s 内**，且没有任何弹窗/提示。

**回归测试**（`core/sys/tun_win_test.go`，7 个用例，含事故复现）：
- 事故复现：只有 `本地连接 / Wintun Userspace Tunnel / Down` → 必须返回 `ErrTunAdapterNotFound`（而不是 `not_ready`）；
- 假阳性防护：外来 Wintun **即使 Up** 也不能判 `ActualTun=true`；
- 自家适配器 Down → `not_ready`（可等）；精确 Down + 前缀 Up → 选 Up；强描述 Meta Tunnel 可兜底；空列表 → `not_found`。

> 验证方式（需实机）：开机自启一次，日志应出现 `TUN 适配器缺失 5s（内核未创建设备），跳过有界等待直接重启内核`，
> 且从内核启动到 `TUN 适配器已就绪` 应 ≤10s。

## 5 验收方式（下次复现时）

1. 日志里应看到 `tun_adapter_not_found ...（忽略不相关的 Wintun 适配器: device="本地连接" ...）` 而不是 `not_ready`；
2. 若仍超 25s，界面会出现「启动尚未完成 · 点击立即重试」，而不是一直「启动中...」；
3. 60s 节点上应看到一条 toast（TUN 未就绪，已自动重试）。
---

## 6 对开机自启的影响（用户提问）

**会**，而且是同一条管线：

- `startupWithOS` 只是"用任务计划把程序拉起来"；真正会碰 TUN 的是 `restoreOnStartup`；
- `controller.go:360`：`opts.IsStartupLaunch && Behavior.RestoreOnStartup` → `runStartupRestoreJob` → `Supervisor.Reconcile(ctx, "startup-restore")`——
  和用户手点 TUN 走的是**同一套** EnsureCore / waitForTunAdapter / 静默阶梯；
- 所以缺陷 A/B/C 在开机恢复 TUN 时同样成立：若这次内核没建出网卡，恢复会卡在"启动中"，且因为它是后台启动，用户很可能以为"已经好了"。

开机场景与手动点击的差别（决定了优化方向）：
1. **没有"上一次内核残留会话"**：全新开机时 46s 那种多半来自"运行中重启内核 + 立刻点 TUN"；开机更常见的是 helper 冷启动（日志实测：800ms 快 ping 失败 → 转后台 SCM 拉起）+ Windows 网络栈初始化慢；
2. **开机最不该弹任何东西**：所以策略必须是"静默 + 快速自愈"，而不是"弹框让用户处理"——这也正是本轮的方向（自愈动作全程无 UI，只有 60s 兜底才 toast）。

因此本轮改动对开机自启是**双收益**：判定更快（少 15s 无效等待）+ 自愈更早（5s 就重启内核）。

### 6.1 helper 预热（已实现）

`Controller.Start` 末尾新增：**当前 TUN 意图为真**时，后台预热 helper（`EnsureHelperReadyWithContext(ctx, "startup-warmup")`，20s 上限）。

- 位置：放在 "是否启动恢复" 分支**之后** —— 非恢复启动的 desired 已被清空，不会为一个可能用不到的功能常驻 SYSTEM 特权服务；
- 收益：把 helper 冷启动的 1~2s 从"恢复 TUN 的关键路径"挪到后台；开机恢复时 restore job 本身有 2s 退避，正好覆盖；
- 日志：成功时输出 `helper 已预热（1.2s），TUN 首次操作免去冷启动等待`；失败只记 info，不影响启动；
- 手动点 TUN 本来就不阻塞在 helper 上（`prepareTunEnable` 800ms 快 ping 失败就转后台唤醒并继续），所以这次预热只针对"开机恢复"这一条最在意的路径。

叠加后的预期：**开机自启 → TUN 就绪 ≈ 10s 内**（判定 5s + 重启内核 ~4s，helper 已在后台就绪），全程零提示。
