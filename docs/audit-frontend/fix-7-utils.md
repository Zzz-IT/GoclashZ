# 修复报告 · fix-7（utils / styles）

> 代理：fix-7-utils ｜ 范围：`frontend/src/utils/*.ts`、`frontend/src/styles/{tokens,surfaces,layout}.css`、`frontend/src/style.css`
> 依据：`docs/audit-frontend/05-utils-style.md` 的 U-03/05/06/07/08/09/10/11/12/13/14/15/16/18/19/22（+ U-04 的 token 部分）
> 未运行：`pnpm build` / `vite build` / `vue-tsc` / 应用本体（按硬性规则 5，类型检查由主控统一跑）。
> 已做的自查手段：8 个 TS 文件用 TypeScript 编译器 **只解析不落盘**（`ts.createSourceFile` + `parseDiagnostics`）语法校验全过；
> 所有被删符号 / 被删 token 都用 grep 逐个确认为零引用后才删。

---

## 1 状态总表

| ID | 状态 | 改动位置（file:line，行号为改后） | 一句话说明 |
|---|---|---|---|
| **U-03** | fixed（代码侧） | `utils/format.ts:1-9,41-64`；`utils/errors.ts:4-10,123-130` | 相对时间 5 档与「（已记录日志）」后缀改走 `t()`，key 缺失时回退旧中文（`tr()` 助手），补齐语言包前 UI 不变 |
| **U-05** | fixed | `utils/errors.ts:16-34` | `/helper\|Helper/i` 收窄为 `helper_install_required\|HELPER_NOT_INSTALLED\|helper_repair_required\|需要 Helper 服务\|Helper 服务未(安装\|启动\|注册)`，并把「请求超时」规则前移到它之前 |
| **U-06** | fixed | `utils/format.ts:26-38` | `formatEtaTime` 补 `>=3600s → "h m"` 档；0/负值/NaN 仍返回 `--`，<1h 行为不变 |
| **U-07** | partial | `utils/glass.ts:14-20,216` | 新增独立 `GLASS_TINT_MAX=80` 并用作染色换算除数；**调用点 `SettingsTheme.vue:219/277/586` 属 fix-5 文件**，已写 handoff（H5-1） |
| **U-08** | partial | `utils/theme.ts:205-234,451-477` | 收敛为 `cacheTheme/readCachedTheme` 单一读写 + 旧键 `goclashz_theme` 降级为镜像（由 cacheTheme 双写）+ 新增 `readCachedThemeModeLabel()`；**store.ts 的两处旧键读写属 fix-1 文件**，已写 handoff（H1-1） |
| **U-09** | partial | `utils/theme.ts:220-234` | 主题缓存的 localStorage 全部收进 `readStorage/writeStorage`（try/catch）；**store.ts 4 处、locales/index.ts 2 处不属本代理**，已写 handoff（H1-2） |
| **U-10** | fixed | `utils/hoverGuard.ts:27-51,53-68,70-85` | ①抑制期间重新收集新出现的 `:hover` 规则并去重；②restore 失败不再静默（`console.warn` + 留在 saved 重试，不再永久停在 `:not(*)`）；③删 focus 抑制、`visibilitychange` 只在 `document.hidden` 时抑制 |
| **U-11** | fixed | `utils/scrollFade.ts:38-51,53-80,88-138` | 观察器触发合并到一帧（rAF）；mutation 逐条过滤为「落在滚动容器内 / 新增移除的就是容器」；`initScrollFades()` 返回 dispose（+ `disposeScrollFades()`）；App 侧调用写 handoff（H2-2） |
| **U-12** | partial | `styles/tokens.css`（各定义处）、`style.css`（原 :135/:257）、`utils/theme.ts:429,436`、`utils/glass.ts:99-102,279` | 删 14 个零消费 token 名（5 档 `--ui-z-*`、`--ui-glass-tint/alpha/border`、`--glass-blur-content/control`、`--glass-highlight`、`--glass-fill-content`、`--ui-accent-hover`、`--status-online`）与对应写入；**保留 `--ui-radius-full`**（见 §2 说明）；z-index 真实收口留 Wave 3 |
| **U-13** | fixed | `utils/normalize.ts`（已删除） | 零 import 的孤儿文件整文件删除（删前 grep 全仓确认 0 引用） |
| **U-14** | fixed（按指令，与 C-22 冲突已上报） | `utils/report.ts:1-29` | 删 `records`/`MAX_RECORDS`/`getRecentErrors()`/`ErrorRecord`，只保留 `reportError`（console 留痕）；`06-contract.md` C-22 建议相反，见 §2 与 handoff §6 |
| **U-15** | fixed | `utils/glass.ts`（删 3 个）、`utils/format.ts`（删 1 个） | 删 `GLASS_BLUR_OVERLAY_DEFAULT`/`GLASS_BLUR_CONTROL_DEFAULT`/`normalizeGlassBlurStrength`/`formatDurationCn`（删前逐个 grep 确认零引用） |
| **U-16** | fixed | `style.css:224-227`（替为说明注释）、`tokens.css`（删守卫块） | 删 `.dark{--danger}`、`html:not([data-theme-mode]) .dark` 两块（守卫永远为假）、`@keyframes fadeIn/pulse`（组件 scoped 各有同名定义） |
| **U-18** | partial | `utils/clipboard.ts`（新建） | 统一实现 `copyToClipboard/copyText` 落地；**3 个调用方（App.vue / SettingsDiagnostics / Subscriptions）不属本代理**，已写 handoff（H2-1/H4-1/H5-3） |
| **U-19** | 驳回（保留两套，理由见 §2） | `utils/hoverGuard.ts`（保留并修好） | 复核认为 `clearCtrlHover`（JS hover 态）与 `hoverGuard`（CSS `:hover`）职责不同，不是重复机制；不删 hoverGuard，也不动 App.vue |
| **U-22** | partial | `style.css:722`、`surfaces.css:10-21`、`layout.css:25-54`、`style.css:1449-1451`、`tokens.css:54-95`、`glass.ts:104-115` | ①`.action-btn` 重复 `gap` 删前者 ②`.ui-surface-group` 重复 `background` 拆成两条 ③`.ui-page-body` 两段合并 ④注释/常量漂移（82%→46%、28px→40px、四级→三级）⑤**`*{font-family:!important}` 未去**（依赖已确认，见 §2） |
| **U-04** | partial | `styles/tokens.css:41-47` | 只在 tokens.css 保证 `--ui-radius-*` 体系完整（sm/md/lg/full 全在，full 加注"等 Wave 3 铺开"），未改任何组件 |

