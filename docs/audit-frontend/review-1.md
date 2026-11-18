# 独立评审 · 编号 1（store / App / utils / styles / locales / Go 契约）

> 评审代理：reviewer-core（Wave 3 独立评审，**只读**，未改任何源码）
> 日期：2026-11-18
> 基线：HEAD `840837f` + 工作区未提交改动（`git status` 59 个改动文件）
> 范围（本代理负责）：`frontend/src/store.ts`、`App.vue`、`api.ts`、`trafficWaveState.ts`、`main.ts`、
> `frontend/src/utils/**`（含删除的 normalize.ts、新增 alertError/clipboard/storage）、`frontend/src/styles/**`、
> `style.css`、`locales/**`（8 语言）、`scripts/check-i18n.mjs`、
> Go：`app.go`、`services_v3_*.go`、`v3_events.go`、`core/appcore/delay_manager.go`、`frontend/bindings/goclashz/coreservice.ts`
> 方法：逐文件 `git diff -U5/-U8` 读改后完整上下文；grep/AST 对账被删与被增符号、i18n key、绑定量；
> 用 Node 复算 wails binding ID / locale key 集合 / CSS 变量定义；跑了一次 `node scripts/check-i18n.mjs`（只读校验）。
> **未跑** build / vue-tsc / go test（按指令，由主控统一跑）。

## 0. 结论

| 级别 | 数量 |
|---|---|
| BLOCKER | **0** |
| MAJOR | **1** |
| MINOR | 13 |
| OK（关键不变量逐条确认） | 见 §3 |

唯一 MAJOR 是一个「删了全局 keyframes、漏了一个消费者」的视觉回归（U-16 半成品），不涉及功能崩溃。
本批文件的**功能路径未发现回归**：store 模态管理器自洽、事件订阅 21/21 与 15/15 全配对、
i18n 删除的 103 个键全仓 0 引用、新增 23 键 8 语言齐备且占位符一致、
Go 侧 `proxy-test-finished` 注册类型与两处 emit 完全一致、`ReorderSubs` 从 app.go 到 mapping(109)/bindings 全链路一致（binding ID 已复算）。

---

## 1. 问题清单

