# backend-needs-3 · fix-3-proxy 需要的后端配合（本 Wave 未改任何 Go）

> 我已按"优先读结构化字段、回退兼容旧字符串"实现了前端侧双兼容，以下是**根治**项，供 Wave 2 fix-backend 决策。
> 现状备注：工作树里的 `v3_events.go:80` 与 `core/appcore/delay_manager.go` **已经是** `map[string]any{"ok":bool,"message":string}`（不是本代理改的），前端已按此优先读取。

## 1. `proxy-test-finished` 再加 `batchId`（根治 P1-E / C-06）

现状 payload：`{ok: bool, message: string}`。取消批次时 `ok` 仍为 true、"测速完成"，因此"停止 → 立刻重测"时旧批次的 finished 与正常结束**同形**。

- 建议：`TestAllProxiesWithOptions` 生成 `batchID`（自增或 uuid），随 `proxy-test-start` / `proxy-delay-update` / `proxy-test-finished` 一起下发。
- 前端现在用"本批还没收到过 start 就收到成功通知 ⇒ 是上一批"的启发式（依赖 `cancelBatchAndWait` 的等待顺序）。加了 batchId 后，前端把 `testBatchStarted` 判据换成 `ev.batchId === myBatchId` 即可彻底消除残余窗口（改动只在 `Proxies.vue` 的 `readTestFinish`/监听器，约 10 行）。

## 2. `GetInitialData` 失败分支加显式字段（根治 P-16）

`app.go:317-347` 在 `GetOfflineData` / `clash.GetInitialData` 失败时返回 `groups:{}`，与"配置里真的没有分组"不可区分。

- 建议：失败分支加 `"loadError": true`（可选 `"error": err.Error()`）。
- 前端当前判据（`isOffline===true && isRunning` 或「有 activeConfig 却 0 分组」）是启发式，纯规则配置会误报；后端加字段后，`Proxies.vue` 的 `applyInitialData` 里一行改成 `data.loadError === true` 即可。

## 3. `ConnectionDTO.Chains` 保证非 nil（P-18 双保险）

`toConnectionDTO`（`core/appcore/connections.go`）里建议 `append([]string{}, v.Chains...)`，避免内核省略字段时 DTO 序列化成 `null`。前端已兜 `|| []`，此处只是消除数据契约噪声。

## 4. 可选：`proxy-state-sync` 带 `topologyVersion`（P1-G / P-09 的彻底方案）

当前前端靠"loadData 后重建冻结快照"自愈；若事件里带拓扑版本号，前端可以在版本不匹配时直接作废冻结快照，逻辑更直白（当前做法已足够，不阻塞）。

## 5. 注意（避免事件被 wails cancel）

`v3_events.go` 的 `RegisterEvent[T]` 类型必须与**所有**发射点一致。本次 `proxy-test-finished` 从 `string` 改成 `map[string]any` 时，两侧已同步；若回滚 payload，请同时回滚注册类型，否则事件会被丢弃（审计 06-contract §7-Q2）。
