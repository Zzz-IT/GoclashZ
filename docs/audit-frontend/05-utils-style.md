# 05 · 工具层与样式审计（utils/* / styles/*.css / style.css / locales / 全前端冗余普查）

> 审计者：utils-style（前端只读审计代理，task-5）
> 时间：2026-11-18 20:08 (+08:00)
> 判据：① 代码冗余 ② 逻辑错误 ③ 操作冲突 ④ bug ⑤ 前后端对接
> **只读审计：本报告未修改任何源码 / 样式 / 配置 / locales。**

---

## 0 元信息

### 0.1 审计范围

| 范围 | 文件 |
|---|---|
| 核心（逐行读完） | `frontend/src/utils/{theme,glass,errors,format,normalize,icons,hoverGuard,scrollFade,edgeFade,report}.ts` |
| 样式层（逐行读完） | `frontend/src/style.css`、`frontend/src/styles/{tokens,surfaces,layout}.css` |
| i18n（读完框架 + 机器比对 8 包） | `frontend/src/locales/{index,types}.ts` + 8 个语言包（键集，不逐条读文案） |
| 全前端普查（脚本 + 定点 grep） | `frontend/src` 下 62 个文件（28 ts / 28 vue / 4 css / 2 资源） |
| 交叉阅读（为回答"两份主题状态是否冲突"） | `store.ts:1-420 / 930-1251`、`App.vue:1-40 / 55-185 / 240-360 / 700-1030`、`components/Settings.vue`（全 258 行）、`components/SettingsTheme.vue:180-435 / 434-684`、`composables/useTunToggle.ts`（全 139 行）、`components/Overview.vue:230-320`、`bindings/goclashz/core/appcore/models.ts:1-174`、Go 侧 `core/appcore/controller.go:2395-2485`、`core/appcore/behavior.go:18-88`、`app-notification` 生产端 |

### 0.2 实际读过的行数（供覆盖度核对）

| 文件 | 总行 | 读过 |
|---|---|---|
| utils/theme.ts | 455 | 455（100%） |
| utils/glass.ts | 300 | 300（100%） |
| utils/errors.ts | 149 | 149（100%） |
| utils/format.ts | 50 | 50（100%） |
| utils/normalize.ts | 19 | 19（100%） |
| utils/icons.ts | 65 | 65（100%） |
| utils/hoverGuard.ts | 73 | 73（100%） |
| utils/scrollFade.ts | 65 | 65（100%） |
| utils/edgeFade.ts | 95 | 95（100%） |
| utils/report.ts | 40 | 40（100%） |
| style.css | 1523 | 1523（100%） |
| styles/tokens.css | 145 | 145（100%） |
| styles/surfaces.css | 43 | 43（100%） |
| styles/layout.css | 102 | 102（100%） |
| locales/index.ts + types.ts | 106 | 106（100%） |
| locales × 8 语言包 | 867 × 8 | 键集 100% 机器解析（未逐条读文案） |

### 0.3 未覆盖部分（诚实清单）

1. **8 个语言包的文案质量**：只做键集/死键的机器比对，未逐条通读 867×8 行译文（历史 F-A4-17 的"14 处日文未译"本轮未复核）。
2. **运行时视觉验证**：禁止运行应用，下列结论靠 CSS 层叠规则与框架契约推断，已在条目内标注证据等级：scoped 样式对 `@keyframes` 的重命名、`mask-image` 在 WebView2 的实际渲染、玻璃染色观感、focus 环失效的可见性。
3. **组件内部实现**（模板/逻辑）属其他代理范围；我只做跨文件去重统计 + "调用点是否用错工具函数"两类检查。
4. `settings/manifest.ts` 的逻辑未审（只在 i18n key 层面校验了 125 个动态 key）。
5. `store.ts:420-930` 未读（store 主体属其他代理范围；我只读了主题相关段与 initStore 段）。

### 0.4 证据等级

- **A** = 代码可直接确证（读到的行 + 语言/框架的确定语义，如 CSS `!important` 优先于特异性、`var()` 惰性求值）。
- **B** = 代码 + 运行时推断（需要某个运行条件成立，如 localStorage 抛错、走到某分支）。
- **C** = 假设待实测（本次禁止运行，必须标注）。

### 0.5 手段

- **导出符号引用统计脚本**：131 个 export 逐个用词边界正则统计跨文件引用 → 零引用 / 仅文件内引用清单。
- **CSS 规则提取器**（注释剥离 + 括号扫描）：29 个 style 块 / 1138 条规则 → 同名选择器去重、`!important` 计数、`@keyframes` 重名、自定义属性多重定义。
- **token 消费统计**：79 个自定义属性逐个统计 `var(--x)` 引用点。
- **i18n**：自写嵌套键解析器（与 `scripts/check-i18n.mjs` 的 AST 口径交叉验证），比对 8 包键集、抽取全部 `t('字面量')`、校验 `manifest.ts` 动态 key。
- **代码卫生**：`console.*` / `TODO` / `@ts-ignore` / `as any` / `debugger` / 空 catch / `: any` 分布统计。
- 只读跑了 `node scripts/check-i18n.mjs`（项目自带校验，不写文件）。

---

## 1 问题总表

| ID | 严重度 | 类别 | 位置 | 一句话问题 | 证据 |
|---|---|---|---|---|---|
| U-01 | **P1** | 操作冲突 | store.ts:134-145 / Settings.vue:202-216 / SettingsTheme.vue:558-563 / Go controller.go:2405-2408 | 设置页用**陈旧 behavior 快照整体覆盖**写库，把侧栏刚切换的 themeMode 顶回去（两个入口写同一后端配置） | A+B |
| U-02 | **P1** | bug / 样式冲突 | style.css:75-80 ↔ SettingsTheme.vue:747,780,826,901,929 | 全局 `outline:none!important` 打死主题页 5 处 `:focus-visible` 焦点环，全应用无键盘焦点指示 | A |
| U-03 | **P1** | i18n / 逻辑 | format.ts:37-50、errors.ts:109、locales/index.ts:53 | 工具层硬编码中文（"刚刚/分钟前/从未/（已记录日志）"），8 语言下固定显示中文 | A |
| U-04 | **P1** | 逻辑 / 设计系统 | theme.ts:412-415、tokens.css:43-45、surfaces.css:13/32 | 「圆角」设置只被 8 处 `var(--ui-radius-*)` 消费，155 处写死 `border-radius` → 设置半失效 | A+B |
| U-05 | P2 | 逻辑错误 | errors.ts:18 | helper 正则过宽（详见 §U-05）：含 helper 的超时/占用错误都被显示成「需要 Helper 服务」 | A |
| U-06 | P2 | 逻辑错误 | format.ts:16-21 | `formatEtaTime` 无小时档：3600s → 60m 0s、86400s → 1440m 0s | A |
| U-07 | P2 | 逻辑错误 | glass.ts:218、SettingsTheme.vue:586 | 染色上限借用「模糊上限」`GLASS_BLUR_MAX`（两处），两个语义量共用一个常量 | A |
| U-08 | P2 | 操作冲突 | store.ts:70/117/229 ↔ theme.ts:203/421-437 | 明暗状态有两条缓存键 + 三个真源，首帧可能自相矛盾 | A+B |
| U-09 | P2 | bug | locales/index.ts:29/85、store.ts:69-75/346/402-405 | 启动期与状态同步里的 `localStorage` 无 try/catch（同仓库其它 4 处都有） | A+B |
| U-10 | P2 | bug | hoverGuard.ts:27-52 | 抑制期间不重收集新样式、`restore` 失败被吞后 `:hover` 永久变 `:not(*)`、focus/visibility 无条件抑制 | A+B |
| U-11 | P2 | 性能 / 冗余 | scrollFade.ts:52-64 | 对整个 body 子树观察 `class/style`，任意类名变化触发全文档 2 次 `querySelectorAll`；观察器永不 dispose | A+B |
| U-12 | P2 | 冗余 / 死代码 | tokens.css:11/46/55-57/66/68/91/98/101-105、style.css:135/584/949、theme.ts:398/405、glass.ts:282 | 14 个自定义属性零消费（含完整 5 档 z-index 体系），同时硬编码 9999/10000 两套层级 | A |
| U-13 | P2 | 死代码 | utils/normalize.ts（全 19 行） | `normalizeStartupTaskInfo` 零引用，全前端无任何 StartupTask 调用，整文件是孤儿 | A |
| U-14 | P2 | 死代码 | report.ts:18-19/38-40 | 错误环形缓冲只写不读（`getRecentErrors` 零引用），50 条常驻对象永无出口 | A |
| U-15 | P2 | 死代码 | glass.ts:11/13/15/44、format.ts:23-35 | 4 个死导出：`GLASS_BLUR_OVERLAY_DEFAULT` / `GLASS_BLUR_CONTROL_DEFAULT` / `normalizeGlassBlurStrength` / `formatDurationCn` | A |
| U-16 | P2 | 死代码 | style.css:234-236/238-258/711-733、tokens.css:120-132 | 约 50 行永不匹配的 CSS 兜底块 + 2 个无消费者的 `@keyframes` | A+B |
| U-17 | P2 | 冗余 | Settings*.vue × 11 | 11 个设置子页复制同一套 scoped 样式（约 500 行）与同一段 `nextTick→双 rAF` 时序 hack | A |
| U-18 | P2 | 冗余 | App.vue:576-581/586-596、SettingsDiagnostics.vue:245-251、Subscriptions.vue:435-440 | 「复制到剪贴板」3 套实现，失败文案不一致（一处硬编码 `✗`，一处 i18n） | A |
| U-19 | P2 | 冗余 | App.vue:336-339/773-777 ↔ hoverGuard.ts:54-63 | 同一「清理窗口切换后残留 hover」两套机制并存 | A |
| U-20 | P2 | 前后端对接 | Settings.vue:166、bindings/core/appcore/models.ts:8-174 | 前端兜底 behavior 缺 4 个后端字段，Get 失败后任意保存会把它们清零 | A+B |
| U-21 | P2 | i18n | locales/tray.*、scripts/check-i18n.mjs:93-104 | `tray.*` 11 键前端零引用；校验脚本对动态 key 是盲区（188 条"死键"里 143 条是误报） | A |
| U-22 | P2 | 冗余 / 漂移 | style.css:260-265/761-778/1500-1513、tokens.css:63-93、surfaces.css:10-18、layout.css:25/82 | 同规则内重复声明 + 注释/常量与实际值漂移 + `*{font-family:!important}` 使字体覆盖失效 | A |
| U-23 | P2 | bug（低危） | App.vue:154 | 日志列表 `v-for` 用索引作 key，过滤/环缓冲淘汰后 DOM 复用会错位 | B |

