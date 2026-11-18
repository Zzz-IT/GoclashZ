# R-1 复制粘贴型冗余收敛报告（Wave 3 · r1-common）

> 范围：审计 §4.3「复制粘贴型冗余」中的错误提示模板（R-4）与页面内 localStorage 薄包装（P-34），
> 外加审计 §4.1 在本轮文件权限内点名的 4 条死代码。
> 性质：**纯重构**。不改行为、不改文案、不改调用次数、不改事件订阅数量、不新增依赖。
> 基线：上一轮 6 个修复代理完成后的工作区（commit 840837f + 未提交修复）。

---

## 0. 结果摘要

| 项 | 数量 | 说明 |
|---|---|---|
| 新增工具文件 | 2 | `utils/alertError.ts`（1 个导出）、`utils/storage.ts`（2 个导出） |
| alertError 替换点 | **6** | Rules 3 + Subscriptions 3（审计 R-4 的 9 个点里，只有这 6 个是逐字相同的模板） |
| alertError 保留未替换 | 3 | Subscriptions 1（无 i18n key，正文 `u.message`）+ YamlEditor 2（正文是原始 `errorMsg`，未走 toUserError） |
| localStorage 收敛 | 3 个文件 / 4 个键 | 删除局部包装 2 组（Proxies、Connections）+ 删除内联 try/catch 2 处（Rules） |
| 死代码删除 | 4 项 | Proxies 3 项（webkit 滚动条伪元素、`.size-standard`、节点 `now` 字段）+ Rules 1 项（`.loading-state`） |
| 未合并项 | 1 处 | Proxies 两个 `onMounted`：按任务允许改为「只加注释说明不合并」 |
| 改动文件 | 4 | Rules.vue / Subscriptions.vue / Proxies.vue / Connections.vue |
| 未改（权限内但无合格点） | 8 | YamlEditor.vue、SettingsTun/Lan/Network/Dns/Behavior/Theme.vue、UpdateTaskRow.vue |
| 禁区文件 | 0 改动 | store.ts / App.vue / errors.ts / format.ts / locales/** / styles/** / style.css / 其它组件 / Go / scripts 全部未动 |

---

## 1. A · `utils/alertError.ts`

### 1.1 新文件

```ts
export async function alertError(
  e: unknown,
  key: string,
  params?: Record<string, string | number>
): Promise<void> {
  const u = toUserError(e);
  // params 先铺、error 后写：即使调用方误传 error 也不会覆盖人话错误文本
  await showAlert(t(key, { ...(params || {}), error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
}
```

- 标题固定 `t('common.error')`、`isDanger` 固定 `true`、detail 固定 `toUserError(e).detail`，与模板逐项一致；
- `params` 是为「非 error 的附加插值」预留（本次 6 个调用点都没有传）；
- 文件头注明 **store.ts 禁止反向 import 本文件**（会成环），与 `utils/clipboard.ts` 同一约定。

### 1.2 等价性说明（逐项核对）

| 维度 | 原模板 | alertError | 是否等价 |
|---|---|---|---|
| 模态次数 | 1 次 `showAlert` | 1 次 `showAlert` | ✅ |
| 正文 | `t(key, { error: displayErrorMessage(e) })` | 同 | ✅ |
| 标题 | `t('common.error')` | 同（函数内固定） | ✅ |
| 危险态 | `true` | 同（函数内固定） | ✅ |
| 折叠区 detail | `toUserError(e).detail` | 同 | ✅ |
| toUserError 调用次数 | 显式 1 次 + displayErrorMessage 内部 1 次 = 2 次（纯函数，无副作用） | 同（1 + 1） | ✅ |
| await 语义 | `await showAlert(...)` | `await` 后返回，拒绝照常抛给调用点 | ✅ |
| 去重 | `showAlert` 按 message+title 去重 | 入参完全相同 | ✅ |
| 语言切换 | 调用时取 `t` 的当前语言 | 同（模块引用同一 `t`） | ✅ |

alertError 内部**不**捕获 showAlert/t 的异常，因此若 `showAlert` 返回的 Promise 被拒绝，调用方的 `catch` 仍能看到——与原 `await showAlert` 一致。

### 1.3 替换点清单（file:line 为改后行号；改前行号见备注）

| # | 文件（改后行号） | 改前行号 | 新写法 | 原 key |
|---|---|---|---|---|
| 1 | `frontend/src/components/Rules.vue:261` | 263-264 | `await alertError(e, 'rules.loadOptionsError');` | rules.loadOptionsError |
| 2 | `frontend/src/components/Rules.vue:495` | 500-501 | `await alertError(e, 'rules.addError');` | rules.addError |
| 3 | `frontend/src/components/Rules.vue:551` | 556-557 | `await alertError(e, 'rules.deleteError');` | rules.deleteError |
| 4 | `frontend/src/components/Subscriptions.vue:466` | 464-465 | `await alertError(e, 'subs.importFailed');` | subs.importFailed |
| 5 | `frontend/src/components/Subscriptions.vue:489` | 487-488 | `await alertError(e, 'subs.renameFailed');` | subs.renameFailed |
| 6 | `frontend/src/components/Subscriptions.vue:578` | 576-577 | `await alertError(e, 'subs.deleteFailed');` | subs.deleteFailed |

原写法（6 处逐字相同，仅 key 不同）：
```ts
const u = toUserError(e);
await showAlert(t('<key>', { error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
```

配套 import 调整：
- Rules.vue：删除 `showAlert`、`displayErrorMessage, toUserError` 三个不再使用的导入（该文件已无其它裸 showAlert/toUserError 调用），新增 `alertError` / `storage` 导入；
- Subscriptions.vue：仅新增 `alertError` 导入；`showAlert` / `toUserError` / `displayErrorMessage` 仍被其它 6 个调用点使用，保留。

### 1.4 未替换点与原因（审计 9 个点里的另外 3 个）

审计 R-4 的行号（Rules 3 + Subscriptions 4 + YamlEditor 2 = 9）把三种不同写法归成了一组，逐处核对如下：

| 点 | 位置 | 现状 | 为什么不能替换 |
|---|---|---|---|
| 1 | `Subscriptions.vue:405`（updateSingleSub 失败，审计坐标 341） | `const u = toUserError(e); await showAlert(u.message, t('common.error'), true, u.detail);` | 标题/isDanger 相同，但**没有 i18n key**，正文是 `u.message` 而不是 `t(key, { error })`。套 `alertError` 必须新造 key 或改成 displayErrorMessage → 文案变化，违反纯重构。 |
| 2 | `YamlEditor.vue:342`（readError，审计坐标 301） | `showAlert(t('yamlEditor.readError', { error: errorMsg }), t('common.error'), true);` | 正文插的是原始 `e.message`，**没有走 toUserError/displayErrorMessage**，且**没有 detail**。替换后正文会变人话映射（可能带「（已记录日志）」后缀）并多出折叠区原文 → 信息量/文案变化。 |
| 3 | `YamlEditor.vue:391`（saveFileError，审计坐标 325） | `showAlert(t('yamlEditor.saveFileError', { error: errorMsg }), t('common.error'), true);` | 同上。 |

补充（审计未计入、但同属该变体，**保留**）：
- `Subscriptions.vue:726`（`subs-update-error` 事件）：`await showAlert(u.message, t('common.error'), true, u.detail)`，与第 1 点同型；
- `YamlEditor.vue:388`（fix-5 P-30 新增的 saveReloadFailed）：标题是 `t('common.warning')`，本就不在 R-4 模板内。

结论：R-4 的「9 处」实际是 **6 处同模板 + 3 处不同模板**；本轮替换 6 处，3 处保留并在此登记。

---

## 2. B · `utils/storage.ts`

### 2.1 新文件接口

```ts
export function safeGetString(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}
export function safeSetString(key: string, value: string): void {
  try { localStorage.setItem(key, value); } catch { /* 存不下不影响功能 */ }
}
```

语义与页面原实现逐行相同：读失败 → `null`（调用方各自回退默认值），写失败 → 静默。文件头同时登记了 store.ts / utils/theme.ts / locales/index.ts 里尚未合并的同构私有包装（见 handoff）。

### 2.2 替换点清单

| 文件 | 键 | 改动 |
|---|---|---|
| `Proxies.vue:156` | — | 删除局部 `readStorage` / `writeStorage` 两个函数（原 153-160 行），保留 P-34 注释并注明抽离 |
| `Proxies.vue:159` | `goclashz_proxyGroup` | `readStorage(...)` → `safeGetString(...)` |
| `Proxies.vue:289` | `goclashz_proxyGroup` | `writeStorage(...)` → `safeSetString(...)` |
| `Proxies.vue:322` | `goclashz_proxySortState` | `readStorage(...)` → `safeGetString(...)` |
| `Proxies.vue:326` | `goclashz_proxySortState` | `writeStorage(...)` → `safeSetString(...)` |
| `Connections.vue:143` | — | 删除局部 `readStorage` / `writeStorage`（原 140-147 行），保留 P-34 注释 |
| `Connections.vue:151` | `goclashz_connFilter` | `readStorage(...)` → `safeGetString(...)` |
| `Connections.vue:281` | `goclashz_connFilter` | `writeStorage(...)` → `safeSetString(...)` |
| `Rules.vue:148-152` | `goclashz_ruleTab` | `readSavedRuleTab` 去掉内联 try/catch，改 `safeGetString`；默认值与合法值校验逻辑不变 |
| `Rules.vue:378` | `goclashz_ruleTab` | watch 里的 try/catch + `localStorage.setItem` → `safeSetString` |

等价性：
- Proxies / Connections 的原包装与本文件**逐字同义**（含 catch 后静默），只是把函数体搬走；
- Rules 原读逻辑：`try { getItem } catch {} ` + 合法值枚举 + 默认 `'subscription'`；现为 `safeGetString`（catch→null）+ 同一套枚举与默认值，行为相同；
- Rules 原写逻辑：`try { setItem } catch {}`；现 `safeSetString` 内部同一 try/catch，失败静默，不打断后续 `searchQuery/currentPage/resetRuleSlider` 逻辑；
- 同步性不变（都是同步 localStorage API），调用次数不变（每键读 1 次 / 写 1 次）。

---

## 3. C · 死代码清理（每条均 grep 复核零引用）

### 3.1 Proxies.vue · `::-webkit-scrollbar-button / -track / -thumb / -thumb:hover`（原 1098-1113）

**确认方式**：`grep -n "scrollbar-button|scrollbar-track|scrollbar-thumb" frontend/src/components/Proxies.vue` → 仅这 4 条 CSS 命中，模板零引用；同文件轨道自 1044 行 `scrollbar-width: none`（Firefox）+ 1052-1055 行 `::-webkit-scrollbar { height: 0; display: none; }`（Chromium）起滚动条根本不渲染，button/track/thumb 永远不会被匹配。审计 §4.1 同一结论。
**处理**：删除 4 条规则，留一行注释说明删除依据（保留 1051-1054 的有效规则）。

### 3.2 Proxies.vue · `.node-grid.size-standard`（原 1323-1326）

**确认方式**：`grep -n "size-standard" frontend/src` → 仅 CSS 1 处；模板 92 行是动态 `:class="\`size-${cardMode}\`"`，`standard` 时同时带基础 `.node-grid` 类。对比声明：基础 `.node-grid`（现 1293-1298）= `grid-template-columns: repeat(auto-fill, minmax(260px,1fr))` + `gap:16px`；`.size-standard` 两项值与之一字不差，纯重复。`ProxyCardMode` 含 `'standard'`（store.ts:75），删除不影响该模式（由基础规则兜住）。
**处理**：删除重复块，注释保留「标准模式 = 基础 .node-grid」的说明。

### 3.3 Proxies.vue · 节点对象 `now` 字段（原 620）

**确认方式**：`grep -n "\.now\b|now:" frontend/src` 全仓只有以下读写：

| 位置 | 语义 | 读者 |
|---|---|---|
| Proxies.vue:97（模板） | `activeGroupData.now`（**组**的 now） | 选中态判断 |
| Proxies.vue:620（原） | `now: item.now`（**节点**对象字段） | **无** |
| Proxies.vue:629 | `now: item.now`（**组**对象字段） | 模板 97 / 事件 964 |
| Proxies.vue:958/964 | 事件里更新**组**的 now | — |

节点对象（`proxies` 数组元素）全仓无 `n.now` / `node.now` 读取，也无 `Object.keys/JSON.stringify` 遍历（已 grep 复核）。审计 §4.1「node.now 从未读」成立。
**处理**：只删节点级的 `now: item.now`（原 620 行），**保留**组级的 `now`（模板依赖）。

### 3.4 Rules.vue · `.loading-state`（原 758）

**确认方式**：`grep -n "loading-state" frontend/src` → 仅该 CSS 1 处，模板零引用（Rules.vue 的加载态由 EmptyState / 卡片结构承载；scoped 样式其它文件无法消费）。
**处理**：删除规则，留注释。

### 3.5 Proxies.vue · 两个 `onMounted`（现 305 与 872 起）——**不合并**

任务允许「有风险只加注释」。核对结论：
- 非错误路径上，合并（把 305 的同步块挪进 872 的 async 钩子开头）执行顺序完全相同（同一 mounted flush 内、父实例先于子组件）；
- 但合并后该钩子只能是 `async`：同步段若抛错会变成该 async 函数的 Promise 拒绝。Vue 3.5 的 `mounted` 钩子数组由 `flushPostFlushCbs` 直接 `cb()` 调用（runtime-core.cjs.js:374-384 无逐 cb try/catch），返回值拒绝不会交给全局 `errorHandler`；而现状下同步钩子的抛错会被 `flushJobs` 的 catch 交给 `handleError`。错误上报路径不同，按「行为不变」硬规则**不合并**，在 299-304 行加注释说明（`onMounted` 本体在 305 行起）。
- 收益仅为少一个钩子，不涉及重复实现，不属于本轮「复制粘贴型冗余」实质。

---

## 4. 验证

| 项 | 方式 | 结果 |
|---|---|---|
| Vue SFC 语法 | `@vue/compiler-sfc` `parse` + `compileScript`（不跑 vue-tsc） | Rules / Subscriptions / Proxies / Connections 全部 OK |
| 新增 TS 语法 | `typescript.transpileModule`（仅语法，不做类型检查） | alertError.ts / storage.ts 0 error |
| 引用零残留 | grep：`readStorage|writeStorage`（组件内）、`localStorage`（组件内）、`size-standard`、`loading-state`、`scrollbar-button/track/thumb`、`toUserError|displayErrorMessage|showAlert`（Rules） | 均只剩注释或合法的组级 `now` |
| 文件范围 | 本轮实际只编辑 4 个组件 + 新增 2 个 util | ✅（git 工作区其余改动来自上一轮修复代理，非本轮） |
| 构建命令 | **未跑** vue-tsc / pnpm build（按要求由主控统一跑） | — |

---

## 5. 回归风险（3 条 + 1 条补充）

1. **alertError 的模块依赖方向**：`utils/alertError.ts → store.ts`。目前无环（store 不 import 它），但后续若有人让 store.ts 引用 alertError 会形成循环初始化——文件头已写明禁止。
2. **Rules.vue 的 import 收缩**：删掉了 `showAlert` 与 `displayErrorMessage/toUserError` 导入。若后续在该文件新加裸 showAlert 调用，会在编译期报「未定义」——属显性失败，不会静默。
3. **删除节点 `now` 字段**：目前全仓零读取，但节点对象是 `any` 类型，未来若模板想读 `node.now` 会得到 `undefined`（无编译期保护）。已在删除处留注释提示组级 `now` 才是选中态来源。
4. （补充）**dead CSS 删除**：仅删被 `display:none`/`scrollbar-width:none` 覆盖的伪元素规则与逐字重复的 `.size-standard`，若浏览器行为差异导致滚动条重新出现，样式会回落到无自定义的外观（不会崩）。

---

## 6. 附：改动文件清单

| 文件 | 改动 |
|---|---|
| `frontend/src/utils/alertError.ts` | **新增** |
| `frontend/src/utils/storage.ts` | **新增** |
| `frontend/src/components/Rules.vue` | 3 处 alertError + 2 处 storage + 1 条死 CSS |
| `frontend/src/components/Subscriptions.vue` | 3 处 alertError |
| `frontend/src/components/Proxies.vue` | 4 处 storage + 3 项死代码/字段 + 1 处注释（onMounted 不合并） |
| `frontend/src/components/Connections.vue` | 2 处 storage |
| `docs/audit-frontend/handoff-r1.md` | **新增**（跨文件事项） |
