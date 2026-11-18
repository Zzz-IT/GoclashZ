# fix-4-pages · 修复报告（Rules / Subscriptions / YamlEditor）

> 修复代理：fix-4-pages（8 人修复团队，主控 lead）
> 时间：2026-11-18（Asia/Singapore）
> 独占文件：`frontend/src/components/Rules.vue`、`Subscriptions.vue`、`YamlEditor.vue`
> 未触碰：Go 代码、`frontend/src/locales/*`、其它代理的独占文件、依赖清单
> 自检：3 个 SFC 用 `@vue/compiler-sfc` 做过 parse + compileScript + compileTemplate（带 typescript 插件）→ 脚本/模板语法 0 错；**未**运行 pnpm build / vite build / vue-tsc / 应用本体（按规矩由主控统一跑）

## 1. 条目状态总表

| ID | 状态 | 改动位置（file:line） | 一句话说明 |
|---|---|---|---|
| P-01 | fixed | Rules.vue:44, 466, 508-562 | 删除按钮改传规则文本；确认后重读当前段并按文本定位，找不到就提示并同步列表，绝不再复用渲染时的 originalIndex |
| P-05 | fixed | Subscriptions.vue:13-19, 246-253 | 排序入口 `v-if="SORT_MODE_AVAILABLE"`（=false）隐藏；moveUp/moveDown/toggleSortMode 原样保留但不可达，等后端 SaveSubOrder |
| P-06 | fixed | YamlEditor.vue:88, 292-348, 453-488 | keydown 与 theme MutationObserver 移到 `await loadConfig()` 之前同步注册；新增 disposed + 读盘序号判据，卸载/过期续体不再建 EditorView；onUnmounted 置 disposed、清定时器、destroy + 置空 view |
| P-07 | fixed（编辑器侧） | YamlEditor.vue:5-6, 414-422 | 返回箭头改走 `handleBack`：isModified 时先 showConfirm 再 `emit('back')`；isModified 与 status-change 在改/存/重载/保存失败四条路径上都及时更新。**App.vue 侧守卫属 fix-2**，见 handoff-4.md |
| P-08 | partial | Subscriptions.vue:665-668, 693-704, 706-714；Rules.vue:537-551 | 删掉 Subscriptions 里永不触发的 `config-changed` 监听（唯一 emit 点是保存激活配置文本，本页与 YamlEditor 互斥挂载）；Rules 侧不新造事件，改用「删除前重读当前段」作为页内刷新通道。Proxies.vue:765-767 同款死监听属 fix-3 |
| P-22 | fixed | Rules.vue:564-566 | 删除 `onMounted(loadRules)`（watch(activeConfigId,{immediate}) 已覆盖挂载时那一次），进页只发一次 GetRulePageData |
| P-23 | fixed | Rules.vue:194, 471-506 | 添加规则加同步 in-flight 标记 `addingRule`（不依赖 :disabled 的重渲染时机），loading 也在入口同步置位 |
| P-24 | fixed | Rules.vue:522-528 | 确认前捕获 configId / 段，确认后校验 configId 未变（变了就 toast 取消），不再确认后重读 globalState |
| P-25 | fixed | Subscriptions.vue:375-397, 154-156, 663-667 | 导入前用 `new URL()` 校验 http/https；命名弹窗内实时判重（同名/同链接）并就地警示「会新增第二条订阅」。**没有**叠第二个确认框（两个 modal-overlay 同 z-index 会互相遮挡） |
| P-26 | fixed | Subscriptions.vue:274-305 | fetchConfigs 加递增序号：旧响应不再回写列表/激活配置，loading 也只由最后一次请求收起 |
| P-27 | fixed | Subscriptions.vue:343-356, 696-698 | 监听 `subs-update-cancelled` 复位 isUpdating；handleUpdateAll 入口同步置位并挡重复发起，传输层失败在 catch 里复位 |
| P-28 | fixed | Subscriptions.vue:528-547 | 删除前捕获 id；删掉的正是 activeConfigId 时先本地清空三个 active* 字段，再 fetchConfigs，避免悬空 id |
| P-29 | fixed | YamlEditor.vue:56, 85-88, 241-251 | 删掉 statusText（写 8 次读 0 次）与未使用的 globalState 导入；状态 emit 收敛成唯一出口 `emitStatus()` |
| P-30 | partial | YamlEditor.vue:349-357, 380-392 | 前端双兼容：先读 `e.saved/e.reloadFailed` 结构化字段，回退匹配后端文案「保存成功…重载」；命中则提示「已保存，但重新加载失败」并清 dirty。**根治需要后端结构化返回值**，见 backend-needs-4.md |
| P-31 | fixed | YamlEditor.vue:241-251, 271-290, 398-408 | docChanged 不再硬发 `error:false`：error 由 `yamlError` 推导，红标只在 validate/save/load 结算时改变 |
| P-32 | fixed | YamlEditor.vue:253-258, 271-290, 359-369 | docChanged 同步置脏 + 180ms 后做一次全文比较（只用于「改回原样则清脏」）；保存前再精确比一次，避免"改回原样立刻 Ctrl+S"白发一次内核重载 |
| P-33 | fixed | YamlEditor.vue:441-451 | 新增 `watch(() => props.configId)`：换配置时重载（脏则先确认），防未来改 v-show/KeepAlive 后把旧内容写进新 id |
| P-34 | fixed | Rules.vue:144-156, 379-384 | ruleTab 读（含取值校验）/写全部包 try/catch。Subscriptions.vue 与 YamlEditor.vue 内无 localStorage，无改动点 |