**统计：P0 = 0，P1 = 4，P2 = 19。**

---

## 2 逐条详情

### U-01（P1，操作冲突）设置页保存时用陈旧 behavior 快照整体覆盖，顶掉侧栏刚切换的明暗

**位置**
- `frontend/src/components/Settings.vue:170-216`（loadBehavior / saveBehavior）
- `frontend/src/store.ts:134-158`（setThemeMode / syncThemeFromBehavior）
- `frontend/src/components/SettingsTheme.vue:558-563`（commitGlass 只写玻璃字段就 save）
- 后端：`core/appcore/controller.go:2405-2410`（`SetAndSave(b)` 整结构覆盖）

**现状**

```ts
// Settings.vue:202-216
const saveBehavior = async () => {
  try {
    await App.SaveAppBehavior(behavior.value);
    if (behavior.value.language) setLocale(behavior.value.language as SupportedLocale);
    ...
  } catch (error: any) { ... await loadBehavior(); }
};
```

```ts
// Settings.vue:183-189  进页时只同步外观，刻意【不回写】behavior
behavior.value = value;
// 后端配置（重置 / 托盘侧改动 / 语言切换触发的 reload）重新加载后，
// 主题与玻璃必须跟着重算，否则界面会停在旧外观。统一走 syncThemeFromBehavior。
syncThemeFromBehavior(behavior.value);
```

```ts
// store.ts:152-158  注释明确"不回写 behavior，避免把默认值写库"
export function syncThemeFromBehavior(behavior: any): boolean {
  applyGlassEffect(!!behavior?.glassBlurEnabled, {
    tint: behavior?.glassTintStrength, depth: behavior?.glassDepth
  });
  return applyThemeToApp(themeConfigFromBehavior(behavior));
}
```

```ts
// store.ts:134-145  侧栏切换走自己的通道：Get → 改一个字段 → Save 整结构
export async function setThemeMode(mode: ThemeMode): Promise<void> {
  applyThemeToApp({ ...themeState.config, mode });
  const current: any = await App.GetAppBehavior();
  if (!current) return;
  current.themeMode = themeState.config.mode;
  await App.SaveAppBehavior(current);
```

```go
// core/appcore/controller.go:2405-2410
func (c *Controller) SaveAppBehavior(b AppBehavior) error {
	old := c.Behavior.Get()
	if err := c.Behavior.SetAndSave(b); err != nil { return err }
```

**为什么是问题（触发路径，可复现）**

1. 侧栏与 `<Settings>` 是**同时挂载**的：`App.vue:61-67` 的 `<Sidebar>` 与 `App.vue:169-171` 的 `<Settings>` 都在同一个 `main-layout` 里，所以"设置页开着"和"点侧栏明暗"可以同时发生。
2. 进设置页 → `loadBehavior()` 拿到的快照里 `themeMode = 'dark'`。
3. 点侧栏切浅色 → `setThemeMode('light')`：前端立即生效，后端 `Get→改→Save` 写入 `light`。**设置页的 `behavior.value` 仍是 `dark`**（`syncThemeFromBehavior` 按注释刻意不回写）。
4. 在设置页做**任何**一次保存：主题页拖玻璃滑条（`commitGlass` → `emit('save')`）、DNS/网络/行为页的自动保存，最终都走 `Settings.vue:204 SaveAppBehavior(behavior.value)` —— 把 `themeMode: 'dark'` 一起写回去。
5. 后端 `SetAndSave` 整结构落库 → `themeMode` 回到 `dark`。下次 `loadBehavior` / 下次启动时 `syncThemeFromBehavior` 把界面刷回深色；用户看到"我明明切成浅色，进一次设置又变回来了"。

这正是 `store.ts:127-133` 注释里描述过的同型事故（"每次重新进入设置页都会把用户的切换顶回去"），但只修了**读**的方向，没修**写**的方向。`AppState.Theme` 也刻意不被消费（`store.ts:337-341`），所以后端也没有二次校正的机会。

**修复建议**

首选：只提交本页确实改过的字段（差分提交），从根上消掉多写者：

```ts
// Settings.vue
const baseline = ref<any>(null);
const loadBehavior = async () => { ...; baseline.value = structuredClone(value); };
const saveBehavior = async () => {
  const patch: any = {};
  for (const k of Object.keys(behavior.value)) {
    if (JSON.stringify(behavior.value[k]) !== JSON.stringify(baseline.value?.[k])) patch[k] = behavior.value[k];
  }
  if (Object.keys(patch).length === 0) return;
  await App.SaveAppBehaviorPartial(patch);   // 需后端新增；或 Go 侧 SaveAppBehavior 先 Get 再 merge
  baseline.value = structuredClone(behavior.value);
};
```

次选（不动后端）：每次保存前把主题字段从真源覆盖进快照 —— 在 `saveBehavior` 开头加一行：

```ts
writeThemeToBehavior(behavior.value, themeState.config);   // 已存在于 utils/theme.ts:448
```

至少保证"主题"这个被侧栏共享的字段永远以 `themeState` 为准。

---

### U-02（P1，bug / 样式层冲突）全局 `outline: none !important` 打死组件自己的焦点环

**位置**
- `frontend/src/style.css:67-80`（全局打死）
- `frontend/src/components/SettingsTheme.vue:747, 780, 826, 901, 929`（5 处尝试画焦点环）

**现状**

```css
/* style.css:67-80 */
*,
*::before,
*::after { outline: none !important; -webkit-tap-highlight-color: transparent; }

*:focus,
*:focus-visible {
    outline: none !important;
    box-shadow: none;
}
```

```css
/* SettingsTheme.vue（scoped，无 !important） */
.seg-btn:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 1px; }
.preset-tile:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.accent-dot:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.follow-btn:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.blur-range:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 3px; }
```

**为什么是问题**

CSS 层叠第 4 级规则：**`!important` 声明优先于任何普通声明，与特异性无关**。所以这 5 条（特异性 0,2,0）必然输给 `*:focus-visible { outline: none !important }`（0,1,0 + important）。证据等级 A（纯层叠规则，无需实测）。

影响：
- 主题页的 5 个可键盘聚焦控件（分段控件、12 张预设卡、强调色色点、跟随按钮、滑条）**Tab 时完全无视觉反馈** —— 恰恰是全应用唯一做了 a11y 努力的页面，努力全部作废。
- 全应用其它控件（按钮、输入框、开关）本来就没写 focus 样式，又被全局打死，等于整个桌面应用没有任何键盘焦点指示。
- 顺带把 `box-shadow: none` 也铺到所有 `:focus` 上：任何"用 box-shadow 画 focus 环"的写法（`--ui-accent-ring` 的常规用法）都会失效。

**修复建议**

```css
/* style.css:67-80 改为 */
*:focus:not(:focus-visible) { outline: none !important; }   /* 鼠标点击不画框 */
*:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
```

若产品坚持"桌面端不要焦点框"，则至少删掉组件里那 5 条死规则，别留下"写了但不生效"的样式。

---

### U-03（P1，i18n）工具层硬编码中文，8 语言下固定显示中文

**位置**
- `frontend/src/utils/format.ts:37-50`（`formatRelativeTime` 全部中文）
- `frontend/src/utils/errors.ts:108-109`（`ERROR_LOGGED_SUFFIX`）
- 调用点：`components/SettingsUpdate.vue:32/52`（渲染"上次检查"）；`errors.ts:144-149`（所有未命中规则的错误文案尾部）

**现状**

```ts
// format.ts:37-50
export const formatRelativeTime = (timestamp: number) => {
  if (!timestamp) return '从未';
  ...
  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return Math.floor(diff / 60000) + ' 分钟前';
  if (diff < 86400000) return Math.floor(diff / 3600000) + ' 小时前';
  if (diff < 2592000000) return Math.floor(diff / 86400000) + ' 天前';
  return new Date(tsMs).toLocaleDateString();
};
```

```ts
// errors.ts:108-109 + :144-149
export const ERROR_LOGGED_SUFFIX = '（已记录日志）';
export function displayErrorMessage(e: unknown): string {
  const message = toUserError(e).message;
  if (classifyError(e).kind !== 'unknown') return message;
  if (matchesKnownErrorRule(e)) return message;
  return message + ERROR_LOGGED_SUFFIX;
}
```

**为什么是问题**

- `formatRelativeTime` 只有一处调用（`SettingsUpdate.vue` 的"上次检查更新"），但这处用户可见；en-US / ja-JP / ru-RU / de-DE 下会显示"3 小时前"。
- `displayErrorMessage` 是全前端错误文案的统一出口（38 处调用点），非中文语言下每个未命中规则的错误都会带一串中文后缀。
- 这是历史 **F-A4-16（硬编码文案绕过 i18n，已标 fixed B3）的漏网点**：当时只改了 `SettingsAbout.vue:456/459`、`SettingsTun.vue:111`、`Proxies.vue:388` 四处，**没有覆盖 utils/ 层**。

**修复建议**

```ts
// format.ts
import { t } from '../locales';
export const formatRelativeTime = (timestamp: number) => {
  if (!timestamp) return t('common.never');
  ...
  if (diff < 60000) return t('common.justNow');
  if (diff < 3600000) return t('common.minutesAgo', { n: Math.floor(diff / 60000) });
  if (diff < 86400000) return t('common.hoursAgo', { n: Math.floor(diff / 3600000) });
  if (diff < 2592000000) return t('common.daysAgo', { n: Math.floor(diff / 86400000) });
};
```

