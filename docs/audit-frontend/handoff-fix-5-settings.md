# 交接 · fix-5-settings（跨文件 / 跨代理项）

> 只列「不在本代理独占文件内」或「需要别的代理/主控决策」的事。已完成的部分见 `fix-5-settings.md`。

## 1 → fix-1（store.ts）：setThemeMode 与设置页保存的配合约定

设置页保存（`Settings.vue:236-277`）本轮改成 **GET 最新 → 只合并本地脏字段 → SAVE**，并串行化。
`store.ts:134-145 setThemeMode` 仍是「GET 全量 → 改 themeMode → SAVE 全量」。两边合起来的实际行为：

- 设置页保存时：先 GET，拿到侧栏刚写的 themeMode，所以**不会**把主题打回旧值（这是 G-04 的目标）。✅
- 侧栏切换时：它 GET 到的可能是设置页保存前的快照，整份写回会覆盖设置页刚写的字段（**残余窗口仍在**）。
  按你 S-02 的结论（不改读改写结构，只加注释），这个窗口本轮不修；根治靠后端补丁接口（已列 `backend-needs-fix-5-settings` §1）。
- 如果你后面把 setThemeMode 改成"只提交差异"，请在那之前告诉我（或直接在 store.ts 注释里写明「设置页侧已 diff-merge」），
  否则两边注释会互相矛盾。

**已核对、不需要你改的一点**：`Settings.vue:272-273` 保存成功后把后端口径的 `logLevel='warning'` 直接写进
`globalState.logLevel / appLogLevel`。`App.vue:450 rankOf()` 内部走 `normalizeLogType`（`'warning'→'warn'`，`App.vue:441-448`），
所以日志过滤地板不受影响。若你打算把 `globalState.logLevel` 的语义收紧成"仅 warn/debug/info/error/silent"并**去掉**消费侧的归一化，
请提前通知，我这边要加一次转换。

**可选（nice-to-have）**：你把 `tr(key, fallback)`（`store.ts:985`）做成了私有函数。若愿意导出，我这边三处
（G-03 按钮文案 / G-05 还原提示 / G-11 任务失败提示）就能换成独立的新 key + 中文兜底，
不必像现在这样复用 `invertSelect`/`restoreSuccess`/`statusFailed` 的现有文案。不导出也完全能跑，不影响验收。

## 2 → fix-7（utils/format.ts）：G-22 formatRelativeTime 硬编码中文

`frontend/src/utils/format.ts:37-50` 直接返回中文（`从未/刚刚/N 分钟前/N 小时前/N 天前`），
调用点 `SettingsUpdate.vue:32`（设置 → 组件与库 → GeoIP/GeoSite/MMDB/ASN 行）。这是设置模块里唯一的漏译点，属你的文件，我不动。

建议改法（二选一，都在你的范围内）：
1. `formatRelativeTime(ts, t)` 接受 t 注入，返回 `t('time.minutesAgo', { n })`；
2. 或返回 `{ key, params }`，由调用点拼（调用点在我文件里，需要我改一行的话发消息给我）。

需要的 5 个 key 与 8 语言文案已写在 `i18n-needs-fix-5-settings.md` §4（`time.never / justNow / minutesAgo / hoursAgo / daysAgo`）。
⚠️ 这些 key 一旦出现在代码里，`scripts/check-i18n.mjs` 会在语言包补齐前报 error，所以**语言包与代码要同一个 PR/同一次改动落地**。

## 3 → 主控（lead）

1. **Wave 2 的 i18n 只需改两条已有文案**，代码零改动：`settings.uwp.invertSelect`（反选→全部不选，8 语言）、
   `settings.about.restoreSuccess`（即时生效→已还原并重载，8 语言）。见 `i18n-needs-fix-5-settings.md` §1-2。
   我刻意没新增 key：`check-i18n.mjs` 对"代码用了但 zh-CN 未定义"是 error + exit 1，会在 Wave 2 之前把验收闸门 #2 打红。
2. **G-21 的附带项本轮未修**（请裁决是否本轮做）：`SettingsUpdate.vue:88 cancelCoreUpdateConfirm` 直接写
   `globalState.componentUpdate.pendingCoreUpdate = false`（全局字段，等于替全应用取消了"待更新"），
   而 `:89 executeCoreUpdate` 又不清该字段。改法涉及 store 侧事件回收语义（fix-1 的文件），本代理只做了 G-21 的 catch。
3. **超出 G-16 原始两条的一处文档修正**：`manifest.ts:998` 的 note 原写"全选 / 反选按钮"，随 G-03/G-18 一起改成
   "全选 / 全部不选（两者只作用于当前搜索结果）"。属同一个 bug 的文档面。
4. **复核 diff 时请注意**：`SettingsTheme.vue` 与 `manifest.ts` 在我开始前就有未提交改动（玻璃染色格改 `Transition`、
   `.blur-range:disabled` 样式简化、manifest 第 570 行 glassTint conditional 文案）。这些**不是**本代理改的，
   我的改动只有：`:disabled="loading"` 12 处、删 resetKey/emit('reset')、watch 改写、handleReset 的 isDanger + colorDelay。
5. **check-i18n 现状**：`node scripts/check-i18n.mjs` 当前报 12 项缺键，全部来自其它代理新增的 key
   （logs.std / logs.levelFilter / yamlEditor.* / proxies.loadFailedHint / rules.* / subs.* / traffic.*），
   本代理 13 个文件 0 贡献。`node scripts/validate-settings-manifest.mjs` 通过（103 项）。

## 4 → fix-2（App.vue）：无

设置页与 App.vue 的交互只有两个：`initialView` prop（H-01 由你侧复位）与 `showConfirm/showAlert` 全局模态（本代理未改语义）。
若你在 App.vue 里改了 `globalState.logLevel` 的取值域，见 §1 最后一段。
