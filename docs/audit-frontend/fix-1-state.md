# 修复报告 · fix-1-state（状态层：store.ts / api.ts / main.ts）

> 修复代理：fix-1-state（8 人修复团队成员，主控 = lead）
> 日期：2026-11-18
> 独占文件：`frontend/src/store.ts`、`frontend/src/main.ts`、`frontend/src/api.ts`
> 实际改动：**store.ts、api.ts**（main.ts 无需改动）
> 证据来源：`01-state-core.md`（S-01~S-11）、`06-contract.md`（C-04/C-10/C-11/C-12/C-20）、`00-SUMMARY.md`
> 约束遵守：只改独占文件；未运行 pnpm build / vite build / vue-tsc / 应用本体；未改后端 Go；未改 locales。

## 1 逐条状态

| ID | 状态 | 改动位置(file:line) | 说明 |
|---|---|---|---|
| S-01 | fixed | store.ts:1268-1280（看门狗头注释）、store.ts:1297-1302（tick 内闸门） | pending 看门狗新增「有模态在台上就不计时」：`globalState.modal.show` 为真时每 tick 把起点顺延到当下（不打乱 60s 常量）。等用户思考不再被当成卡死，也就不会出现「误清 → 允许重入 → 旧流程 finally 清掉新流程 overlay」。强清前的 `console.warn` 原本就有，保留。 |
| S-02 | partial | store.ts:136-141 | 按约定**不改**读改写结构（GET→只改 themeMode→SAVE），改为把「多写者约定」写成代码注释：两边都必须 diff-merge，禁止盲写本地快照。**根治仍需后端**：SaveAppBehavior 是整对象覆盖 → 见 backend-needs-1。设置页那半由 fix-5 落地。 |
| S-03 | fixed | store.ts:180-233（overlay 加 sinceRevision + settleStaleControlOverlay）、store.ts:405-415（revision 门控内调用） | `setSystemProxyIntent/setTunIntent` 记录发起时的 `controlRevision`；`updateStateFromBackend` 收到**更大** revision 的 desired 时，若本地 overlay 的发起 revision 更旧则丢弃 overlay（settle），可见状态随即回到后端 desired。可见状态语义不变（overlay 优先，否则 desired）。调用方「直写 desired」的那一半已由 fix-2 的 H-14 移除（useTunToggle.ts:133-139 / Overview.vue:274-279 现已改为只 settle overlay + 重拉快照），两半合起来才闭环。 |
| S-04 | fixed | store.ts:931-973 | 全量复核 Go 侧 code 集合（`c.notify(` 11 处 + `c.setLastErrorCode(` 10 处 = 21 个真实 code），删除 7 个 Go 侧 0 引用的死键（ASSET_WINTUN_BUSY / ASSET_CORE_PROBE_TIMEOUT / HELPER_WAKE_TIMEOUT / TUN_ADAPTER_PENDING / ASSET_SEED_UNAVAILABLE / STARTUP_TASK_CHECK_WARNING / STARTUP_TASK_UNHEALTHY），并按「是否需要用户操作」补齐 3 个档位错的真实 code（ROUTE_SWITCHING→silent、TUN_NOT_READY→silent、MODE_SAVE_FAILED→toast）；表内 12 键每条都标了 Go 出处行号。 |
| S-05 | fixed | store.ts:46-64（两个 helper）、store.ts:66-73（模块级 5 处 getItem 换用 readLocalStorage） | 模块级 5 处 `localStorage.getItem` 全部收进 `readLocalStorage`（try/catch）。隐私模式/策略禁用下不再让整个 store.ts 求值失败（原来等于 App 白屏）。 |
| S-06 | fixed | store.ts:1014-1060（storeInited/storeInitPromise/eventUnsubs/initStore/setupStore/disposeStore）、全文 21 处 `eventUnsubs.push(Events.On(...))` | ① `storeInited = true` 从「首个 await 之前」移到 `setupStore()` 全部订阅注册完成之后；② 新增 in-flight `storeInitPromise`，并发的第二次 `initStore()`（main.ts:12 与 App.vue:923）复用同一次初始化——否则挪 flag 反而会让第二个调用插队、把 21 个订阅注册两遍；③ 21 个 unsub 收进 `eventUnsubs`，初始化中途失败时整批注销以便干净重试，另导出 `disposeStore()`（当前无调用者，仅注释说明 App 级生命周期不卸载）；④ 同文件重复注册的 `core-update-success` 合并为一个 handler（store.ts:1176-1182，原 :1132 处删除）。 |
| S-07 | fixed | store.ts:508-513（契约注释）、store.ts:579-585（showNextModal）、store.ts:595-599（settleModal 清 handler）、store.ts:615-619（drainModalQueue）、store.ts:621-624（删除 reconcileModalDisplacement）、store.ts:640-643（enqueueModal）、store.ts:655-676（confirmModal/cancelModal）、store.ts:1291-1292（看门狗改调 drainModalQueue） | 删前全仓 grep `globalState.modal`：写点只有 store.ts 的 presentModal/settleModal，App.vue 仅模板只读 → legacy 分支确证不可达，整体删除 `legacyOccupied`、`reconcileModalDisplacement()` 全文、confirm/cancel 的 legacy 分支、showNextModal/drainModalQueue 里多余的 `modal.show` 判空。`globalState.modal` 字段与 9 个模板字段契约**未变**；settleModal 顺手把 onConfirm/onCancel 置 null，避免残留 handler。 |
| S-09 | fixed | store.ts:349-361 | `normalizeLogLevel` 把 `warning` 归一到 `warn`（原来原样返回 'warning'，落在一个前端不存在的档位），保留 `silent`。App.vue 侧同类函数（fix-2）需同步加 warn/silent，见 handoff-1。 |
| S-10 | fixed | api.ts:19-41 | dev 期（`import.meta.env.DEV`）检查三门面导出名交集，命中即 `console.error` 列出冲突方与覆盖顺序。当前 108 导出无重名，该断言是「未来不变量」，生产构建被 tree-shake 掉。 |
| S-11 | fixed | store.ts:806-812 | `refreshOutboundIPRouteAware` 进入前先 `clearTimeout(routeSettleTimer)` 并置 null：路由 settle 窗口内点刷新不再出现「手动一次 + 定时器再一次」。 |
| C-04 | fixed | store.ts:1145-1155 | `core-update-check-error` 的 `data`（tasks 框架写入的完整错误文本）取出后 `reportError` + `showToast(...,'error',5000)`，不弹模态（与既有 B 类一致）。文案走 `tr('settings.update.checkFailed', …)`：语言包补齐前回落后端原文，不会露出 key 字面量。 |
| C-10 | fixed | store.ts:368-371 / 383-389 / 396-409 / 429-457 | 删除全部 `rawData.Xxx`（PascalCase）兼容分支（约 24 处 `else if` / `??`），只读 camelCase；保留「undefined 不覆盖」的部分更新语义，保留 `actualSystemProxy ?? systemProxy` 这条**合法**的 legacy camelCase 兜底。 |
| C-11 | fixed | store.ts:383、435（3 处 setItem → writeLocalStorage） | `updateStateFromBackend` 内 3 处 `localStorage.setItem` 收进 `writeLocalStorage`：写缓存失败不再中断后半段（logLevel/actual*/controlRevision/desired*/activeConfig*/language）的状态同步。 |
| C-12 | fixed | store.ts:22-30 | `UpdateTaskState`/`OutboundIPResult` 改为 `import type` 复用 `bindings/goclashz/core/appcore/models` 并为同名类型 re-export（`UpdateTaskRow.vue:41` 的 `import type { UpdateTaskState } from '../store'` 零改动）。逐字段核对：两个 interfaces 与手抄版字段名/类型/顺序完全一致，无需映射函数。 |
| C-20 | fixed | store.ts:975-993（NOTIFICATION_TEXT + tr + localizedNotificationText）、store.ts:954-957（ROUTE_SWITCHING→silent）、store.ts:1116-1121（toast 分支改用本地化文案） | 两段式：① ROUTE_SWITCHING 移入 silent（中文技术串不再进 toast，与 06-contract §C-20 建议②一致）；② 会展示给用户的 code 走映射表 `NOTIFICATION_TEXT` + `tr()`，缺键回落后端原文——只收录「后端文案是固定句、不含变量」的 code（SYSTEM_PROXY_STATE_UNKNOWN），含错误串的仍展示后端原文（丢信息比出中文更糟）。新 key 见 i18n-needs-1。 |
| — | fixed | store.ts:1176-1182 + 1216 | （任务附加项）`core-update-success` 在同一函数内注册两次 → 合成一个 handler，顺序保持「先复位组件更新状态，再排期刷出站 IP」。 |