并在 8 个语言包 `common` 段补 `never / justNow / minutesAgo / hoursAgo / daysAgo`（`t()` 已支持 `{n}` 插值，见 `locales/index.ts:70-74`）。

`errors.ts:109` 改为 `export const errorLoggedSuffix = () => t('common.errorLogged')`（依赖方向 errors → locales 无环）。若不想让工具层依赖 i18n，也可把后缀挪到 `store.showAlert` 的显示层统一拼接。

---

### U-04（P1，逻辑 / 设计系统）「圆角」设置只对 8 处 `var()` 生效，155 处写死

**位置**
- `utils/theme.ts:189-193`（`RADIUS_VALUES`）、`theme.ts:412-415`（写变量）
- `styles/tokens.css:43-46`（定义）
- 消费方：`styles/surfaces.css:13/32`（`--ui-radius-lg`）、`SettingsTheme.vue:693/706/721/735/772/873`（sm/md，且都是主题页自己的预览元素）

**现状**

```ts
// theme.ts:412-415
const r = RADIUS_VALUES[applied.config.radius];
s.setProperty('--ui-radius-sm', r.sm);
s.setProperty('--ui-radius-md', r.md);
s.setProperty('--ui-radius-lg', r.lg);
```

机器统计（`frontend/src` 全部 css + scoped 块）：
```
border-radius: 硬编码 = 155    var() = 8
（style.css 19、Proxies.vue 14、SettingsTheme.vue 14、App.vue 9、Rules.vue 8、SettingsAbout.vue 8 …）
var(--ui-radius-*) 消费点：surfaces.css:13 / surfaces.css:32 / SettingsTheme.vue 6 处
```

**为什么是问题**

- 用户在"设置 → 应用主题 → 圆角"选 sharp/round，只有 `.ui-surface-card / .ui-surface-group / .ui-surface-overlay`（模板里共 40 个元素）与主题页自己的预览块会变；**所有按钮（`style.css:745` 写死 8px）、输入框、分段控件、下拉、模态卡片（`style.css:593` 写死 16px）、标签栏（`style.css:860` 写死 12px）全部不动**。
- `--ui-radius-sm/md` 在 surfaces.css / style.css 里一次都没被消费，只有 `SettingsTheme.vue` 自己的样式在用 —— 这套 token 从未铺开。
- 触发路径：任一用户从 sharp 切到 round，整个界面只有卡片角变了。这是"设置项存在但基本无效"的典型缺陷（若产品明确"只控卡片"，则应删掉 sm/md 的暗示并把文案改成"卡片圆角"）。

**修复建议**

1. 若要做成全局偏好：把 `style.css` / 组件里的 `border-radius: 8px|10px|12px|16px` 按语义换成 `var(--ui-radius-sm|md|lg)`（按钮/输入框 → sm；控件槽/标签栏 → md；卡片/浮层 → lg）。155 处可用 codemod，但尺寸档需人工分组。
2. 若只控卡片：删掉 `RADIUS_VALUES.sm/md` 与 `tokens.css:43-44`，文案改成"卡片圆角"。

---

### U-05（P2，逻辑错误）`/helper|Helper/i` 规则过宽，抢占其它语义

**位置**：`frontend/src/utils/errors.ts:10-20`

**现状**
```ts
const RULES: ErrorRule[] = [
  // ... yaml / Access is denied / bind / connection refused / 403 / 404 ...
  { re: /helper|Helper/i, message: '需要 Helper 服务' },          // :18
  { re: /deadline exceeded|timeout|timed out|error 10060.../i, message: '请求超时' },
];
```

**为什么是问题**：`helper` 是路径名/服务名/进程名的一部分，且该规则排在 timeout 之前：
- "启动 helper 超时: context deadline exceeded" → 显示「需要 Helper 服务」，掩盖真正原因；
- "C:\ProgramData\goclashz\helper\helper.exe 已被占用" → 显示「需要 Helper 服务」，丢掉"被占用"。
影响限于人话层（`detail` 仍保留原文），但会让用户按错误提示走错修复路径（去装 Helper，实际是超时）。

**修复建议**：收紧到后端真实语义串，并与 `CLASSIFY_RULES:87` 的 code 对齐：

```ts
{ re: /helper_install_required|HELPER_NOT_INSTALLED|helper 服务未安装/i, message: '需要 Helper 服务' },
```

同时把 `deadline exceeded|timeout` 规则上移到它之前。

---

### U-06（P2，逻辑错误）`formatEtaTime` 无小时档

**位置**：`frontend/src/utils/format.ts:16-21`

**现状**
```ts
export const formatEtaTime = (seconds: number) => {
  if (!Number.isFinite(seconds) || seconds < 0) return '--';
  if (seconds === 0) return '--';
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
};
```
（第 19、20 行是两个模板字符串，上文按真实源码展示。）

**为什么是问题**：`3600` → `60m 0s`、`86400` → `1440m 0s`。调用点 `UpdateTaskRow.vue:29/43`、`SettingsAbout.vue:29/206`。内核/驱动包在慢速链路下 ETA 超过 1 小时很常见，此时从 `60m 0s` 一路到 `1440m 0s`，观感明显异常。而仓库里恰好有一个**支持小时档却零引用**的 `formatDurationCn`（`format.ts:23-35`，见 U-15）。

**修复建议**：合并两者：

```ts
export const formatEtaTime = (seconds: number) => {
  if (!Number.isFinite(seconds) || seconds <= 0) return '--';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  return h > 0 ? '`${h}h ${m}m`' : m > 0 ? '`${m}m ${s}s`' : '`${s}s`';
};
```

---

### U-07（P2，逻辑错误）染色上限借用「模糊上限」常量

**位置**：`frontend/src/utils/glass.ts:218`、`components/SettingsTheme.vue:586`（及模板 `:219`）

**现状**
```ts
// glass.ts:7-8
export const GLASS_BLUR_MIN = 0;
export const GLASS_BLUR_MAX = 80;
// glass.ts:218  ← 染色强度 / 模糊上限
const ratio = normalizeGlassStrength(chroma, GLASS_TINT_DEFAULT) / GLASS_BLUR_MAX;
```
```ts
// SettingsTheme.vue:586-588（染色滑条的填充比例也按 BLUR_MAX 算）
const trackStyle = (value: number, max: number = GLASS_BLUR_MAX) => {
  const percent = Math.max(0, Math.min(100, (value / max) * 100));
```

**为什么是问题**：染色强度与模糊半径是两个独立语义（`tokens.css:69-74` 的注释也明确"滑条只改染成什么颜色，不改透明度"），但换算写死为"值 / 80"。当前两者上限恰好都是 80 所以看不出问题；一旦有人把 `GLASS_BLUR_MAX` 调到 40（例如为限制模糊开销），染色会**静默**在半强度饱和 —— 无类型错误、无运行时告警，只有观感退化；`SettingsTheme.vue:222` 的 `trackStyle(glassTint)` 还会出现"滑条走到 50% 就到头"的错位。

**修复建议**：

```ts
export const GLASS_TINT_MIN = 0;
export const GLASS_TINT_MAX = 80;
export const glassTintChroma = (tint: number) =>
  Math.min(GLASS_TINT_MAX, Math.max(GLASS_TINT_MIN, normalizeGlassStrength(tint, GLASS_TINT_DEFAULT)));
// glass.ts:218 → ratio = glassTintChroma(chroma) / GLASS_TINT_MAX;
// SettingsTheme.vue:586 → trackStyle = (value, max = GLASS_TINT_MAX)，模板 :max 同步替换
```

---

### U-08（P2，操作冲突）明暗状态：两条缓存键 + 三个真源，首帧可能自相矛盾

**位置**：`store.ts:69-70/113-120/229`、`utils/theme.ts:203/421-437`、`App.vue:2`

**现状**
```ts
// store.ts:69-70  启动期读「旧键」
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';
// store.ts:229
theme: cachedTheme,
// store.ts:113-117  applyThemeToApp 内同时写新旧两套
const nextTheme = applied.isDark ? 'dark' : 'light';
if (globalState.theme !== nextTheme) globalState.theme = nextTheme;
localStorage.setItem('goclashz_theme', nextTheme);
```
```ts
// utils/theme.ts:203  「新键」存的是整份偏好
export const THEME_CACHE_KEY = 'goclashz_theme_v2';
```
```vue
<!-- App.vue:2  .app-shell 的 dark 类来自 globalState.theme -->
<div class="app-shell" :class="{ dark: globalState.theme === 'dark' }" :data-lang="currentLocale">
```

**为什么是问题**

- 主题真源是 `themeState`（`store.ts:94-98`，来自 `goclashz_theme_v2`）；`globalState.theme` 只是镜像；`goclashz_theme` 是第三个存储。三者只在 `applyThemeToApp` 里同步。
- 首帧：`main.ts:11 bootstrapTheme()` 已经给 `<html>` 打好 `.dark` 与 `data-theme-mode`，但 `globalState.theme` 的初值来自**另一个键**。若两者不一致（例如"跟随系统 + 系统深色"时 v2 记 `mode:'system'`、旧键此刻是 `light`），就出现 `<html>.dark` 成立而 `.app-shell` 没有 `.dark` 的一帧 —— `tokens.css:109`、`Connections.vue:648`（`.dark .tag-proxy`）、`YamlEditor.vue:579`（`:root.dark`）走不同分支，表现为"底色变了但某些描边/标签还是浅色"。
- 第二帧起 `App.vue:756-765` 的 `GetAppBehavior → syncThemeFromBehavior → applyThemeToApp` 会修正，所以只是首帧不一致 + 多余真源，不是永久错乱。证据 B。

**修复建议**

