# Handoff · fix-7（utils / styles 修复代理）→ 其他代理

> 本文件列出**不在本代理独占文件范围**、但与本轮修复配套的改动。
> 每条都注明「改哪个文件的哪一行」「改成什么」「为什么」。
> 本代理已改完的文件见 `fix-7-utils.md`。

---

## 1 → fix-1-state（`frontend/src/store.ts`）

### H1-1 【U-08】删掉第三个明暗真源（旧缓存键）

- 现状：`store.ts:70` `const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';`
- 改为：`import { readCachedThemeModeLabel } from './utils/theme';` + `const cachedTheme = readCachedThemeModeLabel();`
- 理由：`goclashz_theme` 只存 `'light'|'dark'`，与真源 `themeState.config`（`goclashz_theme_v2`）可能不同步
  （例如「跟随系统 + 系统深色」时 v2 记 `mode:'system'`、旧键此刻是 `light`），首帧会出现
  `<html>.dark` 成立而 `.app-shell` 没有 `.dark` 的一帧。新函数与 `themeState` 同源。
- 配套：`store.ts:113-120` 的 `localStorage.setItem('goclashz_theme', nextTheme)`（含 try/catch 整段）**可以删掉** ——
  `utils/theme.ts` 的 `cacheTheme` 现在每次写 v2 时都会同步写旧键镜像（`theme.ts:451-457`），
  且保证两键由同一次 normalize 结果派发。

### H1-2 【U-09】store.ts 剩余 3 处裸 `localStorage`

- `store.ts:69/71/72/75`（模块顶层 getItem，抛错会让 store 模块加载失败 → 白屏）
- `store.ts:343-347`（`hideLogs` setItem）、`store.ts:402-405`（审计 05 报告 §U-09 指出的第三处）
- 改法：包 `try/catch`，或复用 `utils/theme.ts` 里那对 `readStorage/writeStorage`。
  如果需要复用，可在 theme.ts 把这两个函数导出（本代理未导出，避免扩大 utils 的公开面；由主控决定）。
- 理由：同仓库其它 4 处都有保护，这三处是漏网；模块顶层抛错等于白屏。

### H1-3 提醒（无需改动）

- `store.ts:1027` 等 `displayErrorMessage` 调用点**签名没变**（仍返回 string），不需要改。
- `cacheTheme` / `readCachedTheme` 签名没变，只是内部改为安全读写 + 双写旧键镜像。

---

## 2 → fix-2-shell（`frontend/src/App.vue`）

### H2-1 【U-18】剪贴板三套实现 → 统一 `utils/clipboard.ts`

新文件 `frontend/src/utils/clipboard.ts` 已就绪（本代理新建），导出：

```ts
copyToClipboard(text: string): Promise<boolean>   // 只复制，不弹提示（失败已 reportError）
copyText(text: string, okMsg?: string): Promise<boolean>  // 复制 + toast（成功/失败都有 i18n 文案）
```

改点：

1. `App.vue:576-581`（`handleCopyLogs` 的 try/catch）
   → `await copyText(text);`（保留前面「空列表先 `showToast(t('common.empty'),'info')` 再 return」的分支）
   - 收益：失败文案从 `t('common.copy') + ' ✗'`（把按钮名当失败提示 + 硬编码符号，8 语言都一样）
     变成 `common.copyFailed`。
2. `App.vue:586-596`（`handleCopyLogDir`）
   → `await copyText(dir, t('settings.diagnostics.logDir') + ': ' + dir);`
   - 注意：**保留**外层对 `App.GetDataDirInfo()` 的 try/catch（那是取目录失败，不是剪贴板失败）。
3. 之后 `App.vue` 里不再需要直接碰 `navigator.clipboard`。
4. `components/SettingsDiagnostics.vue:245-251` 与 `components/Subscriptions.vue:435-440` 同理，
   但这两处分别属于 fix-5 / fix-4 的文件，本代理没有跨范围改（见下两节）。

### H2-2 【U-11】滚动渐隐现在有 dispose，App 卸载时释放

- `App.vue:758` `initScrollFades();` → `unsubScrollFades = initScrollFades();`（`initScrollFades` 返回值从 `void`
  变成 `() => void`，调用语句本身不报错，只是现在可以拿到释放函数）。
- 在 `onUnmounted`（`App.vue:976-979` 附近，已在释放 `uninstallHoverGuard`）追加
  `unsubScrollFades?.(); unsubScrollFades = null;`。
- 理由：原来三个观察器（scroll/resize/两个 Observer）没有任何释放路径（U-11 ③）。
  正常运行时 App 是根组件不会卸载，所以**不改也不影响功能**；改了对 HMR / 测试友好。

### H2-3 【U-19】hover 清理两套机制：本代理保留 `hoverGuard`，请勿删除它

- 审计给的候选是「删 `installHoverGuard`、保留 `clearCtrlHover`」，但本代理复核后认为两者**不是同一件事**：
  - `hoverGuard` 改的是**全站 CSS `:hover` 选择器**（清掉浏览器缓存的陈旧 hover）；
  - `clearCtrlHover`（`App.vue:336-339/773-777`）清的是**JS 驱动的 hover 态**（`ctrlHover` ref，
    窗口控件用 `@mouseenter/@mouseleave` 而不用 `:hover`）—— hoverGuard 管不到它。
