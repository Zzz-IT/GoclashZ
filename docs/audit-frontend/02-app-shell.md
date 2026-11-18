# 02 应用外壳与总览页审计（App.vue / Sidebar / Overview / TrafficCard / DeviationStrip / Toast / EmptyState / useTunToggle）

> 生成：2026-11-18
> 范围：frontend/src/App.vue（1392 行，核心）、components/{Sidebar,Overview,TrafficCard,DeviationStrip,Toast,EmptyState}.vue、composables/useTunToggle.ts
> 审计性质：**只读**。未修改任何源码/样式/配置/locales，未运行 pnpm/vue-tsc/应用本体。
> 判据：①代码冗余 ②逻辑错误 ③操作冲突 ④bug（泄漏/资源） ⑤前后端对接（仅本范围内的调用点）
> 证据等级：**A** = 代码可直接确证；**B** = 代码 + 运行时推断（框架行为/事件时序）；**C** = 假设待实测（已标注）

---

## 0 覆盖范围与方法

### 0.1 逐行读过的文件

| 文件 | 行数 | 读法 |
|---|---|---|
| frontend/src/App.vue | 1392 | 全文（template 1-242、script 244-1032、style 1034-1392） |
| frontend/src/components/Overview.vue | 400 | 全文 |
| frontend/src/components/TrafficCard.vue | 339 | 全文 |
| frontend/src/components/Sidebar.vue | 256 | 全文 |
| frontend/src/components/DeviationStrip.vue | 94 | 全文 |
| frontend/src/components/Toast.vue | 23 | 全文 |
| frontend/src/components/EmptyState.vue | 28 | 全文 |
| frontend/src/composables/useTunToggle.ts | 138 | 全文 |
| frontend/src/trafficWaveState.ts | 242 | 全文（TrafficCard/Overview 波形的实际实现） |
| frontend/src/utils/format.ts | 50 | 全文 |
| frontend/src/utils/edgeFade.ts / scrollFade.ts / hoverGuard.ts | 95 / 65 / 73 | 全文（App.vue 直接消费的三个工具） |
| frontend/src/store.ts | 1250 | 定点读约 950 行：globalState 定义（30-320）、intent 覆盖层（169-213）、updateStateFromBackend（325-437）、模态队列（484-703）、IP 刷新（705-925）、initStore（975-1175）、pending 看门狗（1177-1217）、toast（1219-1250） |
| frontend/src/api.ts + bindings/goclashz/{appservice,coreservice,updateservice}.ts + core/*/models.ts | 17 + 108 导出 + 模型 | 导出名全集比对 + 本范围内 25 个调用点的签名逐个核对 |
| 只读参照（后端对账） | — | main.go 280-410（窗口配置）、app.go 70-205/575-615/905-965/1495-1530、core/appcore/controller.go 2000-2271/2340-2403、control_coordinator.go 40-269、updater_orchestrator.go 430-580、logger/level.go、core/logger/file.go 470-514、behavior.go 560-600、appcore/streams.go（grep） |
| 历史报告 | — | docs/audit/A4-frontend.md（383 行）、docs/audit/ISSUE_LEDGER.md（除重对照） |

### 0.2 未覆盖的部分（明确声明）

* App.vue 的 style scoped 只做了「与 UI 行为相关」的核对（窗口按钮 hover、.log-line.*、.global-update-chip），未逐条评估视觉/响应式。
* 本范围内**没有**逐行读：settings/manifest.ts、locales/*.ts（i18n 由其他代理负责）、其他页面组件（仅在需要交叉验证时定点读，例如 Settings.vue、SettingsTun.vue、SettingsBehavior.vue、YamlEditor.vue、Connections.vue 的 emit 契约）。
* 未运行应用，因此「视觉/时序」类结论最高只到 B；标注 C 的条目需要实测。

### 0.3 与前一轮的关系

上一轮（2026-11-16，A4）主线是「提示面与错误处理」；本轮判据不同。若结论与旧 ID 重复，一律单列到 §6，不计入本轮新问题数。

---

## 1 问题总表

| ID | 严重度 | 类别 | 位置(file:line) | 一句话问题 | 证据 |
|---|---|---|---|---|---|
| H-01 | P1 | 逻辑/状态残留 | App.vue:331,375 + App.vue:170 | 更新 chip 跳到设置页后 targetSettingsView 永不复位，此后每次进设置都落在「关于」页 | A |
| H-02 | P1 | 逻辑错误 | trafficWaveState.ts:67-72 | 波形自动缩放只升不降（scaleFallAlpha 不可达），一次高速流量后波形长期被压平 | A |
| H-03 | P1 | 操作冲突 | Overview.vue:196-199、TrafficCard.vue:153-155、App.vue:11/356 | 防重入闸门在 await showConfirm 之后（或完全没有），连点会排队 N 个确认框并执行 N 次后端动作（重启内核 ×2、重置流量 ×2、跑安装器 ×2） | A |
| H-04 | P2 | 逻辑/UX | App.vue:526-539 | 日志面板无条件滚到底：有新日志时用户无法向上阅读（每 100ms 被拽回底部） | A |
| H-05 | P2 | 性能 | App.vue:154,412-447,496-539 | 日志列表 2000 行 + :key="i" + 每 16ms 全量 filter/重渲染，info 档下持续掉帧 | B |
| H-06 | P2 | 逻辑/冗余 | App.vue:396,1204 + 后端 level.go:5-18 | .log-line.warning 永不命中（后端只产 warn），warn 行失去视觉区分；logLevels.warning 是死键 | A |
| H-07 | P2 | 对接/枚举 | App.vue:390,417,420 + logger/file.go:498 | LogSource 有 std/core/app 三值，前端筛选器只有 all/core/app，选 core/app 时 std 日志静默消失；缺 source 时回落成 core（错误归属） | A |
| H-08 | P2 | 逻辑/UX | App.vue:414-431,134-140 | 面板级别下拉的「全部」被全局级别地板压制，控件标签与行为自相矛盾（全新安装默认 core=error，选 DEBUG 仍只有 ERROR） | A |
| H-09 | P2 | 竞态 | App.vue:828-833,837-839 | GetRecentLogs() 用整体赋值覆盖 logLines，启动窗口内已收到的活日志被丢弃；且订阅晚于 StartStreamingLogs | A/B |
| H-10 | P2 | 错误处理 | App.vue:964-974 | handleClearLogs 先清空前端再 await App.ClearLogs()，无 try/catch：后端失败时前后端不一致且未处理拒绝 | A |
| H-11 | P2 | 对接/冲突 | App.vue:356-377,908-939 + 后端 updater_orchestrator.go:521 | 后端发 app-update-busy，前端零监听；下载/安装入口无 in-flight 互斥，第二次点击完全无反馈 | A |
| H-12 | P2 | 错误处理不一致 | App.vue:359-372 vs App.vue:918-926 | 同一个「安装」动作：事件路径对空 path 弹错误框，chip 路径直接静默 return | A |
| H-13 | P2 | 对接/时序 | App.vue:843-845,764-766 | runtime-assets-status 在后端 ServiceStartup 里就 emit 了，前端到 onMounted（且在首个 await 之后）才订阅；可用 GetRuntimeAssetStatus() 兜底却未用 | B |
| H-14 | P2 | 状态冲突 | useTunToggle.ts:129-134、Overview.vue:256-265 | 失败路径前端直接改写后端权威字段 desiredTun/desiredSystemProxy，抵消后端回滚，卡片可能显示与后端意图相反的状态 | A/B |
| H-15 | P2 | 逻辑错误 | Overview.vue:299-320 | 模式切换失败时无条件回滚到 previousMode，但后端有「已落盘、只是内核热切失败」的分支，此时回滚把 UI 与后端真相拉开 | A |
| H-16 | P2 | 逻辑错误 | Overview.vue:101-103 | sliderStyle 用 findIndex，globalState.mode 不是三值之一时返回 -1 → 滑块跑到 -33.33% 且无选中项 | A |
| H-17 | P2 | 操作冲突 | Sidebar.vue:70-76 + store.ts:134-145,140 + Settings.vue:204 | 侧栏明暗开关走「Get 全量 behavior → 改一个字段 → 整对象 Save」；与设置页整对象保存在同一进程内互相覆盖 | A/B |
| H-18 | P2 | 冗余/死代码 | Sidebar.vue:5-13,206-255 | TransitionGroup 里用 v-show 隐藏导航项，enter/leave/move 过渡永不触发，整套 nav-slide CSS（约 50 行）是死代码 | B |
| H-19 | P2 | 性能/竞态 | App.vue:671-673,856,680-685 | resize 每个事件一次 Window.IsMaximised() IPC（无节流），且并发回调可能乱序，还原最大化图标 | B |
| H-20 | P2 | 逻辑/对接 | TrafficCard.vue:102-114 + store.ts:275,869-874 | outboundIPStale 只写不读：切换代理/检测失败后保留的旧 IP 被当作当前值展示，没有任何「已过期」提示 | A |
| H-21 | P2 | 对接/冗余 | DeviationStrip.vue:12-27,39 | 后端 ConfigDeviation.severity（ignored/forced）被完全忽略；v-for 用 index 作 key | A |
| H-22 | P2 | UX | Toast.vue:1-23 + store.ts:1226-1245 | 无 hover 暂停、无同文案去重：2.4s 定时照走，连续失败会堆 3 条一模一样的 toast | A |
| H-23 | P2 | a11y | App.vue:134 | 日志级别 select 的 aria-label 用了 logs.all（「全部」），标签描述的是错误控件 | A |

> 统计：P0 = 0，P1 = 3，P2 = 20。P0 为 0 的结论有依据：本范围内的模态已收敛到 store 队列（含 60s 看门狗兜底），pending 闸门覆盖了 TUN/系统代理两个高频开关，未发现「状态永久卡死」或「崩溃/数据损坏」路径。

---

## 2 逐条详情

### H-01 更新 chip 跳转后 targetSettingsView 永不复位

**位置**：App.vue:331、App.vue:373-376、App.vue:169-171

**现状**：

```ts
// App.vue:331
const targetSettingsView = ref('main');
...
// App.vue:356-377  handleGlobalUpdateChipClick
  } else {
    currentTab.value = 'settings';
    targetSettingsView.value = 'about';   // 只写不复位
  }
```
```vue
<!-- App.vue:169-171；Settings 在切页时被 v-if 销毁重建 -->
<div v-else-if="currentTab === 'settings'" key="settings" class="view-transition-wrapper view-settings">
  <Settings :initialView="targetSettingsView" />
</div>
```
```ts
// Settings.vue:42 / 248
const view = ref(props.initialView as ...);
watch(() => props.initialView, value => { view.value = value as typeof view.value; });
```

**为什么是问题**：targetSettingsView 全局仅有两处写：初始化 'main'（:331）与 chip 点击 'about'（:375），**没有任何地方复位**。Settings 组件是 v-if 重建的，每次重新进入设置页都会用 initialView 初始化 view。
触发路径：点一次更新 chip（下载中/已下载时出现）→ 之后**每一次**点侧栏「设置」都直接落在「关于」页，而不是设置主页，直到重启应用。用户看到的是「设置页记住了我第一次点的地方」这一残留态。

**修复建议**：在切页 watcher（App.vue:650-659 那个已有 watcher）里加复位，或用一次性消费：

```ts
watch(currentTab, (newVal) => {
  if (newVal !== 'yaml-editor') { /* 现有清理 */ }
  if (newVal !== 'settings') targetSettingsView.value = 'main';  // 离开设置页即复位
});
```

（等价做法：chip 里改用 nextTick 后置回 'main'，或把 initialView 改成一次性 v-if 挂载参数。）

---

### H-02 波形自动缩放只升不降

**位置**：frontend/src/trafficWaveState.ts:67-72（消费方 TrafficCard.vue:145-151）

**现状**：

```ts
const updateScale = (prevScale: number, value: number) => {
  const instant = Math.max(WAVE.minScale, value * WAVE.scaleHeadroom);
  const target = Math.max(instant, prevScale);                       // ← target 恒 >= prevScale
  const alpha = target > prevScale ? WAVE.scaleRiseAlpha : WAVE.scaleFallAlpha;  // fall 分支只在相等时进入
  return prevScale + (target - prevScale) * alpha;                   // 相等时增量恒为 0
};
```
```ts
// :120-124 每 900ms 采样一次
const upRatio = toVisualRatio(s.smoothedUpload, s.uploadScale);
s.uploadScale = updateScale(s.uploadScale, s.smoothedUpload);
```

**为什么是问题**：target = Math.max(instant, prevScale) 使 target >= prevScale 恒成立，于是返回值单调不减；scaleFallAlpha（0.025）与「回落分支」在数学上不可达（唯一进入条件 target === prevScale 时增量为 0）。uploadScale/downloadScale 只会被 resetWaveState() 归零，而它只由「切换流量统计模式」和「手动重置流量」调用。
影响：一旦出现过一次高速下载（例如 100 MB/s → scale 锁在 ~118 MB），之后 1~10 MB/s 的常规流量在 toVisualRatio 的高段被压缩（t=(v-1MB)/(scale-1MB)），比值长期贴在 0.62~0.65，**波形看起来「不动了」**，直到用户手动重置流量或切换统计模式。这正是「auto scale」应有的慢回落被写成了棘轮。

**修复建议**：让 target 允许回落，落回速率单独控制：

```ts
const updateScale = (prevScale: number, value: number) => {
  const instant = Math.max(WAVE.minScale, value * WAVE.scaleHeadroom);
  const alpha = instant > prevScale ? WAVE.scaleRiseAlpha : WAVE.scaleFallAlpha;
  return prevScale + (instant - prevScale) * alpha;
};
```

若担心回落让波峰反复顶格，可加下限保护：return Math.max(WAVE.minScale, prevScale + (instant - prevScale) * alpha)。

---

### H-03 确认框之前的 await 让防重入闸门失效（连点 = N 次动作）

**位置**：Overview.vue:195-209（重启内核）、TrafficCard.vue:153-160（重置流量）、App.vue:11 + 356-377（更新 chip 安装）

**现状**：

```ts
// Overview.vue:195-209
const handleRestartCore = async () => {
  if (isRestarting.value) return;                 // ← 闸门在 await 之前
  const ok = await showConfirm(...);              // ← 在这里让出控制权
  if (!ok) return;
  isRestarting.value = true;                      // ← 两次点击都已通过闸门
  try { await App.RestartCore(); ... }
};
```
```ts
// TrafficCard.vue:153-160 —— 完全没有闸门
const handleReset = async () => {
  const ok = await showConfirm(t('traffic.resetConfirmMsg'), t('traffic.resetConfirmTitle'));
  if (!ok) return;
  try { await App.ResetTrafficTotals(); resetWaveState(); showToast(...); }
};
```
```ts
// App.vue:356-372 —— chip 的确认回调里直接 ApplyAppUpdate，同样无 pending
if (p.isDownloaded) { void openModal({ type:'confirm', ..., onConfirm: async () => { if (!p.path) return; await App.ApplyAppUpdate(p.path); } }); }
```

**为什么是问题**：store 的模态是**队列**（store.ts:623-645），两次点击不会互相顶掉，而是排两个确认框。用户在第一个框点「确定」→ RestartCore() 执行、isRestarting=true；第一个 finally 结束后第二个框才上台，用户再点确定 → **第二次 RestartCore() 依然执行**（因为闭包里早已通过闸门判断）。内核被连根重启两次：第一次刚起来就被第二次打断，正在跑的连接全断，且界面上只有一个「重启中」图标，用户无从判断发生了两次。
同类：重置流量 ×2（第二次把刚归零的计数再清一遍，并弹两条成功 toast）；chip 安装器 ×2。
触发路径：双击（或点一下没反应再点一下，GetAppBehavior/渲染延迟下很常见）→ 两个模态 → 两个确定。

**修复建议**：闸门提到 showConfirm **之前**（或用一个独立的 confirming 标志），并在 finally 里复位：

```ts
const handleRestartCore = async () => {
  if (isRestarting.value) return;
  isRestarting.value = true;               // ← 先占坑，连点直接 return
  try {
    const ok = await showConfirm(t('overview.restartConfirmMsg'), t('overview.restartConfirmTitle'));
    if (!ok) return;
    await App.RestartCore();
    showToast(t('overview.restartSuccess'), 'success');
  } catch (e) { ... } finally { isRestarting.value = false; }
};
```

TrafficCard 同理加 let resetting = false;；chip 的 onConfirm 加 globalState.updateBusy 之类的互斥（与 H-11 一起改）。

---

### H-04 日志面板无条件滚到底，无法向上阅读

**位置**：App.vue:526-539（flushLogBatch）、App.vue:1023-1031（resetViewScroller）

**现状**：

```ts
const flushLogBatch = () => {
  logBatchTimer = null;
  if (logBatch.length === 0) return;
  logLines.value.push(...logBatch);
  ...
  if (!scrollTimer) {
    scrollTimer = setTimeout(() => {
      logBox.value?.scrollTo({ top: logBox.value.scrollHeight });  // ← 无条件到底
      scrollTimer = null;
    }, 100);
  }
};
```

**为什么是问题**：只要后端在推日志（core 级别调到 info/debug 时几乎每帧都有），每 100ms 就会把 .terminal-box 拽到最底部。用户向上翻看历史错误时会被反复打断——这是日志面板最核心的可用性动作。代码里没有任何 nearBottom（scrollHeight - scrollTop - clientHeight < 阈值）判断。
影响范围：仅日志页；用户可点「暂停」绕过（F-A3-16 加的），但那是额外的一次操作，且暂停后新行堆在 logPausedBuffer，恢复时仍会跳到底。

**修复建议**：

```ts
const pinnedToBottom = () => {
  const el = logBox.value; if (!el) return true;
  return el.scrollHeight - el.scrollTop - el.clientHeight < 24;
};
// flushLogBatch 里：只在刷新前已贴底时才跟到底
const shouldFollow = pinnedToBottom();
...
scrollTimer = setTimeout(() => {
  if (shouldFollow) logBox.value?.scrollTo({ top: logBox.value.scrollHeight });
  ...
}, 100);
```
resetViewScroller() 里的「切到日志页滚到底」是合理的（新页面），保留。

---

### H-05 日志列表：index key + 每 16ms 全量过滤重渲染

**位置**：App.vue:154、412-447、496-539

**现状**：

```vue
<div v-for="(log, i) in filteredLogLines" :key="i" :class="['log-line', log.type]">
```
```ts
const LOG_BATCH_MS = 16;          // 每 16ms 合并一批
const LOG_MAX_LINES = 2000;       // 最多渲染 2000 行
const filteredLogLines = computed(() => logLines.value.filter(log => { /* 每行 4 个字符串拼接 + 若干比较 */ }));
```

**为什么是问题**：
1. :key="i"：数组从头部 splice(0, overflow) 裁剪时，所有 key 平移，Vue 会把每一行都当成「同位置内容变了」重 patch —— 无法利用 key 复用，也破坏了滚动锚点。
2. 批量窗口 16ms 意味着「批次数」可达 60/s；每次 flush 都会让 filteredLogLines 失效 → 2000 条 filter + 2000 行 VNode diff。注释里说批量是为了「避免每次重算并触发重渲染」，但 16ms 只解决了「push 次数」，没解决「单次渲染成本」。
3. 证据等级 B（未实测帧率），但「2000 行 diff @60Hz」在任何 Chromium 上都不成立，只是卡顿幅度需实测。

**修复建议**：
- 用稳定 key：后端 LogEntry 补 seq（或前端 enqueueLogLine 时自增 uid = ++logSeq 挂到对象上），:key="log.uid"。
- 渲染上限与滚动窗口分离：日志页只渲染最后 N 行（例如 300）+「上方还有 M 条，点击加载」；或在 filteredLogLines 末尾 .slice(-800)。
- batch 间隔提到 100~200ms（日志不是实时动画），或在暂停/不可见时直接不 flush。

---

### H-06 .log-line.warning 永不命中 + logLevels.warning 死键

**位置**：App.vue:396、App.vue:1203-1206；生产端 core/logger/level.go:5-18

**现状**：

```ts
// App.vue:396
const logLevels = { debug: 0, info: 1, warn: 2, warning: 2, error: 3 };
// App.vue:398-405 normalizeLogType 会把 'warning' 归一成 'warn'
```
```vue
<!-- App.vue:154 模板用的是「原始」 log.type，没有再归一 -->
:class="['log-line', log.type]"
```
```css
/* App.vue:1203-1206 */
.log-line.warning .l-type { color: var(--text-sub); font-style: italic; }
```
```go
// core/logger/level.go:11-12 —— 后端只有 warn，永远不会产出 "warning"
case "warn", "warning":
    return "warn"