```ts
// store.ts:69-70 / 94-98
const initialThemeConfig = readCachedTheme();          // 已有
export const themeState = reactive({
  config: initialThemeConfig,
  isDark: resolveIsDark(initialThemeConfig.mode)
});
theme: resolveIsDark(initialThemeConfig.mode) ? 'dark' : 'light',   // ← 替换 cachedTheme
// 并删除 store.ts:70 与 :117 对 'goclashz_theme' 的读写（全仓库仅这两处使用）
```

`App.vue:2` 的 `:class="{ dark: ... }"` 可一并删除：`<html>` 上的 `.dark` 已是所有 `.dark` 选择器的祖先，两处同时挂只增加不一致面。

---

### U-09（P2，bug）`localStorage` 三处无 try/catch，抛错会中断整段同步

**位置**：`locales/index.ts:29/85`、`store.ts:69-75/346/402-405`

**现状**
```ts
// locales/index.ts:28-34  ← 模块顶层执行，无 try
function getInitialLocale(): SupportedLocale {
  const cached = localStorage.getItem(STORAGE_KEY) as SupportedLocale | null;
  if (cached && messages[cached]) return cached;
  return 'zh-CN';
}
```
```ts
// store.ts:69-75  ← 模块顶层，无 try
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';
const cachedActiveConfigId = localStorage.getItem('goclashz_activeConfigId') || '';
const cachedOutboundIP = localStorage.getItem('goclashz_outboundIP');
```
```ts
// store.ts:343-347  中途，无 try；抛错后本次同步剩余字段全部不生效
if (newHideLogs !== undefined) {
  globalState.hideLogs = newHideLogs;
  localStorage.setItem('goclashz_hideLogs', String(newHideLogs));
}
```

**为什么是问题**

- 同仓库其它地方都加了保护（`theme.ts:421-427 cacheTheme`、`store.ts:116-120`、`store.ts:208-212`），说明团队知道会抛；这三处是漏网。
- 触发条件（B 级）：WebView2 在组策略/隐私设置禁用本地存储、或配额满时抛 `SecurityError / QuotaExceededError`。
- 后果两档：模块顶层抛错 → **store/locales 模块加载失败 → 白屏**（`main.ts:7` 直接 import）；`updateStateFromBackend` 内抛错 → 这一次 `app-state-sync` 只应用一半字段（`isRunning/mode` 已改，`desired*/版本` 没改），界面停在混合态。

**修复建议**：抽 `utils/storage.ts` 并全仓库替换（含组件里 6 处直接调用：`Connections.vue:142/269`、`Rules.vue:143/363`、`Proxies.vue:152/279/314/318`）：

```ts
export const safeGet = (k: string): string | null => { try { return localStorage.getItem(k); } catch { return null; } };
export const safeSet = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* ignore */ } };
```

---

### U-10（P2，bug）`hoverGuard` 三个缺陷：新规则不纳入 / restore 失败被吞 / 无条件抑制

**位置**：`frontend/src/utils/hoverGuard.ts:27-52`、`:54-57`

**现状**
```ts
const suppress = () => {
  if (suppressed) return;                 // ①抑制期间新出现的 :hover 规则永远不会被收集
  saved = [];
  for (const sheet of Array.from(document.styleSheets)) { try { if (sheet.cssRules) collect(sheet.cssRules); } catch {} }
  if (saved.length === 0) return;
  for (const item of saved) {
    try { item.rule.selectorText = item.selectorText.replace(/:hover\b/g, ':not(*)'); }
    catch { /* some browsers may expose read-only selectors */ }   // ②吞掉：该规则永久保留 :not(*)
  }
  suppressed = true;
};
const restore = () => {
  if (!suppressed) return;
  for (const item of saved) { try { item.rule.selectorText = item.selectorText; } catch { /* ignore */ } }
  saved = []; suppressed = false;
};
const onBlur = () => suppress();
const onFocus = () => suppress();                 // ③窗口获得焦点也抑制
const onVisibility = () => suppress();            // ③不看 document.hidden
```

**为什么是问题**

1. **永久性副作用**：`restore()` 的 catch 一旦对某条规则失败，这条规则的 `:not(*)` 再也不会被改回来，该元素 hover 效果永久消失直到重启。
2. **抑制期间新增的样式不进入清理集**：`suppressed===true` 时直接 return，所以抑制期间动态插入的 `:hover` 规则不会被禁用 —— 行为不一致但不致命。
3. **`onFocus` / `onVisibility` 无条件抑制**：窗口从后台回到前台也把全站 `:hover` 改写成 `:not(*)`，只有 mousemove/mousedown 才 restore。只用键盘（Tab+Enter）的用户切回窗口后，所有 hover 样式保持失效直到碰一下鼠标。
4. 机制本身很重：它把**全站每一条含 `:hover` 的规则**（含 CodeMirror 注入的样式表）改写，每次 blur/focus 都是一次全文档样式失效重算。

**修复建议**

- 首选：放弃改写 CSSOM。需要清理陈旧 hover 的元素改为 JS 驱动 hover 态（`App.vue:30-34` 的 `ctrlHover` 已示范），或在窗口失焦时给 `<html>` 加一次性类，配少量白名单规则。
- 若保留现方案，至少：① `suppress` 期间记录待处理规则并在下一次 collect 时补处理；② `restore` 的 catch 走 `reportError` 并把失败规则留在 `saved` 重试；③ `onVisibility` 加 `if (!document.hidden) return;`，`onFocus` 直接删掉（blur 已覆盖）。

---

### U-11（P2，性能/冗余）`scrollFade` 的全局观察器：任意 class/style 变化都全文档扫描

**位置**：`frontend/src/utils/scrollFade.ts:38-65`

**现状**
```ts
export function initScrollFades() {
  if (started) return;
  started = true;
  document.addEventListener('scroll', (e) => {...}, true);      // 无 dispose
  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => updateAll());
    ro.observe(document.body);
  }
  if (typeof MutationObserver !== 'undefined') {
    const mo = new MutationObserver(() => updateAll());
    mo.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style'] });
  }
  window.addEventListener('resize', updateAll);
}
```

**为什么是问题**

- `updateAll()` 每次执行两遍 `document.querySelectorAll` + 逐元素读 `scrollHeight/clientHeight`（**强制同步布局**）。观察器挂在 body 整个子树、`attributes: true`：应用里**任何**元素的一次 `:class` 变化（列表刷新、延迟颜色、选中态、toast 入场）都会触发一次全文档查询 + 2 次强制 reflow。代理页/连接页高频更新时是可测开销（B 级：量级推断，未实测）。
- 三个观察器都**没有 dispose 路径**（不返回清理函数、`started` 是模块级单例）。App 是根组件不会卸载，所以不构成真实泄漏；但未来若 App 变成可重挂载、或做 HMR/测试，会重复注册。

**修复建议**

1. 把 mutation 触发节流到一帧：
```ts
let pending = false;
const schedule = () => { if (pending) return; pending = true; requestAnimationFrame(() => { pending = false; updateAll(); }); };
// ResizeObserver / MutationObserver 都改调 schedule
```
2. 缩小观察面：只 observe 已知滚动容器（`.ui-page-body` / 三个 `*-tabs-track`），切页时重新绑定；或去掉 MutationObserver，改为滚动时按需计算（`apply()` 本来就只读目标元素）。
3. 返回 dispose（`disconnect()` + `removeEventListener`），在 `App.vue:758` 的 onUnmounted 里调用（那里已在释放 hoverGuard）。

---

### U-12（P2，冗余/死代码）14 个自定义属性零消费；z-index 同时存在 token 与硬编码两套

**位置**：`styles/tokens.css:11/46/55-57/66/68/91/98/101-105`、`style.css:135/584/949`、`utils/theme.ts:398/405`、`utils/glass.ts:282`

**现状（机器统计：`var(--x)` 引用点 = 0）**
```
--ui-z-sticky(101) --ui-z-dropdown(102) --ui-z-modal(103) --ui-z-toast(104) --ui-z-tooltip(105)
--ui-glass-tint(55) --ui-glass-alpha(56) --ui-glass-border(57) --ui-radius-full(46)
--glass-blur-content(66) --glass-blur-control(68) --glass-highlight(98/113) --glass-fill-content(91)
--ui-accent-hover(11/123)   ← theme.ts:398 每次主题切换都在写
--status-online             ← theme.ts:405 每次主题切换都在写
```
实际层级用的是裸值：
```css
/* style.css:584 */ .modal-overlay { z-index: 9999; }
/* style.css:949 */ .toast-stack    { z-index: 10000; }   /* 注释：必须高于 .modal-overlay */
/* ModernSelect.vue:100/191/269 */ z-index: 100; z-index: 50;
```

**为什么是问题**

- 5 档 z-index token 是"设计系统已建立"的假象：改 `--ui-z-modal` 完全无效，真实层级靠两个魔数维持（toast 必须高于 modal，见 `style.css:948` 注释）。
- `--status-online` / `--ui-accent-hover` / `--glass-fill-content` 属于"写者活着、读者没有"：每次切主题/拖玻璃滑条都在 `setProperty` 三个没人看的变量（开销可忽略，但意味着主题系统的意图与实现脱节，后续改色会改到断头路）。
- `--glass-blur-content/control` 仍定义着，而 `glass.ts:1-5` 的注释明确说"只有浮层那一级真的挂了 backdrop-filter，另外两级已移除"。

**修复建议**：删掉 14 个死 token 与对应 `setProperty`；z-index 真正收口：`style.css:584` → `var(--ui-z-modal)`，`:949` → `var(--ui-z-toast)`，`ModernSelect` 三处 → `var(--ui-z-dropdown)`。若暂不动实现，至少在 `tokens.css:100-105` 上方注明"当前未被消费，改它无效"。

---

### U-13（P2，死代码）`utils/normalize.ts` 整个文件是孤儿

**位置**：`frontend/src/utils/normalize.ts:1-19`

**现状**
```ts
export const normalizeStartupTaskInfo = (info: any) => {
  if (!info) return null;
  return {
    exists: info.exists ?? info.Exists ?? false,
    enabled: info.enabled ?? info.Enabled ?? false,
    mode: info.mode ?? info.Mode ?? 'disabled',
    ...
    isHealthy: info.isHealthy ?? info.IsHealthy ?? false,
  };
};
```
机器统计：**0 引用**（文件内、文件外都没有第二次出现）；全 `frontend/src` 里也不存在 `StartupTask` 任何字样的调用点 —— 说明前端没有消费启动项自检结果。

