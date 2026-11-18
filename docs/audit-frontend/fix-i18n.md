# Wave 2 · fix-i18n 交付报告

> 代理：**fix-i18n**（修复团队 Wave 2）
> 独占范围：`frontend/src/locales/`（8 个语言包）+ `locales/index.ts` + `locales/types.ts` + `scripts/check-i18n.mjs`
> 未改动任何组件 / store / util（`locales/index.ts`、`locales/types.ts` 本轮也无改动）。
> 日期：2026-11-18

---

## 0 结论摘要

| 项目 | 结果 |
|---|---|
| 新增 key | **23 个**（× 8 语言 = 184 行），覆盖 6 个 `i18n-needs-*` 文件 |
| 另按 needs 文件要求修正的既有 key 文案 | **2 个**（`settings.uwp.invertSelect`、`settings.about.restoreSuccess`，× 8 语言） |
| 删除真死键 | **102 个**（× 8 语言 = 每文件 102 行；另整块删除 `tray` 命名空间 2 行括号） |
| check-i18n 死键数 | **186 → 0**（任务书写 188，本轮实测基线 186） |
| check-i18n 缺键 error | **12 → 0** |
| 8 语言键集一致性 | **752 键 × 8 完全一致**，0 解析错误、0 重复键 |
| `validate-settings-manifest.mjs` | **exit 0**（103 个设置项锚点全部就位） |
| `check-i18n.mjs` | **exit 0**（死键 0，动态 key 提示 23 处） |

数字变化明细：

| 阶段 | 脚本 | 死键 | 缺键 error | baseKeys / baseLeaves |
|---|---|---|---|---|
| 基线（Wave 1 结束后，未补 key） | 旧脚本 | 186 | 12 | 830 / 796 |
| 补完 23 个新 key 后 | 旧脚本 | 199 | 0 | 855 / 819 |
| 修好判定逻辑后（未删任何 key） | 新脚本 | 102 | 0 | 855 / 819 |
| 删除 102 个真死键 + 2 处文案修正后 | 新脚本 | **0** | **0** | **752 / 717** |

> 旧脚本 186 条死键里，**84 条是误报**：34 条是命名空间对象（`common`、`settings.about` 这类容器本来就不会被 `t()` 整体引用），50 条是「数据表 / 动态拼接」在用（清单见 §2.4）。任务书估的「188 里约 143 条误报」是更早快照，本轮实测 84 条。

---

## 1 A 部分：补齐 Wave 1 各代理提出的新 key

### 1.1 总清单（23 个，8 语言同步、键顺序同构）

插入位置一律是「对应既有命名空间的末尾 / 指定锚点之后」，8 个语言包位置完全一致；未重排、未改动任何既有 key 的译文（除 §1.3 的两处按 needs 文件要求的语义修正）。