```

**为什么是问题**：后端 NormalizeLevel 把所有级别归一到 debug/info/warn/error（slog 路径同样只给 warn，见 core/logger/slog.go:80-91），所以 log.type 永远不是 warning。结果是：
- 该死 CSS 规则永不命中 → **WARN 行丢失「次要色 + 斜体」的视觉区分**，看起来和 INFO 一样。
- logLevels 里的 warning: 2 是死键（rankOf() 先归一，永远不会查到它）。

**修复建议**：模板里用归一后的类型：
```vue
<div v-for="(log, i) in filteredLogLines" :key="i" :class="['log-line', normalizeLogType(log.type)]">
```
（normalizeLogType 是 setup 里的 const，模板可直接引用。）随后删掉 .log-line.warning 与 logLevels.warning。

---

### H-07 LogSource 三值，前端只筛两值

**位置**：App.vue:390、App.vue:416-422、模板 123-125；后端 core/logger/file.go:39-40,494-500

**现状**：

```ts
const logSourceFilter = ref('all');
...
if (logSourceFilter.value !== 'all' && source !== logSourceFilter.value) return false;
```
```vue
<!-- 只有 all / core / app 三个 tab -->
<button ... @click="logSourceFilter = 'core'">{{ t('logs.core') }}</button>
<button ... @click="logSourceFilter = 'app'">{{ t('logs.app') }}</button>
```
```go
// logger/file.go：标准库 log 收编的条目走同一 sink
s.enqueue(record{ level: "info", source: string(LogSourceStd), payload: text })   // source = "std"
```
```ts
const source = String(log.source || 'core').toLowerCase();   // 缺字段时错误归属成 core
```

**为什么是问题**：std 来源（内核/运行组件的标准库输出，恰恰是排障最需要的那批）在选「core」或「app」时被静默过滤掉，而 UI 没有第四个入口；默认 all 能看到，但用户一旦按来源筛选就会误以为「没有内核日志」。
另外缺 source 的历史条目被当作 core，与后端语义（未知来源）不符。

**修复建议**：要么把筛选器改成 core | app | std（把 all 作为默认「全部」并允许组合），要么在过滤时把 std 归到「core」一侧并在 i18n 里说明；回落值改为 'app' 或新增 'unknown'。

---

### H-08 面板级别下拉的「全部」被全局地板压制

**位置**：App.vue:414-431、App.vue:134-140

**现状**：

```ts
const filterRank = logLevelFilter.value === 'all' ? 0 : rankOf(logLevelFilter.value);
const settingsFloor = source === 'core' ? rankOf(globalState.logLevel) : rankOf(globalState.appLogLevel);
if (entryRank < Math.max(settingsFloor, filterRank)) return false;
```
```vue
<select v-model="logLevelFilter" ...>
  <option value="all">{{ t('logs.all') }}</option>   <!-- 「全部」实际被地板限制 -->
  <option value="debug">DEBUG</option> ...