**为什么是问题**：20 行无人调用的代码 + 一个"看起来存在的能力"。维护者会以为前端有启动项归一化层，实际没有；ESLint/tsc 也不会报（没有被 import）。

**修复建议**：删除该文件。若"启动项自检"确实打算上界面，那是功能缺口（见 §6 第 3 条），应由主控确认后单独排期，而不是留一个 0 引用文件当实现。

---

### U-14（P2，死代码）错误环形缓冲只写不读

**位置**：`frontend/src/utils/report.ts:18-19/33-40`

**现状**
```ts
const MAX_RECORDS = 50;
const records: ErrorRecord[] = [];
export function reportError(e: unknown, ctx: string): void {
  ...
  records.push({ at: Date.now(), ctx, message: u.message, detail: u.detail });
  if (records.length > MAX_RECORDS) records.shift();
}
/** 供日志面板 / 诊断复制的最近错误快照（只读） */
export function getRecentErrors(): readonly ErrorRecord[] { return records; }
```
`reportError` 有 59 处调用（正常），但 `getRecentErrors` **0 引用**。

**为什么是问题**：注释写着"供日志面板/诊断复制"，但日志面板（`App.vue` 的 logs 页）与诊断导出（`SettingsDiagnostics.vue`）都没接它 —— 用户点"导出诊断"时，最近 50 条前端错误**不在包里**。这让"失败留痕"（F-A4-11 的目标）只完成了一半：控制台有，用户拿不到。

**修复建议**（二选一）
1. 接上：`SettingsDiagnostics.vue` 的导出内容追加 `getRecentErrors()`（约 3 行），并让日志面板新增"前端错误"来源。
2. 删掉：若短期没有落盘通道，删掉 `records` 与 `getRecentErrors`，只保留 `console.error`，别维护一个永不出货的缓冲。

---

### U-15（P2，死代码）glass / format 的 4 个死导出

**位置**：`utils/glass.ts:11/13/15/44`、`utils/format.ts:23-35`

**现状**（机器统计：文件内外总引用 0）
```ts
glass.ts:11  export const GLASS_BLUR_DEFAULT = 12;            // 仅文件内 2 处当 fallback
glass.ts:13  export const GLASS_BLUR_OVERLAY_DEFAULT = 28;    // 0 引用，且与 tokens.css:67 的 40px 不一致
glass.ts:15  export const GLASS_BLUR_CONTROL_DEFAULT = 8;     // 0 引用
glass.ts:44  export function normalizeGlassBlurStrength(...)   // 0 引用（注释说"兼容旧调用方"，旧调用方已不存在）
format.ts:23 export const formatDurationCn = (seconds) => ... // 0 引用
```

**为什么是问题**：`GLASS_BLUR_OVERLAY_DEFAULT` / `CONTROL_DEFAULT` 与 `tokens.css:66-68` 的设计常量重复（control 8 一致、overlay 28 vs 40 不一致），死值会误导后来者以为可以调；`formatDurationCn` 是唯一支持"小时"档的时长格式化函数（见 U-06），死掉的同时 `formatEtaTime` 缺小时档。

**修复建议**：删 `normalizeGlassBlurStrength`、`GLASS_BLUR_CONTROL_DEFAULT`；`GLASS_BLUR_OVERLAY_DEFAULT` 若要保留就与 `tokens.css:67` 对齐成同一个值（或干脆只留 CSS 一处）；`formatDurationCn` 保留并改造成 `formatEtaTime` 的小时档（见 U-06 修复片段）。

---

### U-16（P2，死代码）约 50 行永不匹配的 CSS 兜底块 + 2 个无消费者的 `@keyframes`

**位置**：`style.css:234-236/238-258/711-733`、`tokens.css:120-132`

**现状**
```css
/* style.css:234-236 */
.dark { --danger: #E81123; }        /* 与 :root 同值，纯冗余 */

/* style.css:238-258 与 tokens.css:120-132 —— 全部被 html:not([data-theme-mode]) 守卫 */
html:not([data-theme-mode]) .dark {
  --app-bg: #111111; --surface-panel: #1A1A1A; ...
}
```
```ts
// main.ts:11  createApp 之前就执行；theme.ts:379 无条件写 dataset.themeMode
bootstrapTheme();
createApp(App).mount('#app');
```
```css
/* style.css:711-733 */
@keyframes fadeIn { ... }   /* 消费方 SettingsNetwork.vue:471 / SettingsLan.vue:400 各自在 scoped 块内定义了同名 keyframes
@keyframes pulse  { ... }   /* 消费方 TrafficCard.vue:226 同样自带 scoped @keyframes pulse（TrafficCard.vue:336）
```

**为什么是问题**

- `html:not([data-theme-mode])` 在 `bootstrapTheme()` 之后**永远为假**（`applyTheme` 每次都写 `data-theme-mode`），所以这 21+13 行是纯死 CSS，且它们的存在让"纯 CSS 兜底"听起来像真的。
- `.dark { --danger: #E81123 }` 是同一变量同值的第二次声明（`:root` 已有）。
- `fadeIn`/`pulse` 两个全局副本没有消费者：组件的 scoped `@keyframes` 会被 `@vue/compiler-sfc` 加上 scope 哈希并重写引用，走不到全局定义（证据 B，依赖框架行为，未实测）。

**修复建议**：删除 `style.css:234-236`、`238-258`、`711-733` 与 `tokens.css:120-132`。若确实要保留"主题引擎未接管"的降级路径，请在注释里写明**唯一可能场景**（`applyTheme` 抛错），而不是留一份永不执行的 40 行副本。

---

### U-17（P2，冗余）11 个设置子页复制同一套 scoped 样式与同一段时序 hack

**位置**：`Settings{About,Behavior,Diagnostics,Dns,Home,Lan,Network,Tun,Update,Uwp,Theme}.vue` 的 `<style scoped>`

**现状（机器去重：同名选择器出现次数）**
```
.back-btn                12 次   .settings-page        11 次   .setting-group       11 次
.setting-item            11 次   .sub-header           11 次   .back-btn:hover      11 次
h3 / h4 / p / .info      各 10 次  .setting-group.scrollable 8 次  .divider 8 次
.modern-switch + input + .slider + :before + disabled/checked 变体  各 7 次
.sub-header.section-header h3  7 次   .mini-btn-reset 6 次   .section-header 5 次
```
另：`Settings.vue:197-199`、`SettingsDns.vue`、`SettingsLan.vue`、`SettingsNetwork.vue`、`SettingsTun.vue` 各写了一遍
```ts
requestAnimationFrame(() => { requestAnimationFrame(() => { behaviorLoading.value = false; }); });
```
（"必须等浏览器真正绘制一帧后再解除 no-anim"的时序 hack，最重要的注释只存在于 `Settings.vue:194-196`。）

**为什么是问题**：单文件 40-60 行 × 11 ≈ 500 行重复 CSS；任何一次视觉调整（比如 `.back-btn` 内边距）都要改 12 个地方，改漏一处就出现"某个设置页返回按钮比别人宽 2px"。`nextTick→双 rAF` 复制 5 份，任何一份的改动都会造成加载动画行为不一致。

**修复建议**：新建 `frontend/src/styles/settings.css`（在 `main.ts:6` 的 layout.css 之后 import），把 `.settings-page / .setting-group / .setting-item / .sub-header / .divider / .modern-switch / .slider / .mini-btn-reset / .back-btn / .section-header` 与 h3/h4/p 上移为全局类；组件 scoped 块只留真正独有的部分。时序 hack 抽成 `utils/nextPaint.ts`：

```ts
export const afterNextPaint = (fn: () => void) =>
  requestAnimationFrame(() => requestAnimationFrame(fn));
```

---

### U-18（P2，冗余）「复制到剪贴板」3 套实现，失败文案不一致

**位置**：`App.vue:576-581`、`App.vue:586-596`、`components/SettingsDiagnostics.vue:245-251`、`components/Subscriptions.vue:435-440`

**现状**
```ts
// App.vue:576-581
try { await navigator.clipboard.writeText(text); showToast(t('common.copied'), 'success'); }
catch { showToast(t('common.copy') + ' ✗', 'error'); }

// SettingsDiagnostics.vue:245-251  ← 与上面几乎逐字相同
try { await navigator.clipboard.writeText(text); showToast(t('common.copied'), 'success'); }
catch { showToast(t('common.copy') + ' ✗', 'error'); }

// Subscriptions.vue:435-440  ← 同样动作，文案走 i18n
try { await navigator.clipboard.writeText(config.url); status = t('subs.linkCopied'); }
catch { status = t('subs.copyFailed'); }
```

**为什么是问题**：同一动作三种反馈；`t('common.copy') + ' ✗'` 把按钮名（"复制"）当失败文案，且符号是硬编码（8 语言下都是"复制 ✗"）。剪贴板在 WebView2 里失败是真实场景（窗口失焦时抛 `NotAllowedError`），失败提示质量直接影响可诊断性。

**修复建议**：抽 `utils/clipboard.ts`：

```ts
import { t } from '../locales';
import { showToast } from '../store';
import { reportError } from './report';
export async function copyText(text: string, okMsg?: string): Promise<boolean> {
  try { await navigator.clipboard.writeText(text); showToast(okMsg ?? t('common.copied'), 'success'); return true; }
  catch (e) { showToast(t('common.copyFailed'), 'error'); reportError(e, 'clipboard.copy'); return false; }
}
```
（`common.copyFailed` 需补进 8 语言包；可复用已存在的 `subs.copyFailed` 译文。）

---

### U-19（P2，冗余）「清理窗口切换后的残留 hover」两套机制并存

**位置**：`App.vue:336-339`、`App.vue:773-777`、`utils/hoverGuard.ts:54-63`