| 级别 | 位置 | 问题 | 影响 | 建议 |
|---|---|---|---|---|
| **MAJOR** | `frontend/src/components/Overview.vue:386` + `frontend/src/style.css`（U-16 删除块，HEAD `style.css:723`） | `.orb-glow` 仍写 `animation: pulse 2s infinite`，但本轮把**唯一的全局** `@keyframes pulse`（HEAD `style.css:711/723` 的 fadeIn/pulse 两块）删掉了。现存脉冲定义只有 `TrafficCard.vue:362` 的 `<style scoped>`（SFC scoped 会把 keyframes 改名成哈希，不产生全局名），Overview.vue 自己的 scoped 块只定义了 `spin-centered`/`scan-dash`。 | 首页「运行中」状态点的**呼吸扩散晕圈不再动**：`.orb-glow` 停在 opacity 1 / scale 1 的静态径向渐变（注释 378-385 行描述的「脉冲扩散」失效）。U-16 报告里「消费方各自 scoped 块里有同名定义」的结论漏了 Overview.vue。 | 在 Overview.vue 的 scoped 样式里补回 `@keyframes pulse { 0%{transform:scale(1);opacity:.5} 100%{transform:scale(2.5);opacity:0} }`（即 HEAD style.css:723-733 原值）。不建议把全局块加回去（会重新制造重名覆盖问题）。 |
| MINOR | `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts:68` vs `v3_events.go:80` | 事件类型表仍写 `"proxy-test-finished": string`，而 Go 注册已改 `RegisterEvent[map[string]any]` 且两处 emit 都是 map。文件 mtime：`v3_events.go` 11-18 20:22:02、`eventdata.d.ts` 11-16 20:27（`coreservice.ts` 11-18 20:22:44 已重生成）→ 事件表未跟着重生成。 | 运行时无影响（注册类型与 emit 一致；`Proxies.vue:909` 把 data 标成 `unknown` 自解析）。但工作区里 TS 契约与后端相反，后续若有人用 typed `Events.On` 会按 string 写。bindings 是 .gitignore 生成物，提交面不受影响。 | 下次 `wails3 generate bindings` 会自愈；建议主控在验收时补跑一次生成，确认 eventdata 变 `{ [_ in string]?: any } \| null`。 |
| MINOR | `frontend/src/api.ts:4,6` | 注释仍是「CoreService（62 个）」「108 个调用点」，新增 ReorderSubs 后应为 63 / 109（mapping/go 侧已全部改成 109）。 | 纯文档漂移，容易让下一个人按 62/108 复核绑定数。 | 改成 63 / 109（与 `services_v3_mapping.go:8-9` 对齐）。 |
| MINOR | `services_v3_mapping.go:10` | 文件头第 8-9 行已写「109 个绑定方法」，紧接着第 10 行仍写「不计入 108」——同一段注释自相矛盾。 | 读注释的人会以为还是 108，go test 的 wantTotal 是 109。 | 「计入 108」→「计入 109」。 |
| MINOR | `frontend/src/store.ts:1031` | 注释「main.ts 与 App.vue onMounted 都会调用」已与现状相反：App.vue 的调用本轮被删（`App.vue:945-946` 注释写明唯一调用点是 main.ts）。 | 误导后来者以为 App 侧还在初始化；实际上 in-flight promise 的必要性只剩「防并发调用」这一条。 | 注释改为「main.ts 是唯一调用点；仍保留 in-flight promise 以防将来出现第二个调用者」。 |
| MINOR | `frontend/src/App.vue:277` | `initStore` 仍留在 store 的 import 列表里，全文只出现在 945 行注释中（grep：import 1 处 + 注释 1 处）。 | 死导入（vue-tsc 全绿说明 noUnusedLocals 未开）；暗示这里还有初始化调用。 | 从 import 列表删掉 `initStore`。 |
| MINOR | `frontend/src/styles/tokens.css:45` | 注释「本 Wave 还没有消费者，保留给 Wave 3 把组件里写死的 999px 换成它」，但本轮已经铺开 8 处 `var(--ui-radius-full)`（`style.css:1012`、`DeviationStrip.vue:114`、`TrafficCard.vue:255`、`SettingsDiagnostics.vue:295/329`、`SettingsTheme.vue:858/912/984`）。 | 矛盾注释：读者会以为该 token 没人用而删掉它。 | 注释改为「已由 R-B 铺开（8 处），后续新药丸直接用它」。 |
| MINOR | `frontend/src/utils/theme.ts:455-456`（+ `:218` `LEGACY_THEME_KEY`） | `cacheTheme` 每次双写旧键 `goclashz_theme`，注释称「仍有启动路径读它」；实际全仓 grep（`frontend/src` 全部 ts/vue/css + scripts）**没有任何读取方**——唯一读点已被 `store.ts:72` 换成 `readCachedThemeModeLabel()`。 | 死写：每次主题应用多一次 localStorage 写；注释与代码相反，U-08「单一写者/单一真源」实际停留在「写两份、读一份」。 | 二选一：①删掉镜像写与 `LEGACY_THEME_KEY`（最干净，本版本已无旧版回滚需求）；②保留则把注释改成「仅兼容旧版本回滚，当前无读取方」。 |
| MINOR | `frontend/src/App.vue:440` vs `:446` | 注释 A 说「H-06：warning 死键删掉」，代码 `logLevels` 仍保留 `warning: 2`；注释 B 解释「必须接受它，否则 rank 回落 info」也不成立——`rankOf()`（`:457-459`）先跑 `normalizeLogType()`，`'warning'→'warn'`，所以 `logLevels.warning` 永远索引不到。 | 双重矛盾注释 + 一个真死键；后来人删掉它会以为会坏，留下它又以为在生效。 | 删 `warning: 2`，把两段注释合并成一句：「warning 由 normalizeLogType 归一成 warn，故表里只列 5 档」。 |
| MINOR | `frontend/src/store.ts:881` | `refreshOutboundIP` 里 `localStorage.setItem('goclashz_outboundIP', …)` 仍是裸调用，且位于成功分支的 `try` 内。写失败（隐私模式/配额满）会落进 `:902` 的 catch，被当成一次检测失败：刚写入的新 IP 立刻被 `outboundIPStale = true` 标成「上一次检测的结果」（IP 卡出现过期提示）；silent 请求则整段后续逻辑被跳过。C-11 只包了 updateStateFromBackend 的 3 处，fix-1 报告已知此处但未改。 | 低概率（需 localStorage 不可写）；一旦发生是「刚测到 IP 却提示已过期」。 | 改用同一个 `writeLocalStorage()`，让缓存失败与网络失败解耦。 |
| MINOR | `frontend/src/store.ts:1055-1062` | 新增的 `disposeStore()` 只注销 Events 订阅，不清 `startPendingWatchdog` 的 `window.setInterval`（`:1295`）、`routeSettleTimer`（`:791`）、`delayTimers`。注释自称是「HMR/测试边界」的出口。 | 当前无调用者，不算 bug；但作为宣称的 teardown 出口是半成品，将来接 HMR 会留一个永不停止的 5s 定时器 + 写已卸载组件状态。 | 要么补齐清理（并让 `pendingWatchdogStarted=false`），要么在注释里明确「不含定时器，勿用于 HMR」。 |
| MINOR | `frontend/src/utils/storage.ts`（新）vs `store.ts:52/60`（新）vs `theme.ts:222/230`（新） | 同一轮里三处各自新增了**同构**的 localStorage try/catch 包装：R-1 抽了公共 `utils/storage.ts`（Proxies/Connections/Rules 已换），fix-1/fix-7 又在 store.ts、theme.ts 各复制一份私有版。handoff-r1 已登记「未合并」。 | 维护成本：三份实现语义已一致，但改策略要改三处；也是本轮「收敛重复」目标的反例。 | Wave 4 统一：store.ts/theme.ts 改 import `utils/storage.ts`（theme.ts 在 utils 内部，无环；store→utils/storage 无环）。 |
| MINOR | `frontend/src/utils/theme.ts:298` | `shadeHex()` 是导出函数，唯一调用点（applyTheme 里算 `--ui-accent-hover`）随 U-12 一起删掉，现在全仓 0 调用（导出态，TS 不会报未使用）。 | 新产生的死导出。 | 删除，或标注「保留给预设包做色阶用」并说明当前无调用。 |
| MINOR | `frontend/src/App.vue:1117` | `console.log("App update download started...")`（HEAD 已存在，非本轮引入）。 | 生产日志噪声。 | 随手改成 `reportError` 或删除；不阻塞。 |