```

**为什么是问题**：默认 LogLevel = "error"（core/appcore/behavior.go:573），而面板下拉的第一项写着「全部」。用户选「全部」或「DEBUG」时仍然只看得到 ERROR；这不是提示不足，而是**控件标签与实际行为矛盾**（注释 F-A3-16 说这是「地板」设计，但 UI 没有把这个语义透出来）。用户容易得出「日志系统坏了」的结论。

**修复建议**：把第一项改成「跟随全局（error）」并在 title 里显示当前地板值；或允许面板覆盖全局（把 Math.max 换成面板值优先，并在超出时提示「已超出全局级别，仅本次有效」）。

---

### H-09 GetRecentLogs 覆盖式赋值 + 订阅晚于 StartStreamingLogs

**位置**：App.vue:828-839

**现状**：

```ts
void (async () => {
  try {
    const history = await App.GetRecentLogs();
    if (history) logLines.value = history;      // ← 整体覆盖
  } catch (e) { console.error("load recent logs failed:", e); }
})();

App.StartStreamingLogs();                        // ← 先开流
unsubLogMessage = Events.On("log-message", ({ data: log }) => { enqueueLogLine(log); });  // ← 后订阅
```

**为什么是问题**：两个独立问题：
1. 历史快照是「await 之前」的状态；等待期间通过 log-message 收到并 flush 进 logLines 的新行，会被 logLines.value = history **整体丢弃**（后端不会重发），表现为启动瞬间日志列表有一个空洞。
2. StartStreamingLogs() 在订阅之前调用：这两行之间（几毫秒）推来的日志无人接收；虽然 1 里能补一部分，但两者叠加正是「启动期日志丢一段」的经典组合。

**修复建议**：先订阅再开流；历史用前缀合并而不是覆盖：
```ts
unsubLogMessage = Events.On("log-message", ...);   // 先订阅
void (async () => {
  const history = await App.GetRecentLogs().catch(() => null);
  if (history?.length) {
    const live = logLines.value;                    // 等待期间已收到的活日志
    logLines.value = [...history, ...live].slice(-LOG_MAX_LINES);
  }
  App.StartStreamingLogs();
})();
```
（更彻底：后端 LogEntry 加 seq，前端按 seq 去重合并。）

---

### H-10 handleClearLogs 无 try/catch

**位置**：App.vue:964-974

**现状**：

```ts
const handleClearLogs = async () => {
  const confirmed = await showConfirm(t('logs.clear'), t('common.warning'), true);
  if (!confirmed) return;
  logLines.value = [];
  logBatch = [];
  logPausedBuffer = [];
  await App.ClearLogs();        // ← 无 catch
};
```

**为什么是问题**：破坏性动作「先清前端、再清后端」。App.ClearLogs() 被拒绝时（绑定错误/后端未就绪）会：① 产生未处理的 Promise 拒绝（控制台报错，用户无任何提示）；② 前端已空、后端环还在 —— 下一个 log-message 到来时旧条目不再补回，但用户若重启应用又会看到「明明清空过」的旧日志。属于 F-A4-22「短命令缺少错误分支」的同类，但该条清单未覆盖此函数。

**修复建议**：先调后端，成功再清前端：
```ts
try {
  await App.ClearLogs();
  logLines.value = []; logBatch = []; logPausedBuffer = [];
  showToast(t('logs.cleared'), 'success');
} catch (e) {
  reportError(e, 'logs.clear');
  showToast(displayErrorMessage(e), 'error');
}
```

---

### H-11 app-update-busy 无人监听 + 更新入口无互斥

**位置**：App.vue:356-377（chip）、App.vue:881-939（available/downloaded 事件）；后端 updater_orchestrator.go:504-523

**现状**：

```go
// updater_orchestrator.go:507-522
ok := c.Tasks.RunIfIdle(ctx, appUpdateDownloadTask, false, func(ctx context.Context) error { ... })
if !ok {
    c.events.Emit("app-update-busy")     // ← 后端明确会发
}
```
前端：grep app-update-busy frontend/src = **0 命中**（store.ts:1103-1123 只处理 progress/downloaded/error，App.vue 只处理 check-start/end/cancelled/available/start/downloaded/none/error）。

**为什么是问题**：用户在「chip → 确定安装」和「事件弹框 → 确定下载」两个入口都能触发动作，且两处都没有 in-flight 标志。第二次触发时后端只发一条 app-update-busy，前端没有任何反应 —— 用户点了「确定」却什么都没发生，也没有「正在下载中」的说明。这是「后端已经给了语义，前端没有消费」的典型对接缺口。

**修复建议**：在 store 里接一条：
```ts
Events.On("app-update-busy", () => showToast(t('appUpdate.busyHint'), 'info'));
```
并给 patch 入口加互斥（globalState.appUpdateBusy，在 app-update-start/downloaded/error 三个终态里复位）。

---

### H-12 chip 的 onConfirm 对空 path 静默返回

**位置**：App.vue:359-372 vs App.vue:913-927

**现状**：

```ts
// chip 路径（:359-372）
onConfirm: async () => {
  if (!p.path) return;                 // ← 静默
  try { await App.ApplyAppUpdate(p.path); } catch (e) { showAlert(...); }
}
// 事件路径（:913-927）—— 同一个动作，有明确提示
onConfirm: async () => {
  if (!fullPath) {
    void openModal({ type:'alert', title: t('appUpdate.installPathEmptyTitle'), message: ..., isDanger: true });
    return;
  }
  ...
}
```

**为什么是问题**：p.path 只在 store 收到 app-update-downloaded 时才有值（store.ts:1108）。如果 chip 是因为「下载完成后 store 的 payload 缺 path」而进入 downloaded 态（例如后端 payload 变更、或 store 的兜底分支 store.ts:1110-1119 在 payload 缺 path 时把 path 设为 undefined），用户点「安装」得到的是一次**静默 no-op**。两条路径对同一异常的处理不一致，属于「错误语义被吞」。

**修复建议**：chip 路径复用事件路径的空值分支（抽一个 confirmInstall(path) 供两处调用）。

---

### H-13 runtime-assets-status 订阅时机 vs 后端 emit 时机

**位置**：App.vue:843-845（订阅）、App.vue:756-766（onMounted 首个 await）、后端 app.go:202/218

**现状**：

```ts
onMounted(async () => {
  initStore(); initScrollFades(); uninstallHoverGuard = installHoverGuard();
  try { const behavior: any = await App.GetAppBehavior(); syncThemeFromBehavior(behavior); } catch {}   // ← 第一个 await
  ...
  unsubRuntimeAssetsStatus = Events.On("runtime-assets-status", ({ data }) => { if (data) globalState.assetStatus = data; });
```
```go
// app.go:218 ServiceStartup → runStartupRuntimeAssetMaintenance → maintainRuntimeAssetsAsync
emitEvent("runtime-assets-status", status)   // app.go:202
```

**为什么是问题**：后端在 ServiceStartup（WebView 还没加载页面）就启动了资产自检，并在完成时 emit；前端订阅发生在 onMounted **且**在 await App.GetAppBehavior() 之后。自检是「95 MB 哈希 + clash.exe -v」级别的耗时，所以**通常**能赶上 —— 但一旦赶上缓存热路径（几毫秒完成），事件就永久丢失，globalState.assetStatus 保持 null，设置页/更新页的资产状态只能显示默认值。注释本身也承认「后端在窗口加载前就发出了这条事件」，与「这里接住」自相矛盾。
现成兜底没用：App.GetRuntimeAssetStatus() 存在（app.go:890），前端未调用。

**修复建议**：订阅之后补一次快照拉取：
```ts
unsubRuntimeAssetsStatus = Events.On("runtime-assets-status", ({ data }) => { if (data) globalState.assetStatus = data; });
void App.GetRuntimeAssetStatus().then(s => { if (s && !globalState.assetStatus) globalState.assetStatus = s; }).catch(() => {});
```

---

### H-14 失败路径直接改写后端权威字段 desiredTun / desiredSystemProxy

**位置**：useTunToggle.ts:129-134、Overview.vue:256-265

**现状**：

```ts
// useTunToggle.ts:129-134
} finally {
  globalState.tunPending = false;
  if (next === false) globalState.desiredTun = false;   // ← 前端改写后端权威字段
  settleTunIntent();
}
```
```ts
// Overview.vue:256-265
} finally {
  globalState.systemProxyPending = false;
  if (target === false) {
    if (globalState.portProxyMode) globalState.desiredPortProxy = false;
    else globalState.desiredSystemProxy = false;
  }
  settleSystemProxyIntent();
}
```

**为什么是问题**：后端的控制事务在**硬失败**时会用 CAS 把 desired 回滚到旧值并 SyncState() 广播（control_coordinator.go:183-194），在**瞬时失败**时保留新意图并转后台重试（:175-181）。前端这个 finally 在两种情况下都无条件把 desired 写成 false：
- 硬失败时（例如关闭 TUN 遇到 helper 错误）：后端已回滚成 desired=true 并广播，前端随后把它覆盖为 false → 卡片显示「已关闭」，而系统代理/TUN 实际仍然开着；用户再点一次时 target = !false = true 变成「再次开启」，与用户预期相反。
- 该字段在 store 里被定义成「后端拥有」的镜像（store.ts:172-183 注释），前端写入是双写。

**修复建议**：删掉这两处强行赋值，改为在失败后用一次快照对齐（GetAppState → updateStateFromBackend）；若确实要「失败即显示关闭」，应新增独立的 UI 字段而不是覆写 desired*：
```ts
} catch { ... } finally {
  globalState.tunPending = false;
  settleTunIntent();                 // 只清本地覆盖层
  void App.GetAppState().then(s => s && updateStateFromBackend(s)).catch(() => {});
}
```

---

### H-15 模式切换失败无条件回滚

**位置**：Overview.vue:299-320；后端 controller.go:2225-2271

**现状**：

```ts
const runModeWorker = async (targetMode: string) => {
  modeWorkerActive = true;
  try {
    await App.UpdateClashMode(targetMode);
    const latest = await App.GetAppState();     // ← 这行也可能抛
    updateStateFromBackend(latest);
  } catch (err) {
    globalState.mode = previousMode;            // ← 无条件回滚
    const u = toUserError(err); await showAlert(...);
  } finally { ... }
};
```
```go
// controller.go:2231-2236：先落盘 behavior，再切内核
behavior.ActiveMode = mode
if err := c.Behavior.SetAndSave(behavior); err != nil { c.SyncState(); return err }   // A：未提交
...
if err := clash.UpdateModeWithContext(ctx, mode); err != nil { c.SyncState(); return err }  // B：已提交、内核热切失败
```

**为什么是问题**：两类失败语义不同：A 类（落盘失败）回滚正确；**B 类已经写进 behavior**，后端 SyncState() 广播的正是新 mode，前端却把它覆盖成 previousMode —— 界面显示旧模式、后端是新模式。下一个 app-state-sync（任何状态变化）才纠正，期间用户点「规则/全局」会看到滑块先跳回再跳走。另外 GetAppState() 自身失败也会走到同一个 catch，把「已经成功」的切换显示成失败。

**修复建议**：把「刷新状态」移出 try，失败回滚前先对一次真值：
```ts
try { await App.UpdateClashMode(targetMode); }
catch (err) {
  const latest = await App.GetAppState().catch(() => null);
  if (latest) updateStateFromBackend(latest);      // 以后端为准
  else globalState.mode = previousMode;            // 拿不到才回滚
  ...showAlert
  return;
}
const latest = await App.GetAppState().catch(() => null);
if (latest) updateStateFromBackend(latest);
```

---

### H-16 sliderStyle 在未知 mode 下越界

**位置**：Overview.vue:101-103（模板 :63）

**现状**：

```ts
const sliderStyle = computed(() => ({
  left: ${(modes.value.findIndex(m => m.val === globalState.mode) * 100) / 3}%
}));
```

**为什么是问题**：findIndex 未命中返回 -1 → left: -33.33%，滑块完全移出控件左侧，且三个 seg-item 都不高亮，用户看不到任何选中态。触发条件取决于 AppState.Mode 的取值（默认 rule，见 behavior.go:577），若后端在配置缺省/旧配置升级时给出 "" 或 "script"、"Rule"（大小写不一致）即触发。证据 A（代码路径成立），触发条件 C（需旧配置实测）。

**修复建议**：
```ts
const idx = modes.value.findIndex(m => m.val === globalState.mode);
const left = idx >= 0 ? (idx * 100) / modes.value.length : 0;
```
并在 idx < 0 时不渲染滑块（v-show="idx >= 0"）。

---

### H-17 侧栏明暗开关是「整对象读改写」，与设置页保存互相覆盖

**位置**：Sidebar.vue:70-76 → store.ts:134-145（setThemeMode）、store.ts:140、Settings.vue:204

**现状**：

```ts
// store.ts:134-145
export async function setThemeMode(mode: ThemeMode): Promise<void> {
  applyThemeToApp({ ...themeState.config, mode });      // 乐观生效
  try {
    const current: any = await App.GetAppBehavior();    // 读全量
    if (!current) return;
    current.themeMode = themeState.config.mode;
    await App.SaveAppBehavior(current);                 // 写全量
  } catch (err) { reportError(err, 'theme.saveMode'); }
}
```
```ts
// Settings.vue:204
await App.SaveAppBehavior(behavior.value);              // 同一对象的另一个整对象写者
```

**为什么是问题**：侧栏常驻可见，用户完全可以在设置页（SettingsHome/Behavior/Theme...）里改一项的同时点一下侧栏的明暗行。两条路径都是「Get 全量 → 改 → Save 全量」，交错执行时后写者会把对方刚保存的字段整片回退（典型的 lost update）。前端没有 revision/CAS（SaveAppBehavior 无版本参数），后端也不校验。
此外 setThemeMode 的 catch 只在 console 里留痕（reportError），用户点一下没反应时没有任何提示 —— 与「乐观生效」配合会产生「界面变了、重启后又变回去」的现象。

**修复建议**：短期在 store 里串行化 behavior 写入（一个 behaviorSaveChain: Promise 队列，或 module 级 mutex），并让 setThemeMode 复用设置页的保存入口；长期给 SaveAppBehavior 加 expectedRevision 做 CAS。

---

### H-18 侧栏导航动画：TransitionGroup + v-show = 死代码

**位置**：Sidebar.vue:5-13、Sidebar.vue:206-255

**现状**：

```vue
<TransitionGroup name="nav-slide">
  <div v-for="item in menu" :key="item.id"
       v-show="(item.id !== 'logs' || !globalState.hideLogs) && (item.id !== 'proxies' || globalState.mode !== 'direct')"
       ...>