统计：**18 条 = fixed 16（其中 P-07 只完成编辑器侧）· partial 2 · skipped 0 · 驳回 0**（P-08 / P-30 为 partial，均有移交说明）

## 2. 逐条实现要点（只记"为什么这么改"，不重复报告）

### P-01 / P-24 · Rules 删除
- 治本手段是**换定位键**：`handleDelete(rule.originalIndex)` → `handleDelete(rule.text)`。`paginatedRules` 里本来就解析了 `text`，直接带出去，不新增数据结构。
- 确认后先校验 `configId`，再 `GetRulePageData(configId)` 重读一次，用 `rulesOf(data, isLocal, section).indexOf(ruleText)` 定位；`< 0` 时 toast「规则列表已变化，本次操作已取消」并把新列表写回界面。**绝不猜一个下标**（后端对越界索引静默返回 nil，删错是看不出来的）。
- 抽了 `rulesOf(data, localConfig, section)` 给 `currentRulesList` 与删除定位共用：两处取值逻辑一旦分叉就会按错的段算下标（这是本条最容易写错的地方）。
- 代价：一次删除从 1 次 RPC 变 2～3 次（重读 + 删除 + 刷新），本地磁盘操作，量级可忽略。
- 已知残余：同段存在**两条完全相同的规则文本**时，`indexOf` 命中的是第一条而不是被点的那条；两条文本相同、删哪条结果一样，不构成数据损坏。

### P-05 · Subscriptions 排序隐藏
- 只加了一个 `const SORT_MODE_AVAILABLE = false` + 入口 `v-if`；`moveUp/moveDown/toggleSortMode/isSortingMode` 与相关样式**一行未删**，后端落地 `SaveSubOrder` 后把常量改 `true` 即恢复。
- 排序模式内被 `fetchConfigs` 打回原样的问题因此也不再可达。

### P-06 / P-07 / P-29~P-33 · YamlEditor
- P-07 的"及时更新"靠 `emitStatus()` 单一出口实现：docChanged / loadConfig / handleSave / handleValidate / 保存失败分支都走它，杜绝"改了一处忘了发事件"。
- P-06 采用"先注册再 await"而不是"await 后判 disposed"，因为 `onUnmounted` 一定能清到已注册的监听；同时在 `loadConfig` 的 await 之后加了 `if (disposed) return;`，防止卸载续体再建 EditorView。
- P-06 顺手堵了同类竞态的另一条路：`loadConfig` 加了递增序号（`if (disposed || seq !== loadSeq) return`），props.configId 变化（P-33 的 watch）或重新加载与上一次读盘重叠时，两个续体不会各建一个 EditorView（先建的那个将永远没人 destroy）。
- P-31 把 `yamlError` 当作唯一诊断来源（以前它只给死掉的 statusText 用）：validate 失败/load 失败/保存失败写入，成功清空；docChanged 只回显，不再覆盖。
- P-32 的方向是"只允许变干净"：置脏同步（守卫立刻生效），清脏延后 180ms；因此未保存确认只会比以前更**保守**，不会漏。
- P-30 的文案匹配只用于**提示措辞**，不改变"文件已落盘"这一事实判断的落点：命中才清 dirty，未命中仍按保存失败处理（失败方向安全）。
- P-33 的 watch 与 `onMounted` 的初始 `loadConfig` 不冲突：watch 没有 immediate。