状态统计：**fixed 14 / partial 1（S-02，前端侧完成、根治待后端）/ skipped 0 / 驳回 0**。

## 2 关键取舍（给复核用）

1. **S-04 的 code 集合是脚本化取证的**：`c.notify(` 11 处（controller_long_connection.go:175/196/201、controller.go:425/429/444/446/450/1416/2234、control_coordinator.go:457、outbound_ip.go:233）+ `c.setLastErrorCode(` 10 处（controller.go:910/1084/1603/2479/2485、control_coordinator.go:445/528、supervisor.go:238/260/288、updater_orchestrator.go:97 → RUNTIME_OPERATION_FAILED）= 21 个真实 code。没有「照方案表补」任何一条后端不存在的 code。
2. **不把「与兜底同档」的 code 抄进表**：`resolveNotificationUi` 的兜底里还有「长连接延迟降级为 toast」这条语义（error→modal 的例外），显式把 CORE_START_FAILED 等写成 modal 反而会误伤它。TUN_ADAPTER_NOT_READY（controller.go:910，只在**不可自愈**分支才发）也留在兜底→modal，不擅自降级成 toast。
3. **S-03 用 `revision > since` 而不是 `>=`**：revision 相等意味着后端没有给出新的结论，overlay 仍然有效；只有严格更新才说明「后端已经就这次命令表态」。
4. **S-06 必须带 in-flight promise**：只把 `storeInited` 挪到末尾会引入重复订阅（main.ts 与 App.vue 各调一次，第二次会在首个 await 处插队）。两者是同一个修复的两半，缺一不可。
5. **S-07 删代码前的前置验证已做**：`grep -rn "globalState\.modal" frontend/src` 结果 = App.vue 模板只读 9 处 + store.ts 写点，0 组件直写 → 分支不可达。
6. **新增 i18n key 不会让 `scripts/check-i18n.mjs` 失败**：两个 key 都只以 `tr('key', 原文)` 形式出现，而检查器用的是 AST `t('字面量')` 判定（check-i18n.mjs:63）+ .vue 模板正则（:80），`tr(` 不匹配。Wave 2 补上文案后代码自动生效，不需要再改 store。