| # | key | 来源 needs 文件 | 代码消费点 |
|---|---|---|---|
| 1 | `settings.update.checkFailed` | i18n-needs-1 | `store.ts:1160` |
| 2 | `notify.systemProxyStateUnknown` | i18n-needs-1 | `store.ts:987`（**新增顶层命名空间 `notify`**，置于文件末尾） |
| 3 | `proxies.loadFailedHint` | i18n-needs-3 | `Proxies.vue:137`（紧跟 `proxies.noGroupsHint`） |
| 4 | `rules.configSwitched` | i18n-needs-4 | `Rules.vue:534,543` |
| 5 | `rules.ruleChanged` | i18n-needs-4 | `Rules.vue:549` |
| 6 | `subs.urlInvalid` | i18n-needs-4 | `Subscriptions.vue:426` |
| 7 | `subs.dupSubWarn` | i18n-needs-4 | `Subscriptions.vue:155` |
| 8 | `yamlEditor.backConfirmMsg` | i18n-needs-4 | `YamlEditor.vue:426` |
| 9 | `yamlEditor.saveReloadFailed` | i18n-needs-4 | `YamlEditor.vue:387-388` |
| 10 | `traffic.staleShort` | i18n-needs-6 | `TrafficCard.vue:22` |
| 11 | `traffic.staleHint` | i18n-needs-6 | `TrafficCard.vue:22,138` |
| 12 | `deviation.severity.ignored` | i18n-needs-6 | `DeviationStrip.vue:65-66`（`localized('deviation.severity', code)` 动态取） |
| 13 | `deviation.severity.forced` | i18n-needs-6 | 同上 |
| 14 | `logs.std` | i18n-needs-fix-2 | `App.vue:127`（插在 `logs.app` 之后） |
| 15 | `logs.levelFilter` | i18n-needs-fix-2 | `App.vue:137`（插在 `logs.clear` 之后） |
| 16 | `yamlEditor.leaveConfirmMsg` | i18n-needs-fix-2 | `App.vue:760,875` |
| 17 | `common.never` | i18n-needs-fix-7 | `utils/format.ts:42`（`tr()`） |
| 18 | `common.justNow` | i18n-needs-fix-7 | `utils/format.ts:49` |
| 19 | `common.minutesAgo` | i18n-needs-fix-7 | `utils/format.ts:52` |
| 20 | `common.hoursAgo` | i18n-needs-fix-7 | `utils/format.ts:56` |
| 21 | `common.daysAgo` | i18n-needs-fix-7 | `utils/format.ts:60` |
| 22 | `common.errorLogged` | i18n-needs-fix-7 | `utils/errors.ts:128` |
| 23 | `common.copyFailed` | i18n-needs-fix-7 | `utils/clipboard.ts:46` |

文案**逐字照抄** needs 文件，未自行改写；`{n}` / `{error}` 占位符全部保留（8 语言逐一核对）。
分组落位：`common`+7、`logs`+2、`traffic`+2、`proxies`+1、`subs`+2、`rules`+2、`yamlEditor`+3、`settings.update`+1、`deviation.severity`(+2)、`notify`(+1 新命名空间)。

### 1.2 收尾前新出现的 `i18n-needs-fix-5-settings.md`（20:31 新增）

开工时扫描只有 6 个 needs 文件，收尾前复扫发现第 7 个 `i18n-needs-fix-5-settings.md`，已按其中要求处理：

| 条目 | 处理 | 说明 |
|---|---|---|
| **G-03** `settings.uwp.invertSelect` | ✅ 已改 8 语言文案 | 「反选」→「全部不选 / Select None / …」（按文件表格逐字替换），键名保持不变（不改成 `selectNone`，避免动 `SettingsUwp.vue`） |
| **G-05** `settings.about.restoreSuccess` | ✅ 已改 8 语言文案 | 「已即时生效」→「设置已重新载入 / Settings have been reloaded / …」 |
| **G-11** `updateTask.actionFailed`（标注「可选 / 不急」） | ⏸ 未添加 | 文件明确写「不加也不会露 key 字面量」。新增后无人消费会立刻变成 1 个死键，而把它接上需要改 `UpdateTaskRow.vue:92`（不在本代理可改范围）。文案已在本报告记录，交由 `UpdateTaskRow.vue` 的归属方决定：`zh-CN 操作失败: {error}` / `zh-TW 操作失敗: {error}` / `zh-HK 操作失敗: {error}` / `en-US Action failed: {error}` / `ja-JP 操作に失敗しました: {error}` / `ru-RU Не удалось выполнить действие: {error}` / `fr-FR Échec de l'opération : {error}` / `de-DE Aktion fehlgeschlagen: {error}` |
| **G-22** 建议新增 `time.*` 5 键 | ⏸ 未添加 | 与先出现的 `i18n-needs-fix-7`（`common.never/justNow/minutesAgo/hoursAgo/daysAgo`）语义重叠；`utils/format.ts` 已实际消费 `common.*`，再加 `time.*` 会直接产生 5 个死键。**以先出现的 fix-7 为准**（见 §1.3 冲突记录） |

### 1.3 冲突与近似重复记录