---

## 2. 关键判断依据（供复核）

### 2.1 唯一 MAJOR 的证据链
- `git show HEAD:frontend/src/style.css` → `711:@keyframes fadeIn`、`723:@keyframes pulse`；本轮 diff 里两块被整段删除，替换成 `style.css:224-227` 的说明注释。
- 现存 keyframes：`SettingsLan.vue:408`、`SettingsNetwork.vue:495`（同文件 scoped，fadeIn）、`TrafficCard.vue:362`（同文件 scoped，pulse）。四个文件 `<style scoped>` 均已确认（`Select-String "<style"`）。
- `Overview.vue:386` 使用 `pulse`，同文件 `@keyframes` 只有 `spin-centered`/`scan-dash`（`:394-395`）→ 无全局/局部定义可命中。
- 触发条件：`v-if="consoleServiceOn"`（`:10`），即内核运行中（`consoleServiceOnRaw = desiredActive || actualActive`），首页常态可见。

### 2.2 i18n 删除键的独立复核（不依赖 fix-i18n 报告）
- 用「缩进路径解析」分别展开 HEAD 与工作区的 `zh-CN.ts`：797 → 717 个叶子，**删除 103 个**（含报告未列的 `settings.behavior.glassTintNeutral`，属 Wave 1 遗留）、新增 23 个（与报告逐字一致）。
- 对 103 个被删键做全仓字面量扫描（289 个文件：`frontend/src` 排除 locales、`scripts`、所有 `*.go`）：**0 命中**。
- 动态拼接风险单独查：全仓只有 `localized('deviation.field'|'deviation.reason'|'deviation.severity', x)` 三处动态取值（DeviationStrip.vue:62-66），其前缀族键都在；没有 `t(\`…${x}\`)` 形式的无前缀动态键；Go 侧对 `"tray.*"/"common.*"/"subs.*"` 等字面量 **0 引用**（托盘文案是 Go 自带中文）。
- 新增 23 键：逐个 grep 至少 1 个代码引用（`deviation.severity.*` 经 localized 前缀命中）；8 语言里 5 个带占位符的键（`minutesAgo/hoursAgo/daysAgo`=`{n}`，`checkFailed/saveReloadFailed`=`{error}`）**占位符全部一致**。
- 8 个语言包：752 个路径（717 叶子 + 35 命名空间）完全一致、**0 重复键**、`tray` 命名空间 8 文件均已移除；`node scripts/check-i18n.mjs` → `codeKeys=661 baseKeys=752 baseLeaves=717`，**exit 0**。
- `check-i18n.mjs` 新判定可信度抽查：`addLiteral` 只对「点分命名空间字面量」做前缀族（单段命名空间被排除，正好挡住 manifest.ts 里 'settings'/'tray' 这类普通词）；实测会被当成前缀族的字面量只有 `deviation.field/reason/severity`、`diag.note.`、`updateTask.`、`notify.` 等真实前缀，**没有把被删的 common.*/subs.*/tray.* 洗白**。