**现状**
```ts
// App.vue:336-339（局部机制）
const ctrlHover = ref<'min'|'max'|'close'|null>(null);
const clearCtrlHover = () => { ctrlHover.value = null; };
// App.vue:773-777
window.addEventListener('blur', clearCtrlHover);
window.addEventListener('focus', clearCtrlHover);
document.addEventListener('visibilitychange', clearCtrlHover);
```
```ts
// hoverGuard.ts:54-63（全局机制，同样监听这三个事件）
window.addEventListener('blur', onBlur);
window.addEventListener('focus', onFocus);
document.addEventListener('visibilitychange', onVisibility);
```

**为什么是问题**：窗口控件的 hover 已改成 JS 驱动的 `ctrlHover`（`App.vue:30-34` 用 `@mouseenter/@mouseleave` 而非 CSS `:hover`），局部清理就够了；`installHoverGuard` 又用改写全站 CSSOM 的代价解决同一问题。两套机制对同一事件做两件事，任何一处改动都要考虑另一处，而全局机制的副作用（U-10）覆盖整站。

**修复建议**：删除 `installHoverGuard()`（`App.vue:250/621/759/977-978` 一并删），保留 `clearCtrlHover` 这一套；若确认非控件元素也存在"恢复后 :hover 卡住"，再针对那些容器做局部处理。

---

### U-20（P2，前后端对接）Settings 的兜底 behavior 对象缺 4 个后端字段

**位置**：`components/Settings.vue:166`、`bindings/goclashz/core/appcore/models.ts:8-174`、Go `core/appcore/behavior.go:18-88`

**现状**

`Settings.vue:166` 用一个手写的 `ref<any>({...})` 当兜底（节选）：
```ts
const behavior = ref<any>({ language: ..., silentStart: false, closeToTray: true, startupWithOS: false,
  ..., themeMode: 'light', themePreset: 'graphite', themeAccent: '', themeRadius: 'soft', themeAura: true,
  glassBlurEnabled: false, glassBlurStrength: 12, glassBlurContentStrength: 12, glassBlurOverlayStrength: 18,
  glassBlurControlStrength: 8, glassTintStrength: 58, glassDepth: 50 });
```
后端 `AppBehavior` 共 41 个字段，其中这 4 个**不在兜底对象里**：
```
lastUpdateCheck (int64, Unix 秒)   autoDelayTest (bool)   autoDelayTestInterval (int)   portProxyMode (bool)
```

**为什么是问题**：`GetAppBehavior()` 失败时（`loadBehavior` 的 catch 只 `reportError`），页面拿这个缺字段的对象工作；用户随后任意保存 → `SaveAppBehavior(behavior.value)` 发的是**不含这 4 个字段**的 JSON → Go 反序列化成零值 → `SetAndSave` 整结构落库 → `portProxyMode` 被静默关闭（"仅端口代理"模式被关掉）、`lastUpdateCheck` 归零（下次启动必然再检查更新）、`autoDelayTest` 自动测速被关。证据 A（字段缺失）+ B（需 Get 失败）。与 U-01 同源：都是"整结构覆盖 + 前端快照"。

**修复建议**
1. 兜底对象改为从常量派生并**标注类型**（现在是 `ref<any>`，所以缺口不会被编译器发现）：
```ts
import type { AppBehavior } from '../../bindings/goclashz/core/appcore/models';
const DEFAULT_BEHAVIOR: AppBehavior = { /* 41 个字段齐全 */ };
const behavior = ref<AppBehavior>({ ...DEFAULT_BEHAVIOR });
```
2. `loadBehavior` 失败时禁用保存入口（保持 `behaviorLoading=true` 或加 `loadFailed`），不要允许在"没读到"的状态下写。

---

### U-21（P2，i18n）`tray.*` 11 键前端零引用；校验脚本对动态 key 是盲区

**位置**：`frontend/src/locales/*.ts`（`tray` 段）、`scripts/check-i18n.mjs:93-104`

**现状**
- 8 个语言包键集**完全对齐**：每个 795 个叶子键，无缺键、无多余、无文件内重复（本轮复核，历史 F-A4-17 的"1 缺键"已不存在）。
- `node scripts/check-i18n.mjs` 输出：`[i18n] OK：语言对齐 + 字面量 key 全部存在`，同时 warn **188 个死键**。
- 我复核这 188 个：其中 143 个是"动态前缀"用法（脚本盲区）：
  `Connections.vue:235 const key = 'diag.note.' + code`、
  `DeviationStrip.vue:55-56 localized('deviation.field', d.field)`、
  `theme.ts:69-170 preset.nameKey = 'settings.theme.presets.*'`、
  `settings/manifest.ts` 的 `descKey`（125 个，我单独校验全部存在）。
- **真正死的是 `tray.*` 11 个**：`show / sysProxy / portProxy / tun / outbound / rule / global / direct / restart / quit / noConfig` —— 托盘菜单在 Go 侧构建（`tray_runtime_windows.go`），前端从不读这些 key。

**为什么是问题**：11 键 × 8 语言 = 88 条译文维护成本，而实际显示的是 Go 里的字符串；另外脚本的"死键"噪声太大（188 条里只有 11 条真死），没人会去看，等于这项检查失效。

**修复建议**：删除 `tray` 段（8 个文件各 11 行）；在 `check-i18n.mjs` 里给已知动态前缀建白名单（`['diag.note.', 'deviation.field.', 'deviation.reason.', 'settings.theme.presets.', 'settings.home.']` + 从 `manifest.ts` 收集的 `*Key` 值），让死键报告回到可用状态。

---

### U-22（P2，冗余/漂移）重复声明、常量与注释漂移、`*{font-family:!important}`

**位置**：`style.css:260-265/761-778/1500-1513`、`tokens.css:63-93`、`surfaces.css:10-18`、`layout.css:25/82`

**现状（四处）**
```css
/* ① style.css:761-778 同一条规则里 gap 声明两次 */
.action-btn { ... gap: 8px; ... gap: 10px; ... }

/* ② surfaces.css:10-18  .ui-surface-group 的 background 声明两次 */
.ui-surface-card, .ui-surface-group { background: var(--ui-surface-card); border-radius: var(--ui-radius-lg); }
.ui-surface-group { background: var(--ui-surface-group); }

/* ③ layout.css:25-43 与 82-91  .ui-page-body 被拆成两段重复选择器 */
.ui-page-body { --ui-fade-top: 0px; ... mask-image: ...; }
.ui-page-body { flex: 1 1 auto; ... padding-bottom: ... !important; }
```
```css
/* ④ style.css:1500-1513 注释写「浮层 82% vs 卡片 62%」，实际 tokens.css:92 是 46% */
/* style.css:260-265  全站字体锁死 */
* { box-sizing: border-box; margin: 0; padding: 0; font-family: var(--font-sans) !important; }
```

**为什么是问题**
- ①②③ 都是"看起来两套值、实际后者覆盖前者"的写法：读代码的人会以为 `.action-btn` 的 gap 是 8px（前一条还带注释），实际是 10px；改第一个值不生效。这类冗余的**直接成本是改错地方**。
- ④ 注释与实现漂移会误导玻璃调参（46% 与 82% 是两种完全不同的材质观感）。
- `font-family: !important` 挂在 `*` 上：任何 `font-family` 覆盖（`style.css:634 .global-modal-detail`、`style.css:657 .modal-input`、CodeMirror 的 `.cm-content`）**全部失效**。当前 `--font-sans` 第一顺位就是 JetBrains Mono（等宽），肉眼看不出差别，但这是一条"以后想给代码块换字体就换不了"的死约束。
- 同类漂移：`tokens.css:66-68` 仍定义 `--glass-blur-content: 12px / --glass-blur-control: 8px`，而 `glass.ts:1-5` 注释说这两级"已移除"；`GLASS_BLUR_OVERLAY_DEFAULT = 28` vs `--glass-blur-overlay: 40px` vs `tokens.css:63-65` 注释"28px 已经能把卡片抹成一片"。

**修复建议**
- ②③ 合并成一条规则/一段；① 删掉 `gap: 8px`。
- ④ 把注释里的 82% 改成 46%，或在 `--glass-fill-overlay` 处注明"设计常量 46%；实测 >45% 时 backdrop-filter 不再产出"。
- `font-family` 去掉 `!important`，改为 `:root { font-family: var(--font-sans) }` + 需要等宽处显式 `var(--font-mono)`。

---

### U-23（P2，bug 低危）日志列表 `v-for` 用索引作 key

**位置**：`App.vue:153-159`

**现状**
```vue
<div class="terminal-box ui-page-body" ref="logBox">
  <div v-for="(log, i) in filteredLogLines" :key="i" :class="['log-line', log.type]">
    <span class="l-time" :title="log.ts || log.time">{{ logTimeLabel(log) }}</span>
    ...
  </div>
</div>
```

**为什么是问题**：`filteredLogLines` 是"按级别/关键字过滤后的派生列表"，源数组是环形缓冲（满后从头部淘汰）。用索引作 key 时源数组前移会让第 i 个 DOM 节点绑定到不同日志行；当前只影响渲染效率（文本全量重算），但一旦给日志行加任何局部状态（展开/高亮/复制），就会出现"点开第 3 行详情，滚动后状态跑到别的行"。证据 B。

**修复建议**：给每条日志一个稳定 id（入口处自增），模板改用 `:key="log.id"`：

```ts
// App.vue 收日志处（unsubLogMessage 回调）
let logSeq = 0;
logs.value.push({ ...log, id: ++logSeq });
```

---

## 3 冗余对照表（建议收口到哪）