| 情况 | 处理 |
|---|---|
| `time.never/justNow/…`（fix-5 建议）vs `common.never/justNow/…`（fix-7 已落地并在用） | **同一语义两套命名**：以先出现的 `i18n-needs-fix-7` 为准，只落 `common.*`，不加 `time.*`。若以后有人照 fix-5 补 `time.*`，会变成 5 个死键 |
| `yamlEditor.backConfirmMsg`（needs-4）vs `yamlEditor.leaveConfirmMsg`（needs-fix-2） | **不是冲突**，是两个不同调用点：YamlEditor.vue 编辑器内「返回」按钮 vs App.vue 侧栏切页/更新 chip 跳转拦截。两个 key 都保留，文案近似（一个问句在前、一个条件句在前）属正常措辞差异 |
| `subs.copyFailed`（早已存在）vs `common.copyFailed`（fix-7 新增） | 两 key 并存，取值一致（needs 文件本身就要求复用措辞），无冲突 |
| 其余 6 个 needs 文件 | 无 key 名/文案冲突，无需裁决 |

---

## 2 B 部分：修好死键检测盲区，再清理真死键

### 2.1 旧脚本为什么会误报 84 条

旧 `scripts/check-i18n.mjs` 的判定是：

- 用 AST 收集语言包**所有**点分路径（含 `common`、`settings.about` 这类命名空间对象）→ 得到 830 个「key」；
- 代码侧只认两种引用：AST 里 `t('字面量')` 调用 + `.vue` 文本正则 `\bt\(\s*['"]…`；
- 死键 = 830 全集中未被上述两种方式引用的。

盲区因此有四个：

1. **命名空间对象必然被算死**（34 条）——`t()` 永远不会整体引用 `subs`；
2. **数据表间接引用看不到**（`settings/manifest.ts` 的 `titleKey: 'settings.dns.enableDns'`、`utils/theme.ts` 的 `nameKey: 'settings.theme.presets.nord'`、`SettingsHome.vue` 的 `{ titleKey, descKey }` 数组、`Connections.vue` 的 DNS 来源映射表、`store.ts` 的通知码表）；
3. **前缀 + 拼接看不到**（`'diag.note.' + code`、`'deviation.severity.' + code`、`localized('deviation.field', field)`）；
4. **`tr()` 封装看不到**（`utils/format.ts`、`utils/errors.ts`、`utils/clipboard.ts`）——needs-fix-7 早就点过这条。

### 2.2 新判定（脚本改动说明）

保留原有结构（AST 解析语言包 / 遍历源码 / error 退出码），替换死键口径：

1. **只有「值本身是字符串」的叶子才进入死键候选**；命名空间对象不再参与（直接消掉 34 条误报）。
2. 代码引用收集改为三路：
   - **AST 字面量**：遍历所有 `StringLiteral` / `NoSubstitutionTemplateLiteral`（`.ts` 全文 + `.vue` 的每段 `<script>`）：
     - 与叶子 key 全等 → 已用（覆盖 `titleKey`/`nameKey`/错误码常量表等数据表引用）；
     - 以 `.` 结尾 → 记为**拼接前缀**（`'diag.note.' + code` 里的左操作数就是一个以点结尾的字面量）；
     - 含点且等于某个命名空间 → 记为**整族前缀**（`localized('deviation.field', field)`）；
   - **模板字符串头**：`TemplateExpression` 的 `head.text`（例：模板串 "diag.note.${code}" 的 head 部分是 "diag.note."）记为前缀；
   - **`.vue` 的 `<template>`**：TS AST 看不到模板，改用点分 token 扫描 `/[A-Za-z0-9_$][A-Za-z0-9_$-]*(?:\.[A-Za-z0-9_$][A-Za-z0-9_$-]+)+\.?/g`。**不依赖引号配对**——上一版用「引号内取字面量」的正则在 `split('\\')` / `don't` 这类地方会错位，实测把 `t('settings.about.browseBackupFile')` 整条漏掉。