```
```css
.nav-slide-move { transition: transform .4s ... }
.nav-slide-enter-active { transition: max-height .3s ..., opacity .2s ease-out .1s, transform .2s ... }
.nav-slide-leave-active { ... }
.nav-slide-enter-from, .nav-slide-leave-to { max-height: 0; opacity: 0; ... }
```

**为什么是问题**：TransitionGroup 的 enter/leave 只在**列表项增删**（v-for 数据变化）时触发；v-show 只切换 display，不产生 enter/leave，也不产生 v-move。因此这约 50 行 CSS（max-height 折叠/展开、交错位移）在当前实现下**一次都不会执行**，导航项是瞬切；作者在注释里写的「展开时：高度先撑开」「折叠时：内容先淡出」并没有发生（证据 B：Vue TransitionGroup 语义 + 无 v-if/列表变化点）。
顺带：v-show 隐藏的项仍在 DOM 中（只是 display:none），语义上没问题，但与动效代码自相矛盾。

**修复建议**：要么把 v-show 换成 v-if（或过滤后的列表 computed，让 TransitionGroup 真正接管增删），要么删掉 nav-slide 全套 CSS 与 TransitionGroup 外壳。

---

### H-19 resize → 每个事件一次 IPC，无节流

**位置**：App.vue:671-673、856、680-685

**现状**：

```ts
const handleResize = async () => { isMaximized.value = await Window.IsMaximised(); };
const handleToggleMaximise = async () => {
  clearCtrlHover();
  Window.ToggleMaximise();
  setTimeout(handleResize, 50);        // 未跟踪的定时器
};
window.addEventListener('resize', handleResize);
```

**为什么是问题**：拖拽改变窗口大小时 resize 每秒可达上百次，每次都是一次 Wails IPC（Window.IsMaximised），并产生一个未排序的 Promise；两个响应若乱序返回，isMaximized 可能被更早的请求结果覆盖（按钮图标短暂错误）。另外 setTimeout(handleResize, 50) 是裸定时器（未保存句柄、未在 onUnmounted 清理），50ms 在慢机/动画没结束时可能读不到新状态，图标不更新。

**修复建议**：用 rAF/200ms 节流 + 丢弃过期响应：
```ts
let resizeTick: number | null = null;
const handleResize = () => {
  if (resizeTick !== null) return;
  resizeTick = window.setTimeout(async () => {
    resizeTick = null;
    isMaximized.value = await Window.IsMaximised();
  }, 120);
};
```
handleToggleMaximise 改成 await Window.ToggleMaximise(); await nextTick(); isMaximized.value = await Window.IsMaximised();，并在 onUnmounted 清掉 resizeTick。

---

### H-20 outboundIPStale 只写不读

**位置**：TrafficCard.vue:102-114（展示）；store.ts:275、869-874、896

**现状**：

```ts
const outboundIPText = computed(() => {
  const r = globalState.outboundIP;
  if (!r) return globalState.ipDetecting ? t('traffic.detecting') : t('traffic.undetected');
  if (!r.preferred) return globalState.ipDetecting ? t('traffic.detecting') : t('traffic.failed');
  return r.preferred;                    // ← 不管 stale
});
```
```ts
// store.ts:869-874：检测失败但保留旧成功结果
if (globalState.outboundIP?.preferred) { globalState.outboundIPStale = true; }
```

**为什么是问题**：globalState.outboundIPStale 在 store 里被赋值 4 次、**在前端任何组件里都没有被读取**（全仓 grep 只有 store.ts 命中）。结果是：代理/TUN 切换导致 IP 检测失败时，界面仍然把旧 IP 当作当前出口 IP 展示，用户据此判断「代理生效了」，而实际出口可能已经换了——这是错误信息，不只是缺样式。

**修复建议**：TrafficCard 里消费它：
```ts
const outboundIPText = computed(() => {
  const r = globalState.outboundIP;
  if (!r?.preferred) return ...;
  return globalState.outboundIPStale ? r.preferred + ' (?)' : r.preferred;
});
```
并在 title 里补一行 t('traffic.staleHint')。或者由 store 在 stale 时直接清空 preferred。

---

### H-21 DeviationStrip 忽略 severity、v-for 用 index

**位置**：DeviationStrip.vue:12-27、DeviationStrip.vue:39、模型 clash/models.ts:13-38

**现状**：

```ts
const list = ref<any[]>([]);            // ← 丢掉了 ConfigDeviation 类型
```
```vue
<div v-for="(d, i) in list" :key="i" class="deviation-item">
  <span class="deviation-item__field">{{ fieldText(d) }}</span>
  <span class="deviation-item__reason">{{ reasonText(d) }}</span>