### 2.3 Go 侧契约
- `proxy-test-finished`：全仓 emit 点只有 `delay_manager.go:463`（busy）与 `:498`（defer），都是 `map[string]any{"ok":…,"message":…}`；`v3_events.go:80` `RegisterEvent[map[string]any]`——类型逐字节相同（map[string]interface{} ≡ map[string]any），不会被 wails cancel。前端 `Proxies.vue:750-769` 按「结构化优先、string 回退」双兼容。
- `finishOK` 覆盖：EnsureDelayCore 失败（:514）、拓扑失败（:522）、无有效节点（:535）三处置 false；cancel 路径仍是 ok=true+"测速完成"（与旧行为一致，属已知 C-06/batchId 遗留，不属本批判定）。
- `ReorderSubs`：`app.go:835-872`（排序语义：只排已知 id、未知尾随、重复 id 取首次、空 ids 直接返回）→ `services_v3_generated.go:307` → `services_v3_mapping.go` 登记 → 静态统计 **109 = App31 + Core63 + Update15**，生成文件方法集合与 mapping 集合逐项相等（无漏/无多/无错服务）。
- binding 真伪复核：`hash.Fnv = fnv32a("main." + 类型 + "." + 方法)`（wails v3.1.0 `internal/generator/collect/service.go` + `internal/hash/fnv.go`）。我按该算法复算：`main.CoreService.ReorderSubs` → **3022631746**，与 `coreservice.ts:176 $Call.ByID(3022631746, ids)` 一致；抽查 `RepairRuntimeAssets`(984256296)、`ResetTrafficTotals`(23055341) 同样吻合 → 该 binding 不是瞎编的 ID。109 个 binding ID 全仓无重复。

### 2.4 主题/圆角 token
- 单一写者：`goclashz_theme_v2` 唯一读写都在 theme.ts（`cacheTheme/readCachedTheme`）；`store.ts:108-123` 已不再写旧键；全仓 grep `goclashz_theme` 只剩注释与常量定义（残留镜像写见 MINOR #8）。
- `readCachedThemeModeLabel()` 返回 `'light'|'dark'`（`resolveIsDark` 解析 system 的 prefers-color-scheme），与 `globalState.theme` 的类型/比较点兼容；首帧 `globalState.theme` 与 `themeState.isDark` 同源，U-08 目标达成。
- `cacheTheme → resolveIsDark → matchMedia`：仅在浏览器环境调用，theme.ts 无 import（无循环依赖链）。
- 圆角：`RADIUS_VALUES.soft = {sm:8px, md:10px, lg:12px}` 与 tokens.css 默认值一致 → **默认主题下 90 处 token 化的计算值与改前逐像素相同**；`sharp/round` 才会跟随设置变化（设计意图）。`--ui-radius-full` 由 tokens 兜底 999px、theme 不覆盖。
- 未定义变量审计：`frontend/src` 内 `var(--x)` 共 74 个不重名，定义源（`*: --x:` + JS `setProperty`）68 个；缺的 7 个（`--border/--bg-elevated/--red-text/--green-text/--yellow-text/--text-dim/--accent-rgb`）**HEAD 与现在出现次数完全相同**（4 个文件逐一比对），全部是历史遗留，本轮没有新增未定义 token。
- 90 处 `border-radius` token 化：87 处能与被删的 px 值一一配对且数值等价（8/10/12/999）；另 3 处是**新增**声明（DeviationStrip.vue:114、TrafficCard.vue:255 的药丸、surfaces.css:21 的 group 圆角——后者改前由合并规则继承同值），无「50%→sm」式误改（SettingsTun 的 50% 保留在原处）。