3. **前缀必须能真正匹配到既有 key 才生效**（`addPrefix` 会校验），避免把源码里的 `2.`、`p.` 之类噪声当成前缀族。
4. **单段命名空间字面量不算引用**：`settings/manifest.ts` 的关键词表里有 `'tray'`、`'settings'` 这种普通单词，若不排除会把 `tray.*` 整族「洗白」（实测过，会直接漏掉 11 个真死键）。
5. `t()` 的间接封装纳入「key 必须存在」检查：`codeKeyRefs` 的 WRAPPERS 从 `['t']` 扩展为 `['t','tr','localized']`，`.vue` 兜底正则同步为 `\b(?:t|tr)\(`。
6. 死键 warn 改为**输出完整清单**（原来只打前 30/60 条，清理时会被截断误导）。

保持不变：`error` 两项检查（「代码使用但基准语言未定义」+「某语言缺少基准语言的 key」）、exit 0/1 语义、动态调用 info 列表。新增输出字段 `baseLeaves`（叶子数），便于对照。

脚本开销：单次 `node scripts/check-i18n.mjs` ≈ **430 ms**（8 个语言包 + 60 个源码文件）。

### 2.3 删除的真死键（102 个，8 语言同删）

判定标准（与任务书一致）：**新脚本仍判死** + **全仓（排除 `locales/`，含 `frontend/`、`cmd/`、`core/`、`*.go`、`.vue`、`.ts`、`.ps1`、`.json`…）grep 该 key 字面量 0 命中**。
实测：102/102 在代码与构建脚本中 0 命中；其中 23 个只在 `docs/`、`.task/`、`.fixbackup/`（历史审计与备份）里出现过，属「功能已改掉、文档/备份留痕」。

| 命名空间 | 数量 | 被删 key |
|---|---|---|
| `common` | 10 | `close`, `delete`, `disabled`, `edit`, `enabled`, `refresh`, `rename`, `reset`, `retry`, `saveSuccess` |
| `connections` | 13 | `closeAllSuccess`, `colDuration`, `colHost`, `colNetwork`, `colProcess`, `colRule`, `colTraffic`, `downSpeed`, `filterPlaceholder`, `noConns`, `title`, `totalConns`, `upSpeed` |
| `diag` | 2 | `dnsFakeIp`, `uncertain` |
| `overview` | 1 | `restartError` |
| `proxies` | 5 | `noGroupsTitle`, `noMatchingNodes`, `noProfileTitle`, `switchSuccess`, `testing` |
| `rules` | 6 | `colPayload`, `colProxy`, `colType`, `loadError`, `title`, `totalRules` |
| `settings.about` | 6 | `accessBtn`, `downloadFinished`, `newVersionReady`, `remainingTime`, `repo`, `repoDesc` |
| `settings.behavior` | 9 | `glassBlurContent`, `glassBlurContentDesc`, `glassBlurControl`, `glassBlurControlDesc`, `glassBlurDesc`, `glassBlurOverlay`, `glassBlurOverlayDesc`, `min5`, `sec60` |
| `settings.dns` | 1 | `fallbackGeoipCode` |
| `settings.theme` | 8 | `accentDesc`, `aura`, `auraDesc`, `modeDesc`, `presetDesc`, `previewHint`, `previewTitle`, `radiusDesc` |
| `settings.tun` | 3 | `missingDriverMsg`, `missingDriverTitle`, `tunError` |
| `settings.update` | 1 | `updateTaskTitle` |
| `subs` | 21 | `applySuccess`, `confirmImportBtn`, `createBlank`, `createBlankDesc`, `createBtn`, `deleteConfirmMsg`, `deleteConfirmTitle`, `exportSuccessNotice`, `importFromFile`, `importFromFileDesc`, `importFromUrl`, `importFromUrlDesc`, `importTitle`, `namePlaceholder`, `pickFileBtn`, `updateDone`, `updateError`, `updateFailedMsg`, `updateResultTitle`, `updateSuccessNotice`, `updateSuccessTitle` |
| `tray` | 11 + 整块 | `direct`, `global`, `noConfig`, `outbound`, `portProxy`, `quit`, `restart`, `rule`, `show`, `sysProxy`, `tun`（11 个全删 → 命名空间整块移除） |
| `yamlEditor` | 5 | `formatBtn`, `loadFailed`, `saveSuccess`, `syntaxInvalid`, `title` |