## 3 自查（未跑构建，用只读手段代替）

| 项 | 方法 | 结果 |
|---|---|---|
| 语法 | typescript `createSourceFile` 解析 3 个文件 | 0 语法错误 |
| 类型 | typescript `createProgram` + 项目 tsconfig 等参（strict / Bundler / ESNext，含 `src/vite-env.d.ts`）只报本文件诊断 | 0 诊断（`import.meta.env.DEV` 由 vite/client 类型解析通过） |
| PascalCase 残留 | grep `rawData\.[A-Z]` | 0 |
| localStorage 裸调用 | grep `localStorage\.(get|set)Item` | 仅剩 :116、:242（原本就有 try/catch）与 :890（在 refreshOutboundIP 的 try 内，失败被 catch 降级为 stale 标记；不属 C-11 的 3 处） |
| 死符号 | grep `reconcileModalDisplacement` / `legacyOccupied` | 仅剩解释性注释 |
| 重复订阅 | grep `core-update-success` | 1 处注册 |
| 临时文件 | 复查 git status | 仓库内无新增临时/备份文件（构建期脚本放在系统 TEMP 且已删） |

## 4 回归风险（3 条）

1. **S-03 让 overlay 比原来早消失**：成功路径上 overlay 会在调用方 finally 之前就被后端 revision 收掉，可见值来源从「本地 intent」切成「后端 desired」。正常情况下两者相等、无可见变化；但若后端广播的 desired 与本次目标不一致（后台 reconcile 抢先改写），卡片会立刻显示后端值而不是用户刚点的目标——这是设计意图，首次实机点开关时容易被误判成「点了没反应」，建议验收时重点看这条路径。
2. **S-04 的档位调整会静默掉两类通知**：ROUTE_SWITCHING 与 TUN_NOT_READY 不再有任何 UI（只进 reportError → console/getRecentErrors），MODE_SAVE_FAILED 从模态降为 toast。若产品上认为「路由切换卡住/持久化失败」必须被用户看见，需要回滚这两条档位（单行改动）。
3. **C-10 去掉 PascalCase 兼容后完全没有兜底**：万一某条路径的 AppState 序列化变成 PascalCase（或某个调用方手搓了 Pascal 风格的假 state），表现会是「字段静默不更新」而不是报错。验收时建议点一次系统代理/TUN 开关 + 改一次隐藏日志，确认 `app-state-sync` 仍能正常回灌。