- 因此本轮 **U-19 判定为"保留两套"**，但 `hoverGuard` 本身已按 U-10 修好（抑制期间补收集 / restore 失败重试 /
  只在 blur 与页面隐藏时抑制）。
- 如果主控仍要求收敛成一套，最小改法：给 `installHoverGuard` 加一个可选回调
  `installHoverGuard(onReset?: () => void)`，在 blur / visibilitychange(hidden) 时调用它，
  App.vue 再把 `clearCtrlHover` 传进去、删掉自己的三个监听。
  **本代理没有这么做**：硬性规则 5 要求不改现有导出签名，是否改由主控决定。

---

## 3 → fix-5-settings（`components/SettingsTheme.vue`）

### H5-1 【U-07】染色滑条改用独立上限常量

- 本代理在 `utils/glass.ts:20` 新增了 `export const GLASS_TINT_MAX = 80;`（数值与 `GLASS_BLUR_MAX` 相同，
  语义独立；`glassTintColor()` 内部已改用它做除数）。
- `SettingsTheme.vue` 有 3 处把染色滑条与模糊上限绑在一起，请替换：
  1. `:219` `:max="GLASS_BLUR_MAX"` → `:max="GLASS_TINT_MAX"`
  2. `:277` 的 import 列表：`GLASS_BLUR_MAX` → `GLASS_TINT_MAX`（若该文件其它地方不再用 `GLASS_BLUR_MAX` 就整条换掉）
  3. `:586` `const trackStyle = (value: number, max: number = GLASS_BLUR_MAX)` → 默认值 `GLASS_TINT_MAX`
- 数值相同，**本 Wave 无视觉变化**；换掉后「改模糊上限导致染色半饱和 / 滑条填充错位」的隐患消失。

### H5-2 提醒：tokens 里删了两个变量，SettingsTheme 已确认不消费

- `--glass-fill-content`、`--ui-accent-hover`、`--status-online` 等 10 个零消费 token 已删；
  `SettingsTheme.vue` 里 grep 不到这些名字，无需改。

---

## 4 → fix-4-pages（`components/Subscriptions.vue`）与 fix-5（`SettingsDiagnostics.vue`）

### H4-1 【U-18】订阅页分享链接的复制

- `Subscriptions.vue:431-449` `handleShareLink` 现在是 `navigator.clipboard.writeText` + 自拼 `status`。
- 建议改为：

```ts
import { copyToClipboard } from '../utils/clipboard';
const ok = await copyToClipboard(config.url);
const status = ok ? t('subs.linkCopied') : t('subs.copyFailed');
```
- 保留后面的受管模态（`openModal`）与 `maskUrl` 脱敏细节不变。

### H5-3 【U-18】诊断页路径复制

- `SettingsDiagnostics.vue:239-251` `copyPath` → `await copyText(text);`（`import { copyText } from '../utils/clipboard';`），
  保留「空文本先 toast `common.empty`」的分支。

---

## 5 → fix-i18n（Wave 2）

- 7 个新 key 与 8 语言文案见 `docs/audit-frontend/i18n-needs-fix-7.md`。
- 本代理代码已做「缺键回退中文」，所以 key 补齐前后都可用。

---

## 6 → 主控：两处需要裁决 / 记入 Wave 3

1. **U-14 与 `06-contract.md` C-22 冲突**
   - 本代理按本轮指令**删掉了** `report.ts` 的 50 条环形缓冲与 `getRecentErrors()`（零读取方）。
   - 但 `docs/audit-frontend/06-contract.md:1103` 与 Q7 建议**不要删**，而是把最近错误并入
     `ExportDiagnostics`（需要后端配合追加文件/字段）。两条指令互斥。
   - 若采纳 C-22：在 `reportError` 内部做落盘/转发即可恢复能力（本代理已在文件头注释写明这条路径），
     不要重新引入"只写不读"的缓冲。
2. **U-22⑤ `* { font-family: var(--font-sans) !important }` 本代理未去掉**（`style.css:229-234`）
   - 已确认存在依赖，去掉会改变现有字体渲染的 4 个点：
     `SettingsDns.vue:312` 与 `SettingsNetwork.vue:404` 的 `.modern-textarea { font-family: monospace }`、
     `SettingsAbout.vue:446` 的 `.link-text { font-family: monospace }`、
     CodeMirror 基础主题给 `.cm-scroller` 的 `font-family: monospace`（行号区会从 JetBrains Mono 掉回 Consolas）。
   - 正确顺序：先把这几处改成 `var(--font-mono)`（组件代理），再删 `!important`；否则就是拿一个
     全局字体约束换 4 处字体不一致。建议并进 Wave 3 R-B（设计系统）一起做。
3. **z-index 真实收口属 Wave 3**：5 档 `--ui-z-*` 零消费 token 已删，但真实层级仍是
   `style.css` 的 9999（模态）/10000（toast）+ `ModernSelect` 的 100/50。
   直接把模态改成 `var(--ui-z-modal)=30` 会让打开的下拉（100）盖到模态之上 —— 必须一起重排，见 R-B。