---

## 3. 已确认 OK 清单（关键不变量 + 证据）

| # | 检查项 | 结论与证据 |
|---|---|---|
| 1 | 模态管理器自洽（S-07 删 legacy 后） | `globalState.modal` 写点只有 `presentModal:574-583/settleModal:597-618/confirmModal:667/cancelModal:678`；全仓 grep 组件侧 0 写。`presentModal` 同时置 `activeModal`+`show=true`，`settleModal` 同时清两者 → 「activeModal 非空 ⟺ show」成立。`onConfirm/onCancel` 结算时置 null，全仓 0 处读这两字段（grep `modal.onConfirm` 仅 store 写入 + 注释）→ 清空安全。 |
| 2 | 队列/await 不悬挂 | `enqueueModal` 只在 `activeModal` 非空时入队；`settleModal` 对 alert 一律 resolve(true)、confirm 按 ok；`showNextModal` 延时出队；`showAlert` 重复去重逻辑未变。删掉的 `reconcileModalDisplacement` 不可达（无外部写者）。 |
| 3 | 看门狗 S-01 | `store.ts:1305-1308`：`modal.show` 为真时每 tick 顺延起点、不计时；模态结算后重新起算 60s；强制复位仍调 `settleTunIntent/settleSystemProxyIntent`（`:1320-1321`）→ overlay 不会残留。 |
| 4 | overlay revision S-03 | `systemProxySince/tunSince` 在 set*Intent 记录 `controlRevision`；`settleStaleControlOverlay` 用**严格大于**（revision 相等=后端无新结论，overlay 仍有效）；调用点在 `syncVisibleControls()` 之前；仅 store 写 `desired*`（grep：Overview/useTunToggle 只 set/settle overlay，`:135` 注释已说明 H-14 移除了直写）。 |
| 5 | localStorage 包装 S-05/C-11 | store.ts 模块级 5 处读全部走 `readLocalStorage`（`:69/72/73/74/77`）；updateStateFromBackend 3 处写走 `writeLocalStorage`（`:389/434/438`）；模块顶层不再可能因 localStorage 抛错而白屏。 |
| 6 | initStore 幂等 S-06 | `storeInitPromise` 先赋值/先返回（`:1035,1051`）；`storeInited` 在 setupStore 全部成功后置真；失败时 `eventUnsubs.splice(0).forEach(unsub)` 回滚订阅并在 finally 清 promise → 可干净重试。main.ts:12 是唯一调用点（App.vue 已删）。 |
| 7 | Events.On/unsub 配对 | store.ts：`Events.On` **21** 处 = `eventUnsubs.push` **21** 处（脚本计数）；App.vue：`Events.On` **15** 处 = 15 个 `unsub*` 变量且 onUnmounted 逐个 `?.()`（脚本核对 decl/teardown 全 1）。`core-update-success` 只注册 1 次且合并了 IP 刷新（`:1184-1188`），启动时 IP 刷新路径未丢。 |
| 8 | addEventListener/remove 配对 | App.vue：blur/focus/visibilitychange/resize 4 增 4 删（`:965-967,1059` ↔ `:1244-1249`）；hoverGuard 卸载时 remove 3 个并 `restore()`；scrollFade dispose 移除 scroll/resize + 断开 2 个观察器 + 清初始 300ms 定时器；App.vue 另清 scrollTimer/appUpdateCheckFailsafeTimer/rAF/maximiseResyncTimer。 |
| 9 | 组件卸载后不写状态 | `stopWaveSampling()` + 全部 unsub + `unsubScrollFades?.()`；H-19 的 rAF/timer 句柄均在被清列表内。 |
| 10 | 通知表 S-04 | 表内 12 个 code 全部在 Go 侧有真实 `c.notify(`；被删的 7 个假 code 在 Go 全仓 **0 引用**（grep 逐项为空）→ 现在「有 code 就一定能按表分档」。`resolveNotificationUi` 优先级 code→level 未变，长连接降级逻辑保留。 |
| 11 | C-04 | `core-update-check-error` 复位标志 + reportError + toast；文案走 `tr('settings.update.checkFailed', 原文, {error})`，键在 8 语言存在（zh-CN:671）→ 不会露 key。 |
| 12 | C-10 | 被删的 PascalCase 分支逐个比对 `core/appcore/controller.go:519-559 AppState` 的 json tag：全部 camelCase（isRunning/mode/hideLogs/logLevel/appLogLevel/actual*/desired*/controlRevision/version/appVersion/isAdmin/activeConfig*/delayRetention*/update*/portProxyMode/language），且 `app-state-sync` emit 的就是该 struct → 删除安全；`actualSystemProxy ?? systemProxy` 这条合法兜底保留。 |
| 13 | C-12 | `UpdateTaskState/OutboundIPResult` 改为 bindings 类型 re-export；`UpdateTaskRow.vue:41` 的 import 路径不变；`normalizeOutboundIP` 字段映射仍在。 |
| 14 | C-20 | 只有 `SYSTEM_PROXY_STATE_UNKNOWN` 进 NOTIFICATION_TEXT（固定句无变量），`notify.systemProxyStateUnknown` 8 语言存在；其余含变量 code 仍显示后端原文（设计如此）。 |
| 15 | S-10 | api.ts 的 dev 期重名检测：`Object.keys(AppService/CoreService/UpdateService)` 实测 109 个导出且无重名；生产构建被 `import.meta.env.DEV` tree-shake。 |
| 16 | S-11 | `refreshOutboundIPRouteAware` 在发请求前 `clearTimeout(routeSettleTimer); routeSettleTimer=null`，`scheduleRouteAwareIPRefresh` 的 settle 回调置 null 后重排 —— 一次点击不再双发。 |
| 17 | S-02（前端侧） | `setThemeMode` 保留 GET→只改 themeMode→SAVE 读改写并写明多写者约定；失败路径新增「回滚到 previousMode + toast(settings.saveFailed 已存在)」——行为变化有注释、键存在、旧值来自 `themeState.config.mode`，不会把 system 误写成 light/dark。 |
| 18 | S-09 | store/App 两侧 normalize 一致：trace→debug、fatal/panic→error、warning→warn、保留 silent；App.vue 的 `logLevels` 含 silent:4，地板语义注释与 H-08 实现一致（filter=all 不套地板）。 |
| 19 | theme 双缓存键（U-08）闭环 | 写：只有 `cacheTheme`（applyThemeToApp 每次调用必经）；读：`readCachedTheme/readCachedThemeModeLabel` 只读 v2；store 不再写旧键。镜像写无读取方 → 已作为 MINOR #8 记录（不影响正确性）。 |
| 20 | 样式合并等价（U-22） | `surfaces.css` 拆分：.ui-surface-group 计算值与改前逐条相同（background 覆盖 + 继承的 radius 显式化）；`layout.css` `.ui-page-body` 两段属性集合不重叠，合并后同值；`.action-btn` 删掉被覆盖的 `gap:8px` 后 `gap:10px` 生效值不变；`--status-online/.dark{--danger}/@keyframes fadeIn` 删除前均确认 0 消费（`--danger` 仍由 :root 提供，值相同）。 |
| 21 | 圆角 token 完整性 | 使用到的 4 个名字（sm/md/lg/full）全部有定义（tokens.css + theme.ts setProperty 双保险），无 `--ui-radius-xs` 之类未定义名。 |
| 22 | trafficWaveState S-08/H-02 | 采样改事件驱动：后端 proxy 流就是 900ms ticker（`streams.go:150`）且每 tick emit（`:126`），core 流每次回调 emit（`:105-126`）→ 采样频率不变、不会停摆；`refCount` 只在 App 一处 start/stop（grep），S-08 的钳位 `Math.max(0, …)` 避免负值。H-02 后 `updateScale` 的回落分支可达且 `Math.max(minScale, …)` 保底。 |
| 23 | App.vue 行为链路 | ① H-09：`log-message` 订阅（:1020）先于 `StartStreamingLogs()`（:1024），历史合并 `mergeHistoryLogs` 对 logLines+logBatch+logPausedBuffer 建指纹集去重，且模板 key 用入列时的自增 `seq`（不再 index）；② H-04：`followTail` 在 push **之前**取样，用户上翻不再被拽底；③ H-06：class 走 `normalizeLogType`，`.log-line.warn` 命中；④ H-07：缺 source 归 'unknown' + 新增 std tab；⑤ H-01：离开 settings 复位 targetSettingsView；⑥ C-02：`requestTabChange` 在切页前判定未保存（`yamlLeaveConfirming` 防重入），`closeYamlEditor` 直接放行避免双确认，关窗路径补同类确认；⑦ H-03/H-11：下载/安装闸门在 `openModal` **之前**置位，`onConfirm` 的 finally 与 `onCancel` 双复位（无其它关闭路径：无 Esc 监听，遮罩走 `handleModalCancel→cancelModal`）→ 不会永久锁死；⑧ H-12 两条安装路径都走 `openInstallPathEmptyAlert`；⑨ H-19 乱序保护 seq + rAF 合并。 |
| 24 | utils 新文件 | `alertError.ts` 有 2 处调用（Rules×3、Subscriptions×3 的 6 个点经 imports 确认）、`clipboard.ts` 3 处（App/SettingsDiagnostics/Subscriptions）、`storage.ts` 3 处（Proxies/Connections/Rules）→ **无孤儿 util**；三者都不被 store.ts 反向 import（不会成环）。 |
| 25 | 删文件/删导出 | `utils/normalize.ts` 全仓 0 import；`ERROR_LOGGED_SUFFIX/getRecentErrors/formatDurationCn/normalizeGlassBlurStrength/GLASS_BLUR_OVERLAY_DEFAULT/GLASS_BLUR_CONTROL_DEFAULT` grep 均为 0 引用（只剩注释）。`report.ts` 只剩 reportError，`records` 移除后没有其它读点。 |
| 26 | 调试残留/临时文件 | 本批文件内 console.log 仅 `App.vue:1117`（HEAD 已有）；无 debugger/TODO 占位。`git status` 无新增临时/备份文件；`.probe/`、`.fixbackup/`、`.task/` 均在 .gitignore 中（未入库）。 |
| 27 | Go 事件面其它部分 | v3_events.go 本轮只改 `proxy-test-finished` 一行类型；被删的 `notify-error` 注册注释仍在、前端 0 订阅（一致）。`traffic-data/traffic-stat-mode-changed` 的注册类型与 streams.go emit 类型未变。 |

