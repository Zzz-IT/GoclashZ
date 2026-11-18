# R-1 交接清单（跨文件 / 跨权限事项）

> 本轮（r1-common）只改：
> `frontend/src/utils/alertError.ts`（新增）、`frontend/src/utils/storage.ts`（新增）、
> `Rules.vue`、`Subscriptions.vue`、`Proxies.vue`、`Connections.vue`。
> 下列事项需要其它文件权限或主控决策，本轮**未动**。

---

## 1. alertError 的剩余同类模板（不在本轮文件权限内）

同一模板已收敛的 6 处在 Rules/Subscriptions；以下组件里仍有同型/近型写法，建议后续用
`import { alertError } from '../utils/alertError'` 替换（**逐处核对标题/isDanger/正文**）：

| 文件:行 | 现写法 | 与 alertError 的差异 |
|---|---|---|
| Settings.vue:279 | `showAlert(t('settings.saveFailed', { error: displayErrorMessage(error) }), t('common.error'), true, u.detail)` | 完全同模板，可直接换 `alertError(error, 'settings.saveFailed')`（注意原来是 fire-and-forget，无 await） |
| Settings.vue:309 | 同上，key=`settings.resetFailed` | 同上 |
| SettingsUwp.vue:122 | 同模板，key=`settings.uwp.fetchFailed` | 可直接换 |
| SettingsUwp.vue:185 / 210 | 同模板，key=`settings.uwp.saveFailed` | 可直接换 |
| Settings.vue:80 | `showAlert(displayErrorMessage(e), t('common.error'), true, u.detail)` | 无 key，正文不是 t(key)。需新增「无 key」变体或改文案 |
| SettingsUwp.vue:103 | 同上（无 key） | 同上 |
| SettingsDiagnostics.vue:180 / 261 | 同上（无 key，`void`） | 同上 |
| SettingsUpdate.vue:102 / 103 | `t('settings.update.updateStartFailed', { name, error: u.message })` | 有 name 参数（alertError 的 params 可承载），但正文用 `u.message` 而非 `displayErrorMessage` → 文案口径不同，需评审 |
| SettingsAbout.vue:283 / 323 / 387 | `showAlert(u.message, t('common.error'), true, u.detail)` | 无 key（`u.message` 变体） |
| SettingsDns.vue:242 | `void showAlert(u.message, t('common.error'), true, u.detail)` | 同上（文件在本轮权限内，但按「纯重构」保留） |
| Subscriptions.vue:405 / 726 | `await showAlert(u.message, ...)` | 同上（文件在本轮权限内，保留原因见 fix-r1-common.md §1.4） |
| Overview.vue:272 / 326 | `showAlert(t(key, { error: displayErrorMessage(err) }), t('common.error'), true, u.detail)` | 完全同模板（变量名 err；272 无 await），可直接换 |
| YamlEditor.vue:342 / 391 | `showAlert(t(key, { error: errorMsg }), t('common.error'), true)` | 正文是原始错误、无 detail；换成 alertError 属行为变化，需单独评审 |
| YamlEditor.vue:388 | 标题 `common.warning` | 不属于该模板 |

**建议**：若要一次性收口，先给 `alertError` 增加两个明确的变体（例如 `alertUserError(e)` 走 `u.message`、是否带 detail 可选），
再逐个替换；不要在一个函数里用可选 key 分支，否则行为等价性很难复核。

---

## 2. localStorage 的其它私有包装 / 裸调用

`utils/storage.ts` 已提供 `safeGetString / safeSetString`，以下位置仍各有一份，建议后续统一：

| 位置 | 现状 | 备注 |
|---|---|---|
| `store.ts:52-66` | `readLocalStorage` / `writeLocalStorage`（私有） | 与本轮 storage.ts 逐行同义；store.ts 若 import storage.ts 无环（storage 不依赖 store），可安全替换 |
| `utils/theme.ts:220-234` | `readStorage` / `writeStorage`（私有） | 同上；文件头已有 U-09 说明 |
| `store.ts:248` / `utils/theme.ts:454-456` | 直接 `localStorage.setItem`，但各自包了 try/catch 或用写原语 | 已受保护 |
| `store.ts:881` | `localStorage.setItem('goclashz_outboundIP', ...)` **裸调用、无 try/catch** | 配额满/隐私模式会抛进 `refreshOutboundIP` 的异步链（S-05 同类风险），本轮权限外未改，建议优先处理 |
| `locales/index.ts:29 / 85` | 模块顶层 `getItem` 与 `setLocale` 里 `setItem` **均无 try/catch** | 语言缓存在 store 初始化链最早期执行，抛错会直接白屏（S-05/C-11 同类）；因 locales 不在本轮权限内未改 |

---

## 3. 本轮判断/未做事项

1. **Proxies 两个 onMounted 未合并**：见 `fix-r1-common.md` §3.5（错误传播路径不同，按「行为不变」保留 + 注释）。
2. **Proxies 注释与实现矛盾**（审计 §5 / P-13，如 1042 行附近仍写「36px 按钮 + 4px 滚动条」而实际已 `scrollbar-width:none`）本轮未改注释，避免超出「死代码」范围；建议随下次该文件改动顺手修。
3. **R-A 的其它两项未做**：useTabSlider（4 份同构）、useAsyncAction（11 处三件套）——属结构性重构，需独立任务。
4. **未跑构建**：vue-tsc / `pnpm build` 按要求留给主控统一执行；本轮仅做了 SFC 语法解析与 TS 语法转译检查。

---

## 4. 主控验收建议

1. `cd frontend && npx vue-tsc --noEmit`（重点看 Rules/Subscriptions 的未使用导入是否已清理干净）；
2. `node scripts/validate-settings-manifest.mjs` / `node scripts/check-i18n.mjs`（本轮不涉及 manifest/locales，预期无变化）；
3. 人工回归：
   - Rules 页：打开新增弹窗失败 / 添加失败 / 删除失败的弹窗（正文人话 + 折叠原文）；
   - Subscriptions 页：导入、重命名、删除失败弹窗；
   - 三页刷新后选项卡/分组/筛选模式的记忆（localStorage 路径）。
