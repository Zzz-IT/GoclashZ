# 后端需求 · fix-1-state

> 按规则 3：本轮不改 Go。以下三条是前端侧**已经做了双兼容/单侧加固**、但根治需要后端配合的项。
> 每条都标了「前端现在能容忍什么」与「后端改了会怎样」，便于排期。

## B-1（对应 S-02 / P1-A）AppBehavior 缺字段级补丁语义 → 多写者丢更新

**现状**：`SaveAppBehavior` 是整对象覆盖（app.go 注释：保存后 SyncState）。前端两个入口
（store.ts:134-145 侧栏明暗、Settings.vue saveBehavior）都只能「读整份 → 改自己的字段 → 写整份」，
两步之间有窗口 → 后写者把先写者的字段顶回旧值。

**前端已做**：store 侧保持 GET→只改 themeMode→SAVE，并把「两边都必须 diff-merge」写成约定注释；
设置页侧由 fix-5 改成「GET 最新 + 只合并本页脏字段」。两个入口都不再盲写本地快照。

**后端希望（任选其一，按成本排序）**：
1. 新增 `SaveThemeMode(mode string) error`（只写主题），store.ts 那 4 行整段替换即可 —— 最小、风险最低。
2. `SaveAppBehavior` 改 PATCH 语义：只接受显式传入的键（`map[string]any`），未传字段保持后端现值。
3. 给 AppBehavior 加版本号（`revision`/`etag`），保存时带上次读到的版本，冲突返回错误让前端重读重试。

**收益**：「改了个设置，明暗自己翻回去」这条用户已上报的症状可以彻底关闭（现在只是把窗口缩到最小）。
**注意**：改接口要同步改 bindings 与 api.ts 的 108 方法面（新增方法不会冲突，见 S-10 的 dev 断言）。

## B-2（对应 C-20 / F-A4-14 遗留）`OutboundIPResult` 缺 `code` 字段

**现状**：后端在路由切换时返回 `OutboundIPResult{Mode: currentRoute, Message: "路由切换中: expected=… current=…"}`
（core/appcore/outbound_ip.go:202-206），前端只能靠中文子串判断（utils/errors.ts:127-129
`isRouteSwitchingMessage` 匹配「路由切换中」）。后端改文案 = 前端静默降级。

**前端已做**：① `ROUTE_SWITCHING` 通知已在 NOTIFICATION_UI 里转 silent，内部诊断串不再进 toast；
② 通知文案通道加了 code→i18n 映射 + 缺键回落后端原文，后端补 code 后前端只加一行映射。

**后端希望**：`OutboundIPResult` 增 `Code string`（json: `code`），路由切换分支返回 `Code: "ROUTE_SWITCHING"`；
`GetOutboundIPForRoute` 的其它失败分支同样带 code。前端随后把 `isRouteSwitchingMessage` 改成
「优先读 code、回退文案匹配」（双兼容），文案彻底可翻译。

## B-3（对应 S-03 的自愈尾巴）失败回滚后没有周期性 state 广播

**现状**：已确证 supervisor 的 5s watchdog（core/appcore/supervisor.go:78/130）**只在 reconcile 内部状态
变化时** SyncState，没有无条件心跳。因此「命令失败 → 后端 CAS 回滚 desired」这次广播若丢失，
前端要等到下一次状态变化才知道真实值。

**前端已做**：① overlay 带发起 revision，收到更大 revision 时丢弃本地 intent（S-03）；
② 调用方（fix-2 的 H-14）失败后主动 `GetAppState()` 重拉一次。

**后端希望（低优先，二选一）**：a) 控制命令失败路径**无条件** `SyncState()` 一次（改动最小）；
b) 提供低频状态心跳（如 30s），让前端有最终一致的自愈路径。