**统计：fixed 9 · partial 7 · 驳回 1（共 17 条）。**

---

## 2 需要说明的四个决策

### 2.1 U-12 里为什么保留 `--ui-radius-full`

审计把 `--ui-radius-full`（`999px`）算进"零消费"清单，但本轮 U-04 的约束是
「在 tokens.css 确保 `--ui-radius-*` 体系完整可用，圆角铺开留 Wave 3」。
把药丸档删掉，等于让 Wave 3 重新补一遍 token，还会让"体系完整"这条自相矛盾。
处理：保留定义，并在注释里写明它当前无消费者、是 Wave 3 的落点（`tokens.css:45-46`）。
其余 14 个死 token 已删（含 5 档 `--ui-z-*`），并在 `tokens.css:91-97` 留了一段
「这些 token 为什么删、z-index 真正的收口为什么留到 R-B」。

### 2.2 U-14 与 06-contract C-22 冲突

- 本轮指令：删缓冲与 `getRecentErrors`。
- `06-contract.md:1103` 与 Q7：**不要删**，应把最近错误并入 `ExportDiagnostics`（需后端追加字段）。
- 已按本轮指令执行（删），并在 `report.ts:10-14` 写清"若要恢复，应在 `reportError` 内部做落盘/转发，
  不要再引入只写不读的缓冲"。**需主控裁决**：若采纳 C-22，Wave 2 在 `reportError` 内部加转发即可，
  调用点（59 处）不用动。

### 2.3 U-19 驳回理由

`App.vue:336-339/773-777` 的 `clearCtrlHover` 清的是 `ctrlHover` 这个 **JS hover 态**
（窗口控件用 `@mouseenter/@mouseleave`，模板里根本不走 CSS `:hover`）；
`hoverGuard` 改的是**全站 CSS `:hover` 选择器**。两者作用对象不同，删任一个都会留下另一半问题。
故保留 hoverGuard（并按 U-10 修好），App.vue 侧不动；若要合并需给 `installHoverGuard` 加回调参数
（改导出签名），已列为待裁决项（handoff §2 H2-3）。

### 2.4 U-22⑤ `font-family: !important` 没去掉

硬性规则要求"去掉前先确认没有依赖"。确认结果：**有依赖**。
去掉后这 4 处会从 JetBrains Mono 掉回系统等宽字体（因为它写的是 `monospace` 关键字而不是 `var(--font-mono)`）：

| 位置 | 现状 | 去掉 !important 后 |
|---|---|---|
| `SettingsDns.vue:312` `.modern-textarea` | 实际渲染 JetBrains Mono | 变成 `monospace`（Consolas） |
| `SettingsNetwork.vue:404` `.modern-textarea` | 同上 | 同上 |
| `SettingsAbout.vue:446` `.link-text` | 同上 | 同上 |
| CodeMirror 基础主题 `.cm-scroller`（行号/滚动区） | 同上 | 同上（`.cm-content` 有主题里的 `var(--font-mono)`，不受影响） |

正确顺序是先把这 4 处改成 `var(--font-mono)`（属组件代理 / Wave 3 R-B），再删 `!important`。
本轮不拿"一处全局约束"去换"四处字体不一致"。

---

## 3 样式层跨文件覆盖关系（哪条规则从哪移到哪）

> styles 三个文件与 style.css 有层叠交叠，这里逐条记录本轮的结构变化，供评审核对。