**重点核实（任务书点名 / 高风险）**

- `tray.*`（11 个）：前端 `frontend/src` **0 引用**（只有 `settings/manifest.ts` 搜索关键词里有 `'tray'` 这个普通单词，已排除误判）。托盘已迁到 Wails v3 原生实现：`tray_runtime_windows.go` 自己带文案（`txt.sysProxy` / `view.show` …），全套 Go 侧 `*.go` grep `tray.quit` 等 key 也是 0 命中。→ 前端 `tray` 段确认为死键，整块删除。
- `yamlEditor.loadFailed` / `syntaxInvalid`：needs-4 明确列为「因修复而变成死键」，且 Wave 1 已删掉写它们的 `statusText`，已删除。
- `settings.behavior.glassBlurContent/Overlay/Control(+Desc)`、`glassBlurDesc`：`Settings.vue:172` 注释明写「4 个分级玻璃强度字段前端已无消费者，不再列出（G-23）」，已删除。
- `connections.col*` / `rules.col*` / 表头类 key：页面已改卡片/分页布局，模板里再无表头引用，已删除。
- `subs.*` 21 个（导入向导那批）：`Subscriptions.vue` 现在用的是 `importModalTitle` / `browseFile` / `linkPrefix` 等新 key，旧的导入引导块已不存在，已删除。
- **未删除**的近似项（仍在使用，保留）：`subs.sortBtn` / `sortModeHint` / `doneBtn`（模板仍引用，仅 `v-if` 隐藏，等后端 SaveSubOrder 落地）、`yamlEditor.saveToError/readError/saveFileError/reloadConfirmMsg` 等。
- 通用动作文案（`common.close/delete/edit/retry…`）虽属「以后可能复用」，但当前 0 引用且无后端拼接可能（后端只回错误码/消息，不回前端 key），按判定标准删除；风险见 §4。

### 2.4 被新判定「洗清」的 50 个误报（不删，仍在使用）

以下 50 条旧脚本报死、实际有引用（数据表 / 动态拼接），新脚本已自动识别为「在用」，**均未删除**：

```
deviation.field.* (3) + deviation.reason.* (3)
diag.dnsSourceDefault/Policy/Fallback/Main (4) + diag.note.* (9)
settings.home.* (13，SettingsHome.vue 的 titleKey/descKey 数组)
settings.theme.presets.* (12，utils/theme.ts 的 nameKey)
settings.theme.modeDark/modeLight/modeSystem、radiusRound/radiusSharp/radiusSoft (6)
```

---

## 3 C 部分：收尾自检

均在本机 `frontend/` 目录下执行（未跑 vue-tsc / pnpm build，交给主控）：

```
$ node ../scripts/validate-settings-manifest.mjs
[settings-manifest] OK，103 个设置项锚点全部就位
→ exit 0

$ node ../scripts/check-i18n.mjs
[i18n] locales=8 codeKeys=667 baseKeys=752 baseLeaves=717 dynamicCalls=23
[i18n] 动态 key 调用（无法静态校验，需保证对应文案存在）：   ← 23 处，仅提示
  ...
[i18n] OK：语言对齐 + 字面量 key 全部存在（动态 key 仅提示）
→ exit 0（无死键 warn）
```

**8 语言键集一致（一次性脚本，未入库）**：用 TypeScript AST 展开每个语言包的全部点分路径并互相求差集，同时统计解析错误与重复 key：

```
基准 zh-CN：keys=752 解析错误=0 重复=0
OK   zh-CN  keys= 752 parseErr=0 dup=0
OK   zh-TW  keys= 752 parseErr=0 dup=0
OK   zh-HK  keys= 752 parseErr=0 dup=0
OK   en-US  keys= 752 parseErr=0 dup=0
OK   ja-JP  keys= 752 parseErr=0 dup=0
OK   ru-RU  keys= 752 parseErr=0 dup=0
OK   fr-FR  keys= 752 parseErr=0 dup=0
OK   de-DE  keys= 752 parseErr=0 dup=0
=== 8 语言键集完全一致: 是 ===
```

