# TUN「一直显示启动中」修复（B5）

> 触发问题：切换配置文件后开启 TUN，偶发永久停在「启动中」，无提示、无重试入口。
> 本批修复 3 个代码层缺陷 + 1 处文案防抖；不改错误分级原则（A 静默 / B toast / C 模态）。
> 验证：go build / go vet / go test 全绿；前端 vue-tsc + vite build 全绿；新增 8+3 个单测子用例。

## 0 现象与判据

前端文案只由状态推导（Overview.vue 的 consoleServiceTitleRaw）：
wantTun && !actualTun && isRunning → 「启动中」。所以「永久启动中」= 后端 ActualTun 一直 false，
而 ActualTun 只可能由真实适配器探测置 true，绝不从 desired.Tun 推导。

日志判据（data/logs/app-*.jsonl）：

- `TUN 适配器尚未就绪` → 某次等待超时后降级；
- `TUN 实际状态已校正` → 后台探测恢复过（只是慢）；
- `TUN 适配器在 ... 内未就绪: tun_adapter_not_ready` → 适配器存在但没 Up（本次修复的 D1）；
- `... tun_adapter_not_found` → 适配器根本没创建（内核/驱动侧问题，本批未覆盖）。

## 1 根因与修复

### D1 适配器选优把「Down 的残留适配器」排在「Up 的真实适配器」前面（core/sys/tun_win.go）

旧算法权重 `score*2 + Up`：名字精确但 Down = 8，名字带后缀且 Up = 7。
Windows 遇到重名会给新适配器加后缀（tun_win.go 注释自己就写了 GOCLASHZT 的例子），
于是上一个会话残留的 Down 适配器会永久遮蔽真实适配器 → 探测恒失败 → ActualTun 永远 false。

修复：**分阶段匹配**

- `rankTunAdapter`（L248）：名字精确 3 / 名字前缀 2 / 描述关键字 1 / 不匹配 0；
- 先只在名字匹配组内选优（组内 **Up 优先**，再比强度），只有完全没有名字匹配时才退到描述匹配
  （避免把其它 TUN 软件同为 "Meta Tunnel" 的适配器认成自己的）；
- `betterTunCandidate`（L179）注释写明「Up 优先于匹配强度」的理由。

### D2 配置切换把 TUN 恢复塞在同步事务里，且超时被静默当成功（core/appcore/control_coordinator.go）

旧行为：第 4 步起内核后等适配器最多 20s，第 6 步再等最多 20s；两次超时都只 logWarn，
事务照样返回成功。切配置走的是**无 deadline 的 app ctx**，所以是真的会卡满 ~40s，然后 ActualTun 仍为 false。

修复：

- L424：起内核时用 `withTunWaitBudget(ctx, 5s)` 限制 TUN 等待预算；
- L440：第 6 步改为**一次只读探测**，未命中则 `runAsyncReconcile("config-switch-tun-restore")` + `SyncState()`，
  事务立即返回，不把用户操作路径挂住。

### D3 「等不到」没有出口（core/appcore/controller.go + supervisor.go）

旧行为：等待失败后只剩 watchdog 每 5s 的单次探测；前端无超时降级，切配置期间 tunPending 也没置位，
所以 60s 看门狗（store.ts）对它一次都不触发 → 界面永久停在「启动中」。

修复：新增**静默升级阶梯** `escalateTunReadinessIfNeeded`（controller.go:1375），由 watchdog 调用（supervisor.go:160）：

| 阶段 | 阈值 | 动作 | 用户感知 |
|---|---|---|---|
| L1 | 15s | 后台 `boundedTunWait()`（5s 预算，成功即校正 + SyncState） | 无 |
| L2 | 40s | 后台 `ReconcileAsync("tun-not-ready-restart")` 重启一次内核 | 无 |
| L3 | 60s | `notify("warn", "TUN_NOT_READY", ...)` → 前端 **toast**（非模态） | 一条轻提示 |

TUN 关闭 / 已就绪 / 内核未以 TUN 运行 时 `resetTunLadder()` 清零，不会跨会话误升级。

### D4 注释承诺的 200ms 防抖其实没实现（frontend/src/components/Overview.vue）

`titleTimer` 只被 `clearTimeout`、从未赋值，watch 里直接赋值。
修复（L172-187）：只有「启动中/停止中/重启中」这类**瞬时文案**延迟 200ms 上屏，
终态（接管中/代理中/服务停止）立即生效并取消待触发定时器；卸载时清定时器。

## 2 对「无感自愈」的影响

| 项 | 影响 |
|---|---|
| 探测成本 | 不变（同一次 `GetAdaptersAddresses`，评分是内存比较，0 额外系统调用） |
| 正常开启 TUN | 不变（第一次探测就命中 Up 适配器，比原来更快退出等待） |
| 切配置 + TUN | 同步等待从 ~40s 上限降到 ~5s，其余交给后台收敛 |
| 失败路径 | 从「永久静默卡住」变成「15s/40s/60s 三级自愈，最后一条 toast」 |
| 文案 | 抖动不再闪，终态反馈不延迟 |

## 3 验证

    go build ./core/... ./cmd/... .        → 0
    go vet   ./core/... ./cmd/... .        → 0
    go test -count=1 ./core/... .          → 全 ok
    go test -run 'TestRankTunAdapter|TestBetterTunCandidate' ./core/sys/ → 8+3 子用例全 PASS
    vue-tsc --noEmit + pnpm -C frontend run build → 0（含 i18n lint）
    gofmt（改动文件）                       → 干净（controller.go 仅剩改动前既有的注释列表差异）

单测覆盖的关键回归：`Down 的 GOCLASHZ` 不得压过 `Up 的 GOCLASHZT`；
「device 为空时只看描述」；「同强度同为 Down 时保留先命中的候选」。

## 4 仍需真机确认

1. 切配置 + 开 TUN 连续 10 次，界面不应再出现 >10s 的「启动中」；
2. 若出现 `tun_adapter_not_found`，说明是内核/驱动侧没创建适配器，属另一条链路（下一步查 helper 的 wintun 会话）；
3. 观察日志是否出现 L1/L2 升级记录，确认阶梯真的在走（而不是一直探测失败）。

## 5 回滚说明

本批未单独建 .fixbackup 快照（改动集中在 5 个文件的具名函数/片段）。
回滚按函数级恢复：`rankTunAdapter`/`betterTunCandidate`/`ProbeTunAdapter`、
`waitForTunAdapter` 的 tunWaitBudgetKey、`escalateTunReadinessIfNeeded`/`boundedTunWait`/`resetTunLadder`、
`switchConfigLocked` 第 4/6 步、`Overview.vue` 的 `stableTitle` watch。
.fixbackup/20261116-b3/ 里的同名文件是 B3 之前的快照，整文件覆盖会连带回退 B3 的修复，不要直接用。