| 规则 | 改前 | 改后 | 计算值 |
|---|---|---|---|
| `.ui-surface-card` / `.ui-surface-group` 的 `background` | **一条**合并规则写 `var(--ui-surface-card)`，紧跟一条 `.ui-surface-group` 覆盖成 `var(--ui-surface-group)`（`surfaces.css:10-18`） | 拆成两条规则各自声明一次（`surfaces.css:10-21`） | **不变**（分组仍是 `var(--ui-surface-group)`；玻璃模式下两者都取 `--glass-fill-card`） |
| `.ui-page-body` 布局声明 | 两个同选择器块：`layout.css:25-43`（渐隐/mask）与 `:82-91`（flex/overflow/padding） | 合并成一条 `layout.css:25-54` | **不变**（两段属性集合不重叠，无覆盖关系） |
| `--status-online` | `:root` 定义（`style.css:135`）+ `theme.ts` 每次切主题写入 | 全删（零 `var()` 消费） | 无视觉影响 |
| `html:not([data-theme-mode]) .dark` | `style.css:238-258` 与 `tokens.css:120-132` 各一份 | 全删（`bootstrapTheme()` 早于 `createApp`，守卫永远为假） | 无视觉影响 |
| `@keyframes fadeIn/pulse` | `style.css:711-733` 全局定义 | 删除；消费方（`SettingsLan/Network/TrafficCard`）各自的 scoped 块里有同名定义 | 无视觉影响（scoped keyframes 会被加哈希） |
| `--glass-fill-content` | `tokens.css:91` 兜底 + `glass.ts:282` 写入 | 全删（零读取方，第 4 级填充已废弃） | 无视觉影响 |
| `--glass-blur-overlay` | 40px，被 `style.css:322-325/1420-1435/1494-1497` 消费 | 保留 40px，仅改注释（原文写的是 28px） | 不变 |
| `--ui-radius-*` | `tokens.css:43-46` | 保留，`full` 加注释说明留待 Wave 3 | 不变 |

---

## 4 自查证据

- **死 token**：删除前逐个 `grep 'var\(--xxx\)'` 确认 0 引用；删除后复查
  `--ui-z-|--ui-glass-|--glass-highlight|--glass-fill-content|--glass-blur-content|--glass-blur-control|--ui-accent-hover|--status-online`
  在 `frontend/src` 只剩本轮新写的说明注释。
- **死导出/死文件**：`formatDurationCn`、`GLASS_BLUR_OVERLAY_DEFAULT`、`GLASS_BLUR_CONTROL_DEFAULT`、
  `normalizeGlassBlurStrength`、`getRecentErrors`、`ERROR_LOGGED_SUFFIX`、`normalizeStartupTaskInfo`
  在 `frontend/src` 均 0 引用（只剩本轮注释里的说明文字）。
- **类型面**：未新增 `any`（`clipboard.ts`/`scrollFade.ts` 全部显式类型）；未改任何既有导出签名
  （`installHoverGuard(): () => void` 不变；`initScrollFades` 由 `void` 变 `() => void`，调用语句不受影响；
  `displayErrorMessage` / `formatEtaTime` / `formatRelativeTime` 签名不变）。
- **无调试残留**：新增的 `console.warn` 只出现在 `hoverGuard` 的失败分支（U-10 要求"不要静默吞"），
  没有临时日志、临时文件、备份文件。
- **行尾**：`.gitattributes` 为 `eol=lf`，改动文件与索引一致（`git ls-files --eol` 复核）。

---

## 5 回归风险（3 条）

1. **U-05 正则收窄的降级面**：此后只有 `helper_install_required`/`HELPER_NOT_INSTALLED`/`helper_repair_required`/
   「需要 Helper 服务」/「Helper 服务未安装·未启动·未注册」会显示「需要 Helper 服务」。
   Go 侧若有别的路径只吐裸 `helper` 字样（既无 code 也无这些短语），提示会退化成「原文 + （已记录日志）」。
   影响面：人话层（`detail` 仍保留原文），最坏情况是用户看到一句英文原文而不是误导性的"去装 Helper"。
   彻底收口建议由 fix-backend 补结构化 error code。
2. **hoverGuard 语义变化**：`focus` 不再触发抑制、`visibilitychange` 只在隐藏时抑制 —— 切回窗口后若鼠标未动，
   浏览器可能仍保留个别元素的 `:hover` 高亮（旧版是靠"切回时把所有 hover 打死"掩盖的）。
   这是有意的行为更正（原逻辑会让纯键盘用户切回后全站 hover 失效），但观感与旧版不同；
   另外 `restore` 失败改为每次 mousemove 重试 + `console.warn`，若某条第三方样式表的选择器只读，
   控制台会周期性出现该警告（可控，但属新噪声）。
3. **scrollFade 的 mutation 过滤**：现在只对「落在滚动容器内 / 新增移除的节点是容器」的 mutation 重算。
   如果某个容器的 `scrollHeight` 是被**容器外部**的布局变化（例如页头高度、全局字号）撑出来的，
   底边渐隐要等到下一次滚动 / 窗口 resize 才更新（窗口 resize 已覆盖）。容器内用 `v-show`/展开类
   改高度的情况仍会触发（`style`/`class` 属性变化在容器内被保留观察）。
