# 跨文件移交 · fix-1-state

> 我的独占文件：store.ts / main.ts / api.ts（main.ts 未改）。
> 下面每一条都**不阻塞**我的修复完成，但需要对应文件负责人确认/配合，否则闭环不完整。

## H-1【fix-2 / App.vue:923】重复的 `initStore()` 调用已无害，建议删除

- 现状：main.ts:12 与 App.vue:923 各调一次 `initStore()`。我已加 in-flight promise 守卫（S-06），
  第二次调用复用同一次初始化，**不会重复注册订阅**，行为安全。
- 建议：fix-2 删掉 App.vue:923 的那一行（它是死调用，且会误导后来者以为初始化在组件里）。
- 若保留：也无需再改 store 侧。

## H-2【fix-5 / Settings.vue saveBehavior + SettingsAbout.vue 还原后】S-02 的另一半

- 约定（已写进 store.ts:136-141 注释）：设置页保存前必须 `GET 最新 behavior` → 只把**本页脏字段**
  合并进去 → 再 `SaveAppBehavior`；保存成功后把合并结果写回 `behavior.value`（刷新快照）。
- 不要再用「`behavior.value` 整对象写回」。`SettingsAbout.vue` 还原备份成功后同样必须重拉快照
  （否则第一次保存把还原结果覆盖回旧值，G-05）。
- 两端都是 diff-merge 才算闭环；后端补丁语义另见 backend-needs-1 B-1。

## H-3【fix-2 / App.vue:396-405】S-09 的另一半：logLevels 与 normalizeLogType 要认 `silent`

- store 侧 `normalizeLogLevel` 现在返回 `'warn'`（不再返回 'warning'）且保留 `'silent'`。
- 因此 App.vue 的 `logLevels` 必须加 `warn: 2`（已有）与 **`silent: 4`**，`normalizeLogType` 必须接受
  `'silent'` 而不是回落到 `'info'` —— 否则用户把内核级别设成 silent 后日志面板仍显示历史 info/warn/error，
  「设置静默仍然刷屏」的症状会保留。
- 两边取值域必须完全一致：debug < info < warn < error < silent。

## H-4【所有改 App.vue 模态相关代码的人】S-07 后的模态契约

- `globalState.modal` 现在是**只读渲染**契约：App.vue 只读 show/type/title/message/detail/isDanger/
  confirmText/cancelText，按钮走 `confirmModal()` / `cancelModal()`（签名未变）。
- 不变量：`activeModal !== null` ⟺ `globalState.modal.show === true`。**任何组件直写 `globalState.modal`
  都会立刻打破它**（我已经删掉全部兼容分支，没有兜底了）。要弹窗请用 `openModal/showAlert/showConfirm`。
- 自建模态（如 App.vue 的 showCoreSeedModal）与全局队列模态并存的问题（历史 F-A4-07）不在我的范围，
  但请注意：现在没有任何东西阻止两层遮罩同时出现。

## H-5【主控验收】C-10 之后没有 PascalCase 兜底

- `app-state-sync` / `GetAppState` 的字段名现在按 **camelCase 唯一口径**读取。
- 验收建议：点一次系统代理开关、改一次「隐藏日志」，确认设置与卡片都跟着变；若字段不更新，
  说明该路径的 state 序列化不是 camelCase（Go json tag 应全为 camelCase，属回归信号）。