### P-08 · Subscriptions 死监听
- 删除依据：`config-changed` 全仓唯一 emit 点是 `core/appcore/config_editor_controller.go:35`（保存"激活配置"的文本），而保存入口 YamlEditor 与本页 `v-if` 互斥挂载 ⇒ 挂载期间事件不可能到达。删除后 `unsubConfigChanged` 变量一并去掉，避免留一个永远为 null 的句柄。

### P-25 · Subscriptions 导入
- URL 校验放在 `handleDownload`（点"开始下载"就报），比等到下载失败更早、更能指向问题。
- 判重放在命名弹窗的 computed（`importDuplicate`）：用户改名字/改链接时实时变化；提交按钮**不拦**（与改动前一致，只把后果说清楚），避免把"导入"变成两步确认流程。
- 本地文件导入的 `c.url` 为空，所以链接判重不会误报；同名判重对本地导入同样生效（同症状）。

### P-26 / P-27 / P-28 · Subscriptions 并发与状态
- P-26 用序号而不是 in-flight 合并：写操作之后必须拿到**最新**列表，合并会延迟可见性。
- P-27 同时做两件事：入口同步置位（挡同帧连点）+ 监听 cancelled（兜底）。`UpdateAllSubsAsync` 走的是 `Tasks.Run`（非 RunIfIdle），一定会发 `-start`，所以不会出现"置位后永远收不到事件"的新死锁。
- P-28 只在 id 命中时清空三个字段，不动别的情形。

### P-34 · Rules localStorage
- 读：`readSavedRuleTab()` 包 try/catch 并**校验取值**（旧代码把任意字符串硬转 union，脏值会让"当前规则"页永远空列表）。
- 写：watch(ruleTab) 里的 `setItem` 包 try/catch，失败只丢失"记住上次选项卡"。

## 3. 交叉依赖

| 事项 | 归属 | 说明 |
|---|---|---|
| P-07 App.vue 侧离开守卫 | fix-2 | 见 handoff-4.md（**含"别连弹两次确认"的具体约束**） |
| P-08 Proxies.vue:765-767 死监听 | fix-3 | 我未改 Proxies.vue |
| P-13 Rules.vue:568-572 `onActivated` 死钩子 | 待指派 | P-13 不在我的条目表里，但文件归我独占 → 我有意未动，见 handoff-4.md |
| P-30 结构化错误 | fix-backend | 见 backend-needs-4.md |
| P-05 SaveSubOrder | fix-backend | 见 backend-needs-4.md |
| 6 个新 i18n key | fix-i18n（Wave 2） | 见 i18n-needs-4.md |

## 4. 回归风险（3 条）

1. **Rules 删除多一次重读 + 文本定位**：正常路径多 1～2 次本地 RPC；若确认期间列表被外部改写，用户会看到「规则列表已变化，本次操作已取消」而不是删除成功（需要再点一次）。同文本重复规则会删到第一条（结果等价）。
2. **YamlEditor 的 dirty 判定变保守**：任何文档变更都会立刻置 dirty（"改回原样"最长延迟 180ms 才清），因此「刚改完又改回」的瞬间点返回/切页会比以前更容易弹未保存确认；保存路径已补精确比较，不会白发内核重载。另外 P-31 之后，校验失败的红标会**一直留到下一次校验/保存**（这是修 bug 的预期效果，但会比以前显眼）。
3. **Subscriptions 「更新全部」状态机改为本地先置位**：若后端 RPC 成功但 start/success/error/cancelled 四个事件全部丢失（框架级异常），按钮会一直禁用（以前依赖 start 事件，触发面同源）；P-30 的文案匹配也依赖后端保留「保存成功…重载」措辞，后端改文案后该分支会退回旧的「保存失败」提示（方向安全，不会误判成功）。