---

## 4. 覆盖度附录

### 4.1 逐文件 diff 已读（本批全部改动文件）
`store.ts`（39621B diff 全文）、`App.vue`（29954B 全文）、`api.ts`、`trafficWaveState.ts`、`main.ts`（无改动，确认）、
`utils/{errors,format,glass,hoverGuard,report,scrollFade,theme}.ts`、删除 `utils/normalize.ts`、新增 `utils/{alertError,clipboard,storage}.ts`、
`styles/{tokens,surfaces,layout}.css`、`style.css`、8 个 locales、`scripts/check-i18n.mjs`、
`app.go`、`services_v3_generated.go`、`services_v3_mapping.go`、`services_v3_mapping_test.go`、`v3_events.go`、`core/appcore/delay_manager.go`、`frontend/bindings/goclashz/coreservice.ts`。

### 4.2 未覆盖/说明
- 未跑任何构建/类型检查/单测（指令要求）；Go 的 `TestServicesV3AssignmentMatchesForwarders` 未执行，改用静态集合比对等价验证（§2.3）。
- `frontend/bindings/**` 是 .gitignore 生成物，只核对了工作区副本，不代表提交内容。
- 跨代理文件（Proxies/Subscriptions/useTunToggle/Overview 等）只在与本批契约相关处（事件 payload、intent 调用、i18n key）做了交叉核对，其自身行为归各自评审代理。

### 4.3 建议主控优先处理（按性价比排序）
1. **补回 Overview.vue 的 scoped `@keyframes pulse`**（唯一 MAJOR；改动 3 行，纯视觉回归）。
2. **重跑一次 wails binding 生成**，确认 eventdata.d.ts 与 `v3_events.go:80` 同型（避免后续 typed 事件误用）。
3. 清理 4 处死/矛盾注释与 1 个死导入（api.ts 62/108、mapping:10、store.ts:1031、App.vue:277、tokens.css:45、App.vue:440/446）；顺手把 store.ts:881 的裸 setItem 换成 writeLocalStorage、删 theme.ts 无读取方的旧键镜像写。