</div>
```
```ts
// backend model：severity = "ignored"（未生效）| "forced"（被强制改值）
```

**为什么是问题**：severity 是后端刻意区分「你的设置被忽略」与「你的设置被改成别的值」的唯一字段，前端完全不渲染，两条语义完全不同的偏差在 UI 上长得一样；key 用下标在列表整体替换时虽然能更新，但配合上面的 open 展开态会丢掉「展开的是第几条」的语义（内容变了位置不变）。属「返回值被忽略」的对接缺口 + 轻微冗余（list 用 any[] 放弃了生成的 ConfigDeviation 类型）。

**修复建议**：const list = ref<ConfigDeviation[]>([])，渲染时加 severity 徽标；key 换成 d.key（后端提供稳定语义 id）。

---

### H-22 Toast 无 hover 暂停 / 无同文案去重

**位置**：Toast.vue:1-23；store.ts:1226-1245

**现状**：

```ts
export function showToast(message: string, kind: ToastKind = 'info', opts = {}): number {
  const id = ++toastSeq;
  const item = { id, kind, title: opts.title ?? '', message, duration: opts.duration ?? 2400 };
  globalState.toasts.push(item);
  while (globalState.toasts.length > 3) globalState.toasts.shift();   // 只限制条数
  window.setTimeout(() => dismissToast(id), item.duration);          // 定时不可暂停
  return id;
}
```
```vue
<div v-for="item in globalState.toasts" :key="item.id" class="toast" ...>   <!-- 无 mouseenter/mouseleave -->
```

**为什么是问题**：① 鼠标悬停时 2.4s 照走，长文案（后端通知可能 100+ 字符）来不及看完就消失；② 没有同文案去重：连续两次同样的失败会并排出现两条一模一样的 toast（store.ts:687-690 的 showAlert 反而做了去重，两套策略不一致）。③ ToastItem.duration 只在 store 里用于定时，组件不消费（死字段，但无害）。

**修复建议**：Toast 根加 @mouseenter/@mouseleave 控制暂停（把 duration 换算成剩余时间重排定时器），并在 showToast 里对「同 kind + 同 message 且仍在展示」的条目直接返回已有 id 并重置其定时。

---

### H-23 日志级别 select 的 aria-label 用了 logs.all

**位置**：App.vue:134

**现状**：

```vue
<select v-model="logLevelFilter" class="log-level-select" :aria-label="t('logs.all')">
```

**为什么是问题**：该下拉是「日志级别」筛选，aria-label 却是「全部」（logs.all 是来源 tab 的文案），读屏用户会听到一个语义错误的控件名；且页面同时存在「全部」来源 tab，两个控件同名。属明确缺陷但影响面小。

**修复建议**：补 i18n logs.levelFilter（8 语言），或至少用 t('settings.behavior.logLevel') 之类的既有键；同时给来源 tab 容器加 role="tablist"（可选）。

---

## 3 冗余对照表

| # | 重复/死代码 | 位置 | 建议 |
|---|---|---|---|
| R-1 | 「流量快照」类型重复声明 4 份（字段逐字相同） | App.vue:379-388、Sidebar.vue:53-62、Overview.vue:82-93、TrafficCard.vue:75-84（本身已定义 TrafficSnapshot） | 抽到 frontend/src/types.ts 的 TrafficSnapshot，四处 import；App.vue 只保留 ref<TrafficSnapshot> |
| R-2 | updateLatestTraffic 被写两遍（同一份数据） | App.vue:819（traffic-data 回调）+ TrafficCard.vue:90-96（watch immediate） | 只保留 App.vue 一处；TrafficCard 的 watch 删除（它还每次新建数组导致 watch 恒触发） |
| R-3 | 更新进度百分比计算实现 2 份 | App.vue:350-354 与 SettingsAbout.vue:369-373 | 抽 utils/format.ts: const updatePercent = (p) => ... |
| R-4 | 日志级别归一化实现 2 份且已分叉 | App.vue:398-405 normalizeLogType vs store.ts:317-323 normalizeLogLevel（后者放行 silent，前者回落 info） | 抽到 utils/loglevel.ts 一份导出，两边引用；顺带修 H-06 |
| R-5 | 级别下拉选项两份、且同一含义用了不同 value | SettingsBehavior.vue:256-262（warn 用 'warning'）vs :264-269（warn 用 'warn'） | 复用一个 logLevelOptions 工厂，value 统一 warn |
| R-6 | 侧栏 icons prop 只为 sun/moon | App.vue:65 + Sidebar.vue:64,31-32 | Sidebar 直接 import { ICONS }，删掉 prop（menu 里已携带图标，等于同一份数据传两遍） |
| R-7 | resetViewScroller 在两个过渡钩子里重复调用 | App.vue:321-329（onPaneBeforeEnter / onPaneSettled 都调） | 保留 before-enter 一处即可；after-enter/after-leave 同一次切页会触发两次 |
| R-8 | DataDir 死兜底分支 | App.vue:589 info?.dataDir ?? info?.DataDir | 生成的模型只有 camelCase dataDir（bindings/.../sys/models.ts:6），删掉 DataDir |
| R-9 | logLevels.warning 死键 | App.vue:396 | 删（见 H-06） |
| R-10 | Sidebar traffic prop 声明 8 个字段只用到 2 个 | Sidebar.vue:53-62（模板只用 traffic.up/down） | 收窄成 { up: string; down: string } 或直接复用 R-1 的类型 |
| R-11 | trafficWaveState 的 refCount 机制只有 1 个调用方 | trafficWaveState.ts:111-151（start 仅 App.vue:826，stop 仅 App.vue:996） | 简化成无引用计数的 start/stop，或改为 TrafficCard 挂载时 start（当前设计下 refCount 永为 1） |
| R-12 | useTunToggle 的 lastError 是死导出 | useTunToggle.ts:38,52-127,137 | 两个消费方（Overview:272、SettingsTun:169）都只取 toggle/pending，lastError 从未被读 → 删掉或在卡片内联展示失效原因 |
| R-13 | 侧栏 nav-slide 动画整套 CSS 未生效 | Sidebar.vue:206-255 | 见 H-18 |
| R-14 | ToastItem.duration 组件不消费 | Toast.vue（无 duration 使用）+ store.ts:223,1237 | 若做 hover 暂停（H-22）则保留，否则删字段 |

---

## 4 已核查且无问题的点（证明覆盖度）

| 核查项 | 结论 | 依据 |
|---|---|---|
| 模态层与 store 队列对接 | **正确**。确认/取消都走 confirmModal/cancelModal，alert 类型隐藏取消按钮；store 里 alert 结算恒 resolve(true)，不会被误判为取消 | App.vue:192-199 + store.ts:569-587,653-677 |
| 组件内自建模态是否绕过队列 | 全局模态（含 app-update 全系列）已全部改走 openModal；唯一例外是 core-seed 弹框（历史问题，见 §6） | App.vue:360,885,913,943,955 |
| Events.On 订阅与 onUnmounted 配对 | **完整**：13 个 unsub 全部调用（含 unsubRuntimeAssetsStatus=null），scrollTimer/logBatchTimer/appUpdateCheckFailsafeTimer 全部 clearTimeout | App.vue:976-1017 |
| 全局 keydown / 快捷键监听 | App.vue **没有**任何 keydown 监听，不存在未清理的全局快捷键；YamlEditor 自己的 document.addEventListener('keydown') 有对应的 remove（不在本批范围，仅 grep 核对） | grep 结果：App.vue 仅 blur/focus/visibilitychange/resize 四类 |
| globalAppUpdatePercent 除零/NULL | **有兜底**：!p || !p.totalBytes || <=0 → 0；formatSpeed 内置 Number.isFinite 与 <=0 判断 | App.vue:350-354 + utils/format.ts:11-14 |
| chip 的 isDownloaded 分支与实际下载完成事件 | 一致：store 在 app-update-downloaded 里把 isDownloaded=true 并补齐 version/path | store.ts:1103-1119 |
| 拖拽区与可交互元素冲突 | **正确**：.drag-bar 设 drag，.top-actions（含 chip 与三个窗口按钮）设 none，命中优先级由最近祖先决定 | App.vue:5-6 + App.vue:28-56 |
| 窗口按钮 hover 状态 | 用 JS ctrlHover 而非 CSS :hover，且在 blur/focus/visibilitychange 三处清理；installHoverGuard 有 uninstall | App.vue:336-339,775-777,976-978 |
| isMaximized 初值 false 是否正确 | **正确**：窗口创建时未最大化（Hidden: isSilent，无 Maximise），从托盘恢复时后端主动 UnMaximise | main.go:371-393 + app.go:116-119 |
| Window.IsMaximised/ToggleMaximise/Minimise API 名 | 与 @wailsio/runtime v3 声明一致（types/window.d.ts:125,267） | node_modules 类型声明 |
| traffic-data 字段名/类型对齐 | 完全一致：后端 json tag up/upRaw/down/downRaw/uploadTotal/downloadTotal/*Raw；upRaw 为 float64，前端 ?? 0 兜底 | streams.go:19-24,268-278 + App.vue:808-820 |
| 波形采样与切页 | 采样器活在模块级单例，切页不会重启/丢失；traffic-stat-mode-changed 会重置；App 卸载时 stop | trafficWaveState.ts:137-169 + App.vue:822-826,996 |
| useTunToggle 两条入口是否互相打架（重点） | **不打架**：唯一实现在同一模块，pending 复用 globalState.tunPending，入口处同步 if (tunPending) return；Overview 与 SettingsTun 互斥挂载（v-if），没有同刻双入口 | useTunToggle.ts:64-70 + Overview.vue:272-276 + SettingsTun.vue:169-180 |
| TUN/系统代理 pending 闸门 | 有（虽然 H-03 是另一个动作）；watchdog 60s 兜底存在于 store | Overview.vue:233, useTunToggle.ts:64, store.ts:1190-1217 |
| GetConfigDeviations 返回 null | 已处理：Array.isArray(data) ? data : []；失败保留上次结果（F-A4-21 修复有效） | DeviationStrip.vue:63-82 |
| DeviationStrip 卸载后改状态 | 有 disposed 双检（await 前后各一次） | DeviationStrip.vue:42,64-74,90 |
| 外网 IP 刷新并发 | store 侧 latest-wins + ipDetecting 合并 + 路由切换时保留旧值；TrafficCard 只调 refreshOutboundIPRouteAware('manual')，无需自行加锁 | store.ts:821-925 + TrafficCard.vue:11 |
| YamlEditor 三对 prop/emit 契约 | 名字、参数顺序、类型完全对齐 | Subscriptions.vue:212,416 + YamlEditor.vue:65-68 + App.vue:104,661-669 |
| api.ts 三命名空间合并是否会静默覆盖方法 | **不会**：108 个导出名做全集比对，无重名 | bindings 三文件导出名对比 |
| data-lang 同时设在 html 与 .app-shell | 有意为之：CSS 同时用 :root[data-lang] 与 .app-shell[data-lang] 两套选择器 | App.vue:2,751-754 + style.css:146-192 |
| 根节点 .dark class 是否死代码 | 不是：tokens.css:109-121 / style.css:234-239 用 html:not([data-theme-mode]) .dark 做主题落地前的兜底 | styles/tokens.css:109-121 |
| Sidebar 菜单 id 与 App.vue 页面映射 | 一致：同一份 menu computed 驱动侧栏与 activeMenuLabel，activeId 双向绑定可用 | App.vue:624-637,61-67 |
| 侧栏隐藏项会不会把用户困在隐藏页 | 当前无路径：能改 hideLogs 的只有设置页、能改 mode 的只有首页的 seg 控件，都不会在目标页上触发 | App.vue:60-65 + Sidebar.vue:7 |
| EmptyState | 无问题：props 全消费、可选节点都 v-if、action 事件有类型、样式类在全局 CSS 存在 | EmptyState.vue:8-27 + style.css:1240-1295 |
| Toast 列表 key | 用自增 id，非 index，正确 | store.ts:1231 + Toast.vue:6 |
| 日志时间列格式化 | ts.slice(5,23).replace('T',' ') 对 RFC3339（含/不含毫秒）都得到可读值，旧条目回落 time | App.vue:506-510 |
| theme watcher 会不会双写 theme_setting.txt | **不会**：后端 SaveThemePreference 只写 txt + 窗口外观，不写 Behavior.ThemeMode；behavior 路径由 SaveAppBehavior 负责 | app.go:588-613 |
| Application.Quit 是否被 WindowClosing hook 拦住 | **不会**：hook 只挂在 RegisterHook(WindowClosing)（WM_CLOSE），Application.Quit() 走 impl.destroy 直接销毁，quit 生效 | wails v3 application.go:1002-1006 + main.go:401-404 |
| handleClose 读取 closeToTray 的正确性 | 逻辑正确（true→隐藏，false→退出），成本与异常见 §6/§7 | App.vue:687-695 |

---

## 5 需要跨文件/后端确认的疑点（移交主控）

| # | 疑点 | 证据位置 | 建议归属 |
|---|---|---|---|
| Q-1 | **Alt+F4 与自定义 X 行为不一致**：WindowClosing hook 无条件 e.Cancel(); Hide()，无论 closeToTray 真假 —— 用户把「关闭时最小化到托盘」关掉后，点 X 会退出，按 Alt+F4 仍只是隐藏。前端无法感知，需要在 hook 里读 behavior | main.go:401-404 + app.go GetAppBehavior | 后端（窗口/托盘） |
| Q-2 | **前端直接 Application.Quit()，绕过后端 safeQuit() 的 8s 强退兜底与 panic 恢复**。若退出过程卡住（内核停不下来），窗口没了进程还在，系统代理可能残留 | App.vue:687-695 vs app.go:1514-1530 | 后端（新增 QuitApp 绑定）+ 前端改用 |
| Q-3 | logLevel = "silent"（UI 提供，SettingsBehavior.vue:261）在后端 LevelRank 落到 default=1（等价 info），「静默」档实际不生效 | core/logger/level.go:5-33 | 日志代理 |
| Q-4 | app-update-available 显式带 manual 字段，前端完全忽略；而 AutoUpdate=true / UpdateMethod="startup" 是默认值，于是**每次启动自动检查后若发现新版本会直接弹确认框**。若产品预期「自动检查只点亮 chip」，这里是唯一开关点 | updater_orchestrator.go:496-499 + behavior.go:579-580 + App.vue:881-902 | 产品确认 → 前端/后端 |
| Q-5 | store.initStore() 是 async，第一个 await App.GetAppState() 之后才注册 app-state-sync/app-notification/app-update-progress/proxy-delay-update 等监听；App.vue 只 initStore() 不 await，启动窗口内这几类事件可能丢 | store.ts:977-993,1100 + App.vue:757 | store 代理（若重复请去重） |
| Q-6 | scrollFade.initScrollFades() 注册的 ResizeObserver + MutationObserver（观察 document.body 的 childList/subtree/class/style）**没有 dispose 接口**，App.vue 无法清理；日志页每 16ms 插入节点的场景会触发 updateAll() 全文档 querySelectorAll | utils/scrollFade.ts:36-65 + App.vue:758 | 工具层/UI 基建代理 |
| Q-7 | SettingsTun 的 TUN 复选框失败回滚靠手写 input.checked = !next，因为 :checked 绑定在 globalState.tun 未变化时不会重 patch。一旦有人把 useTunToggle 的乐观覆盖（setTunIntent）改掉，这个手动回滚就失效 | SettingsTun.vue:174-180 + store.ts:190-203 | UI 基建代理 |
| Q-8 | 日志没有序号（LogEntry 无 seq），前端无法判断「历史快照 + 实时流」是否连续，H-05/H-09 只能做到缓解 | logger/models.ts:4-61 | 日志代理 |
| Q-9 | outboundIPStale 由 store 维护但无人消费（H-20）；若 store 侧决定「stale 时清 preferred」，前端展示逻辑需同步 | store.ts:275,869-874 | store 代理 |
| Q-10 | GetDataDirInfo() 的模型只有 dataDir（camelCase），App.vue 的 DataDir 兜底是死分支；同时「打开日志目录」实际是复制路径（注释自述缺绑定），是否有计划补 OpenLogDir 绑定 | bindings/.../sys/models.ts:4-34 + App.vue:584-596 | 后端 + 前端 |

---

## 6 已知问题（历史已记录，不计入本轮计数）

| 本轮复核 | 历史 ID | 现状（2026-11-18 复核） |
|---|---|---|
| handleClose 里 await App.GetAppBehavior() 无 try/catch，失败即「点 X 没反应」；且每次关闭多一次 IPC | **F-A4-22**（A4:271-276） | **未修复**。App.vue:687-695 仍是裸 await。补充：窗口是 Frameless: true（main.go:378），自定义 X 是唯一「真正退出」的入口，严重度比旧报告更高 |
| 启动链把重调用串在事件订阅之前（GetRecentLogs 无兜底），早期事件整段丢失 | **F-A4-08**（A4:164-167） | **部分未修复**。App.vue:764 仍然先 await App.GetAppBehavior()，808/837 才订阅 traffic/log；GetRecentLogs 仍无兜底（H-09 是其残留的具体后果） |
| 启动早期事件被丢弃、runtime-assets-status 发了没人收 | **F-A3-12** | **仍有竞态**：订阅点被推到 onMounted 且在一个 await 之后（H-13），只是「通常赶得上」 |
| core-seed 对比弹框是自建模态，与全局模态双层遮罩并存 | **F-A4-07**（A4:156-158） | **未修复**。showCoreSeedModal 独立 overlay（App.vue:210-240,345）不走队列；启动时若同时有 app-update-available 模态，两层遮罩叠在一起 |
| clash-exited 里 App.SyncState() 无 catch | **F-A4-22** | 未修复（App.vue:847-850，超出本轮判据，仅记录） |
| 日志面板缺搜索/级别/暂停/复制/清空确认 | **F-A3-16** | 已实现；但滚动跟随（H-04）、级别下拉语义（H-08）、来源枚举（H-07）是这轮新增的具体缺口 |
| showAlert 单槽互相顶掉导致 pending 永久 true | **F-A4-01** | 已修复（store 模态队列 + 60s 看门狗，本轮确认有效） |
| TUN 两套实现 | **F-A4-03** | 已修复（useTunToggle 单一实现，本轮确认两入口不冲突） |

---

## 7 本轮结论摘要

- **操作冲突**（用户最关心）在本范围内的**主路径**是干净的：TUN/系统代理都有共享 pending 闸门，模态已收敛到队列，pending 有 60s 看门狗，useTunToggle 的两条入口不打架。
- 真正剩下的冲突都是**「确认框之前的 await」这一类**（H-03）与**「前端覆写后端权威字段」这一类**（H-14、H-17）：前者导致连点重复执行，后者导致 UI 与后端意图短暂相反。
- 最值得先修的 3 条：**H-03**（连点重复执行后端动作，用户在「重启内核」上能直接感到断流）、**H-01**（每个用户只要点过一次更新 chip，之后每次进设置都错页，可复现率 100%）、**H-02**（一次大流量后波形长期失真，等于图表功能失效且无法自愈）。