| 重复内容 | 出现位置（次数） | 差异 | 建议收口 |
|---|---|---|---|
| 设置子页通用样式（`.back-btn / .settings-page / .setting-group / .setting-item / .sub-header / .divider / .modern-switch / .slider / .mini-btn-reset / .section-header` + h3/h4/p） | 11 个 `Settings*.vue` 各自的 `<style scoped>`，每文件 40-60 行，合计约 500 行 | 个别页有微调（如 `SettingsTheme` 的 `.setting-group` padding） | 新建 `styles/settings.css`，组件只留差异 |
| `nextTick / 双 requestAnimationFrame 再解除 no-anim` | `Settings.vue:197-199` + `SettingsDns/Lan/Network/Tun.vue` 各 1 处（5 份） | 逐字相同 | `utils/nextPaint.ts` |
| 复制到剪贴板 + toast | `App.vue:576-581`、`App.vue:591`、`SettingsDiagnostics.vue:245-251`、`Subscriptions.vue:435-440` | 失败文案硬编码 vs i18n | `utils/clipboard.ts`（U-18） |
| inline SVG 图标 | `utils/icons.ts`（45 个）+ `SettingsTheme.vue:313-326`（13 个，其中 reset/check/plus/sun/moon/system 与 ICONS 重复）+ `SettingsLan.vue`（6 个） | `SettingsTheme` 的尺寸/描边略有不同 | 图标本体复用 `utils/icons.ts`，尺寸做变体；或统一 `<Icon name size>` 组件 |
| 玻璃模糊/染色常量 | `glass.ts:11-22`（JS 常量）+ `tokens.css:63-93`（CSS 兜底值） | overlay 半径 28 vs 40；card/soft/content 两处各一套 | CSS 侧保留设计常量，JS 只写"算出来的"（tint-bg / fill-*），删掉 JS 里的半径默认值 |
| `--ui-radius-*` | `tokens.css:43-45` + `theme.ts:412-415` + 155 处硬编码 | sm/md 无消费者 | 见 U-04 |
| 全局 `@keyframes fadeIn / pulse` | `style.css:711-733` + 组件 scoped 同名副本（5 处） | 参数不同（0.2s/0.3s、1.5s/2s） | 删全局副本（U-16），或抽 `styles/keyframes.css` 并在组件引用（注意 scoped 重命名） |
| 错误分类表 | `errors.ts:10-20 RULES` + `errors.ts:84-90 CLASSIFY_RULES` + `store.ts:936-955 NOTIFICATION_UI` | 三张表用三种维度描述同一批后端错误 | 合并成一张 `{ code, regex, message, kind, level, ui }` 表，三处查询函数都从它派生 |
| `localStorage` 读写 | `store.ts` 5 处、`Proxies.vue` 4 处、`Connections.vue` 2 处、`Rules.vue` 2 处、`locales/index.ts` 1 处、`theme.ts` 1 处 | 只有 3 处有 try/catch | `utils/storage.ts`（U-09） |
| 高频应用/节流预览 | `SettingsTheme.vue:566-572`（rAF `glassFrame`）、`:404-416`（120ms `themePreviewTimer`） | 两套实现同一目的（玻璃按帧、主题按 120ms 合并） | 可保留（注释已解释差异），建议抽 `useCoalescedApply(fn, opts)` |
| 时长格式化 | `formatBytes / formatSpeed / formatEtaTime / formatDurationCn / formatRelativeTime` | `formatDurationCn` 已死、`formatEtaTime` 缺小时档 | 合并成 `formatDuration(seconds, style)`（U-06/U-15） |

---

## 4 已核查且无问题的点

> 覆盖度证明。以下都是**读过实现并验算过**的，不是"没看到问题"。

**utils/theme.ts**

1. 颜色工具全部验算：`parseHex` 支持 `#RGB`/`#RRGGBB`、非法返回 null；`toHex` 有 `clamp255`；`relativeLuminance` 是标准 WCAG 分段函数（0.03928 / 12.92 / 2.4）；`contrastForeground` 的 0.179 阈值有正确推导（`sqrt(0.0525)-0.05`），不是拍脑袋的 0.45。
2. `mixHex(a,b,weight)` 的 weight 是 a 的占比，`shadeHex` 的符号方向与它一致（amount>0 取白），无反向。
3. `normalizeThemeConfig`：非法 `mode/preset/radius` 全部回落默认，非法 `accent` 变空串（等价"跟随预设"），`aura` 缺省 true —— 与样式层/设置页的消费语义一致。
4. `resolvePalette/getPreset/resolveAccent` 的"基准 + 预设 tone 覆盖"正确（`Object.assign` 顺序对）；12 个预设的 tone 只覆盖 `appBg/surface/surfaceHover/panel`，`textMain/Sub/Muted` 统一走 `BASE_PALETTE` —— 这是刻意设计（`theme.ts:51-53` 注释），不是漏写。
5. `applyTheme` 只写 `data-*` 与 CSS 变量，**不写任何内联 background**（`theme.ts:5-6` 的约束成立），因此不会盖死 token。
6. `onSystemThemeChange` 有 `addEventListener → addListener` 回退，返回的 disposer 在 `App.vue:771/979` 正确释放；`systemPrefersDark` 对 `window/matchMedia` 缺失做了保护。
7. `bootstrapTheme` 在 `createApp` 之前调用（`main.ts:11`），"首帧不闪"的前提成立。
8. `cacheTheme/readCachedTheme` 都有 try/catch，`normalizeThemeConfig` 对 JSON.parse 的脏数据免疫（错误值回落默认而不是抛错）。

**utils/glass.ts**

9. `normalizeGlassStrength` 对 `null/undefined/NaN` 的区分处理是对的（`:37` 显式挡 `Number(null) === 0`），`normalizeGlassDepth` 同理。
10. `depthLerp` 在 `depth = 50 / 100` 两端连续（左段到 anchors[1]、右段从 anchors[1] 起），无跳变。
11. `glassTintColor`：`chroma = 0` 严格返回面板色；目标色中性时整体短路回面板色（`:229-231`），避免"灰面板被染成红色"；明度夹在 [22,82]；色相走最短弧（`lerpHue`）；`formatAlpha` 限 0..1 并保留 3 位。
12. `applyGlassEffect` 读 `getComputedStyle(root)` 发生在 `applyTheme` 写完内联变量之后（`store.ts:108/123` 的顺序），且 `refreshGlassTint` 用 `lastApplied` 重放参数 —— 注释里描述的"切暗色玻璃不跟随"bug 已被正确修掉。
13. `--glass-fill-overlay` 的 JS 常量 46 与 `tokens.css:92` 的 46% 一致（soft 48 / card 62 也一致）。

**utils/errors.ts**

14. `rawText` 覆盖 string / Error / `{message}` 对象 / `String()` 兜底四种形态，且都包在 try 里。
15. `RULES` 的顺序设计（结构→权限→端口→内核→HTTP→helper→超时）与其头注释一致；系统错误码规则带上下文括号（`(10013)` / `] 10013`）避免误伤端口号 —— 这个细节做对了。
16. `classifyError` 未命中返回 `unknown/C`，而所有调用点（`useTunToggle.ts:77-127`、`Overview.vue:248`）只按 `kind` 分支、`level` 无消费者 —— 因此"C = 不可恢复"的默认值**没有**被误用成"自动升级为致命"，不构成问题。
17. `displayErrorMessage` 的"人话优先、仅 unknown 追加日志后缀"逻辑与其注释一致。

**utils/format.ts**

18. `formatBytes` 对 `0 / 负数 / NaN / Infinity` 全部返回 0 B；`Math.log` 前有 `Number.isFinite` 守卫；`sizes` 有上限夹取 —— 边界安全。
19. `formatSpeed` 复用 `formatBytes` 加 `'/s'`，对 0 输出 `0 B/s`，不泄漏 NaN。
20. `formatRelativeTime` 的秒/毫秒判定（`> 9999999999`）与后端 `lastUpdateCheck`（Unix 秒）一致；负 diff（未来时间戳）落到"刚刚"，不会出负数分钟。

**utils/report.ts**

21. `reportError` 的 `console.error` 包在 try 里（console 被劫持/不存在时不二次抛错），环形缓冲有 `MAX_RECORDS = 50` 上限。

**utils/edgeFade.ts**

22. `begin()` 先 `clear()` 再排两个定时器，`safetyTimer = paneMs + 600` 的兜底确实能防"edge-fade-on 卡住"；`onUnmounted` 移除了 resize 监听。
23. `cssMs` 单位处理正确：`0.5s → 500`、`220ms → 220`（`endsWith('ms')` 判断在读 `parseFloat` 之后，顺序没问题）；读不到时回落 500/220。

**utils/scrollFade.ts**

24. 三个选择器（`.ui-page-body`、`.proxy-tabs-track`、`.rules-tabs-track`、`.conn-tabs-track`）在模板里**全部存在**（grep 计数：`ui-page-body` 37 处、`proxy-tabs-track` 11 处、`rules-tabs-track` 6 处、`conn-tabs-track` 8 处），不会出现"永远算不出 fade"的静默失效。
25. `data-fade-*` 不在 `attributeFilter: ['class','style']` 里，所以 `apply()` 写 dataset **不会**触发 MutationObserver 自激循环 —— 这个坑避开了。

**utils/hoverGuard.ts**

26. 递归收集时 `CSSStyleRule` 先判再 `continue`、其余 `CSSGroupingRule` 走 `cssRules`（`@media` 内的 `:hover` 也能收到），且每层都有 try；`installHoverGuard` 返回的 dispose 在 `App.vue:976-978` 被调用。

**样式层**