结论：**8 个语言包键集完全一致（752 键），零解析错误、零重复键**；新增/删除在 8 个文件中的位置与键名逐一对齐（同一套脚本批量施加，非手工分文件编辑）。

---

## 4 回归风险

1. **`tray.*` 与后端驱动类 key 需保持「前端零引用」前提**
   删除 `tray.*`（11）与 `diag.dnsFakeIp`、`diag.uncertain`、`settings.tun.missingDriverMsg/missingDriverTitle/tunError` 的前提是「这些文案不再由前端渲染」。若以后 Wails 原生托盘改回「Go 侧传 key 给前端渲染」，或 TUN/DNS 诊断改由前端按后端 code 拼 key，需要把这批 key 补回来——静态校验只扫 `frontend/src`，看不到 Go 侧将要用到的前端 key。

2. **增强死键判定对「前缀来自运行时变量」仍有边界**
   `'a.b.' + x`、模板串 `a.b.${x}`、`localized('a.b', x)` 已覆盖；但 `t(prefix + code)` 里 `prefix` 若是函数参数 / 从接口返回值拼出来的字符串，静态仍看不到（会被判死）。同理 `.vue` 模板里的拼接只能靠 token 扫描兜底。若以后新增此类写法，请把它改成「字面量前缀 + 拼接」形式，或在同一 PR 里把 key 显式引用一次。

3. **通用动作文案被删后，新 UI 复原必须同 PR 补 key**
   `common.close/delete/edit/rename/refresh/reset/retry/enabled/disabled/saveSuccess` 已删；任何组件再写 `t('common.refresh')` 时，运行时会直接显示 key 字面量（`t()` 缺键回退 path），而 build 期 `check-i18n` 会以「代码使用但 zh-CN 未定义」**exit 1** 拦下。属「早失败」，但要求代码与语言包同一次改动落地。

---

## 5 遗留 / 后续建议

| 项 | 说明 |
|---|---|
| `updateTask.actionFailed`（G-11） | 文案已在 §1.2 记录；等 `UpdateTaskRow.vue:92` 改成 `t('updateTask.actionFailed', { error })` 时，同 PR 补 8 语言 key 即可（当前实现不露 key，可长期不加） |
| `time.*`（fix-5 G-22 建议） | 已被 `common.*` 取代，**不要**再补，否则是 5 个死键 |
| `subs.sortBtn/sortModeHint/doneBtn` | 模板仍引用（`v-if` 隐藏），等后端 SaveSubOrder 落地再评估 |
| 死键判定口径文档化 | 新规则已写进 `scripts/check-i18n.mjs` 文件头注释，后续新增动态 key 写法照 §4.2 处理 |
| diff 说明 | 每文件 `31 +` / `107 -`：+23 新 key 行、+4 结构括号（`severity` / `notify` 的开合行）、+2 中文注释、+2 改文案；−102 死键、−2 结构括号（`tray` 块）、−2 旧文案；另有 **1 条 `settings.behavior.glassTintNeutral` 的删除是本次会话开始前工作区里既有的改动（Wave 1 遗留），不是本代理所为**（HEAD 有该 key、开工基线已无） |

---

## 附：本轮改动的文件

| 文件 | 改动 |
|---|---|
| `frontend/src/locales/zh-CN.ts` … `de-DE.ts`（8 个） | +23 key（8 语言同构）、−102 死键、2 处文案语义修正（带中文注释）、行尾统一 CRLF（与仓库工作区一致） |
| `scripts/check-i18n.mjs` | 死键判定重写（叶子过滤 + 数据表字面量 + 前缀族 + `.vue` token 扫描 + `tr/localized` 封装），保留 error 检查与 exit 0/1 |
| `docs/audit-frontend/fix-i18n.md` | 本报告 |

未改动：`locales/index.ts`、`locales/types.ts`（API 无变化，无需改）、任何组件 / store / util。