27. `main.ts:3-6` 的 import 顺序（style.css → tokens → surfaces → layout）与 `tokens.css:1-4` 注释一致，`--ui-*` 覆盖 legacy 是设计而非意外。
28. `tokens.css:34` 的 `--glass-border: var(--ui-border-subtle)` 写在被引用变量之前**没问题**：自定义属性是惰性替换（使用点求值），不是顺序敏感。
29. `layout.css` 的 `@property --ui-fade-*`（`syntax: '<length>'; inherits: false`）与 `style.css:495-510` 的 transition 组合，是"mask 渐变停靠点可过渡"的正确写法；`inherits:false` 也避免了漏给后代。
30. `.ui-page-body`（滚动渐隐）与 `.view-scroller`（切页渐隐）两套 mask 分工清晰，各自只在自己的选择器上写 `transition@@，没有互相覆盖（`style.css:474-478` 警告的 `.view-transition-wrapper` 上确实没写 transition）。
31. `html[data-glass="on"]` 的系列规则（`style.css:1317-1523`）与 tokens 的 `html[data-glass="on"]` 覆盖（`tokens.css:137-145`）不冲突：前者改具体元素 background，后者改 `--ui-surface-*` 变量。
32. `style.css:322` 与 `:1415` 的同名选择器 `html[data-glass="on"] :is(.ui-surface-overlay, .custom-modal-card)` 两条规则分别管 `backdrop-filter` 与 `box-shadow`，属性不重叠，不构成"改了不生效"。

**locales**

33. 8 个语言包**键集完全一致**（每个 795 叶子键，无缺、无多、无文件内重复）；`t()` 有 zh-CN 二级 fallback + 返回 path 的三级兜底，不会白屏。
34. 全部 `t('字面量')` 共 642 个 key，**全部存在**（`scripts/check-i18n.mjs` 的 error 级检查为 0；本地独立复算一致）。
35. `settings/manifest.ts` 里 125 个动态 key（`titleKey/descKey/group/note`）**全部存在**于 zh-CN（脚本对这类动态 key 是盲区，我单独校验了）。
36. `t()` 的 `{name}` 插值（`locales/index.ts:70-74`）对缺参保留原占位符，不会输出 undefined。

**其它（全前端普查）**

37. `initStore` 幂等（`store.ts:975-979 storeInited`），`main.ts:12` 与 `App.vue:757` 双调用**不会**重复注册 22 个 `Events.On`。
38. `App.vue` 的 14 个 `Events.On` 全部存了 unsub，并在 `onUnmounted`（`App.vue:976-1016`）统一释放；`unsubSystemTheme`、`uninstallHoverGuard` 也在内。
39. 全仓 `debugger` 0 处、`@ts-ignore/@ts-expect-error` 0 处、`TODO/FIXME` 0 处；只有 2 处 `console.log`（`App.vue:905`、`Subscriptions.vue:365`）、3 处 `as any`。
40. 55 处 `v-html` **全部**是图标常量（`ICONS.* / THEME_ICONS.*` / `EmptyState` 的 `icon` prop 由调用方传常量），无用户/后端字符串进入 `v-html`，不构成注入面。
41. `.ui-surface-*` 类名在模板里的使用与 `surfaces.css` 定义一一对应（`ui-surface-card` 18 处、`ui-surface-group` 22 处、`ui-surface-overlay` 1 处），没有"style 里定义了但模板从不使用"的 surface 类。
42. 主题切换的三个入口之间**不会互相回灌**：`SettingsTheme.vue:614-626` 的 `themeState.config` watcher 有 `themeFingerprint` 比较，`applyThemeToApp` 写的 `themeState.config` 不会再次触发预览应用（无循环）。
43. `--pane-duration / --pane-edge-fade-out / --pane-ease` 的"CSS 定义 → JS 读取"链路闭合（`style.css:353-357/457-467` → `edgeFade.ts:20-27`），没有第二份时长常量。

---

## 5 已知问题（历史已记录，不计入本轮新问题数）

| 历史 ID | 台账状态 | 与本报告的关系 |
|---|---|---|
| **F-A4-14**（P1，中文正则判断后端语义） | fixed(B3) | **机制仍在**，只是从 `store.ts` 搬到 `utils/errors.ts:119-129`（`LONG_CONNECTION_DEFERRAL_RE` / `'路由切换中'`），注释还带 ID 标记。后端生产端确认为 `controller_long_connection.go:39/196`、`outbound_ip.go:203`。归属历史，本轮不重复计数。 |
| **F-A4-16**（P2，硬编码文案绕过 i18n） | fixed(B3) | 当时只修了 4 处（SettingsAbout×2 / SettingsTun / Proxies）。本报告 **U-03 是同类的漏网点（utils/ 层）**，文件不同，按本轮判据单列。 |
| **F-A4-17**（P2，locale 键集 777 键 / 1 缺键 / 14 未译 / 141 死键） | fixed(B4，流程化治理) | 本轮复核：键集现为 **795 键 × 8 语言完全对齐、0 缺键**，`check-i18n` 通过。死键结论仍有偏差（脚本报 188、实际真死 11 个 `tray.*`），已作为 **U-21** 单列（含"脚本盲区"建议）。 |
| **F-A4-11**（P1，无痕失败面） | fixed(B2)，引入 `reportError` | 统一出口未贯彻：仍有 10 处 `console.error/warn` 绕过 `reportError`（`App.vue:711/792/804/832/866`、`store.ts:534/537/579/613/617/1208`）。在 U-14 的上下文里说明，不单列 ID。 |
| **F-A4-01 / F-A4-02 / F-A4-07**（单一模态槽 / 直写 modal / 自建模态） | fixed | 本轮未发现回归：`globalState.modal` 只剩 store 内部写与 `App.vue` 单一渲染点；`store.ts:1185-1217` 的 60s 看门狗有效。 |
| **F-A4-04 / F-A4-15**（错误 code 契约） | fixed | 与 U-05 相关但不同：U-05 是**人话映射规则的顺序/宽度**，不是 code 契约。 |

---

## 6 需要跨文件/后端确认的疑点（移交主控）

1. **`SaveAppBehavior` 的整结构覆盖是全前端系统性问题，不只在主题**（U-01 的放大版）：`Settings.vue:204` 每次保存都提交整个 `AppBehavior`。任何后端侧改动（托盘、helper、自动更新写 `lastUpdateCheck`、`StartupWithOS` 失败回滚 `controller.go:2447-2455`）与设置页同时发生，都可能被设置页的旧快照覆盖。建议后端把保存改成"补丁语义"。**需要后端 agent 确认**：除设置页外还有哪些路径写 `Behavior.SetAndSave`（已知 `controller.go:2400` 写 LastUpdateCheck、`:2451` 自启失败回滚）。
2. **`App.SaveThemePreference(isDark)` 与 `Behavior.ThemeMode` 的双写**：`App.vue:747/769` 在 system 模式下把"OS 解析结果"落盘，`main.go` 建窗口时读 `theme_setting.txt`。需确认后端 `syncThemeSettingFile` 与 `SaveThemePreference` 的写入时机有无竞争（`behavior.go:80-82` 注释声称由 `syncThemeSettingFile` 保持同步）。属后端范围。
3. **`StartupTaskInfo` 全链路无人消费**：前端 `utils/normalize.ts` 是唯一提到它的文件且零引用。请确认 `bindings` 里是否存在 `GetStartupTaskInfo / CheckStartupTask` 导出 —— 若存在而前端从不调用，则是"后端能力未上界面"的功能缺口（是否本应出现在"设置 → 行为"页？）。
4. **`AppBehavior` 的 4 个玻璃分档字段（`glassBlurStrength / glassBlurContentStrength / glassBlurOverlayStrength / glassBlurControlStrength`）已是死字段**：Go 注释仍写"3 组独立半径 + tint 独立"，前端已改成单一 `glassDepth` 总控且不写这 4 个字段（`Settings.vue:166` 的兜底对象里还留着旧默认值 12/12/18/8）。请后端确认是否可标 deprecated 并停止返回，避免前端 `SaveAppBehavior` 整结构回写时反复搬运死值。
5. **`--glass-blur-overlay` 到底是 28px 还是 40px**：`tokens.css:67` 是 40px，`glass.ts:13` 常量与所有注释都是 28px，`style.css:322` 消费 CSS 变量。属视觉决策，但会持续误导（U-22）。
6. **`.view-scroller / .ui-page-body` 双层 mask 对 `backdrop-filter` 后代的影响**：`style.css:487-568` 给滚动容器永久挂 mask（边为 0px 时也挂），mask 会建立新的裁剪层。当前玻璃浮层都 Teleport 到 body（`style.css:480-481` 注释），所以**当前**没问题；但 `Toast.vue / ModernSelect` 的 dropdown 若哪天不再 Teleport，会被 mask 裁掉且模糊失效。需组件 agent 确认这两处的挂载方式是否稳定。
7. **`hoverGuard` 改写第三方样式表的风险必须实机验证**：它遍历 `document.styleSheets` 改写**所有**含 `:hover` 的规则（含 CodeMirror 注入的样式表）。需确认：① 编辑器悬停样式是否被误杀；② 是否有规则因 `selectorText` 只读而落入 catch（那会导致永久 `:not(*)`）。本轮禁止运行应用，属"待实测"（U-10）。
8. **`formatRelativeTime` 在 `SettingsUpdate.vue` 的可见性**：我只确认了调用点（`:32/52`），未逐行读该组件模板；若"上次检查更新"这行在 en-US 下被隐藏，U-03 的可见性会下降。请组件 agent 确认。

---

## 附录 A：脚本化统计原始结果（可复算）

```
导出符号：131 个；零外部引用 48 个（其中真正死的是 U-13/U-14/U-15 列的 6 个，其余是同文件内使用的类型/工具）
CSS 规则：1138 条（29 个 style 块）；同名选择器重复 130 组
!important：style.css 30、Proxies.vue 21、Subscriptions.vue 15、App.vue 10、layout.css 3 …
自定义属性：79 个；多处定义 33 个；零消费 14 个
border-radius：硬编码 155 / var() 8
z-index 取值分布：10000×1、9999×1、100×1、50×1、10×1、2×3、1×5、0×6、-1×4
@keyframes 重名：fadeIn(3)、pulse(2)、breathe(2)
console：log 2 / warn 3 / error 10（另有 report.ts 内部 1 处为设计内）
TODO/FIXME：0   @ts-ignore：0   debugger：0   as any：3   : any：89   空 catch：1（App.vue:766）
Events.On 调用点：54（store 22 / App 14 / SettingsUpdate 6 / Proxies 5 / Subscriptions 4 …）
showConfirm 调用点：20（14 个文件）
locales：8 包 × 795 叶子键，完全对齐；t() 字面量 642 个全部命中；manifest 动态 key 125 个全部命中
```

---

**报告结束。**
