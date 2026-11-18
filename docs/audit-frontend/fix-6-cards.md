# fix-6-cards 修复报告（Overview / TrafficCard / DeviationStrip / Toast / Sidebar / useTunToggle / trafficWaveState）

- 代理：fix-6-cards（独占文件见 FIX_PLAN §分片归属）
- 范围：H-02、H-03（Overview/TrafficCard 侧）、H-14、H-15、H-16、H-17、H-18、H-20、H-21、H-22、S-08、C-19
- 未运行 `vue-tsc` / `vite build` / 应用本体（按约定由主控统一跑）。已用 `@vue/compiler-sfc`（parse + compileScript + compileTemplate）与 TypeScript 语法解析器对 5 个改动的 SFC、2 个改动的 .ts 做语法级校验，全部通过。
- 未改后端 Go、未改 `frontend/src/locales/*`、未加/升依赖、未改行为无关样式。

## 1 改动表

| ID | 状态 | 改动位置(file:line) | 一句话说明 |
|---|---|---|---|
| H-02 | fixed | frontend/src/trafficWaveState.ts:67-73 | `updateScale` 去掉 `target = max(instant, prevScale)` 的棘轮，改为朝 `instant` 收敛（升 `scaleRiseAlpha`／落 `scaleFallAlpha`）并保 `minScale` 下限，回落分支由此可达、波形可自愈 |
| S-08 | partial | frontend/src/trafficWaveState.ts:146-153 | `stopWaveSampling` 的 refCount 改为无条件 `Math.max(0, refCount-1)`，消除多调 stop 造成的永久漂负；「900ms 采样与后端 900ms 推流同频异相」需 App.vue 事件驱动采样，见 handoff-6.md |
| H-03 | partial | frontend/src/components/Overview.vue:201-223、frontend/src/components/TrafficCard.vue:164-185 | 防重入闸门提到 `showConfirm` 之前、`finally` 复位（Overview 用独立 `restartInFlight`，避免确认框期间误显示「重启中」；TrafficCard 用 `resetInFlight`）；App.vue 更新 chip 的互斥归 fix-2 |
| H-14 | fixed | frontend/src/composables/useTunToggle.ts:73,135-143、frontend/src/components/Overview.vue:260-278 | 失败路径不再由前端直写 `desiredTun`/`desiredSystemProxy`/`desiredPortProxy`（那会抵消后端 CAS 回滚），只清本地乐观覆盖层，并在失败后拉一次 `GetAppState` 以后端为准 |
| H-15 | fixed | frontend/src/components/Overview.vue:312-343 | 模式切换失败先 `GetAppState` 对真值，拿不到快照才回滚 `previousMode`；`GetAppState` 自身失败不再被当成「切换失败」 |
| H-16 | fixed | frontend/src/components/Overview.vue:100-108、:63 | 抽出 `modeIndex`，`findIndex` 未命中时 `left` 兜底 0，并用 `v-show="modeIndex >= 0"` 隐藏滑块（未知 mode 不会画出「选中第一项」） |
| C-19 | fixed | frontend/src/components/Overview.vue:100-108、:63 | 同 H-16（C-19 与 H-16 是同一处的两个报告编号） |
| H-17 | skipped | frontend/src/components/Sidebar.vue:70-76（未改） | 组件侧已达标：当前 HEAD 起侧栏开关就是 `void setThemeMode(newTheme)`（store 的唯一入口），没有本地 Get→改→Save；剩余「behavior 整对象写互相覆盖」在 `store.ts setThemeMode`（S-02，fix-1-state），见 handoff-6.md |
| H-18 | fixed | frontend/src/components/Sidebar.vue:5-15、:206-209 | 删掉永不触发的 `nav-slide-*` 整套过渡（约 50 行）与 `<TransitionGroup>` 外壳（v-show 不产生 enter/leave/move；TransitionGroup 本身不产生 DOM，删除不影响结构） |
| H-20 | fixed | frontend/src/components/TrafficCard.vue:21-22、:104-107、:140-143、:255-265 | 新增 `outboundIPIsStale` 消费 `globalState.outboundIPStale`：旧 IP 旁显示「已过期」徽标 + title 追加说明，不再把旧 IP 当当前出口展示（文案待 i18n，见 i18n-needs-6.md） |
| H-21 | fixed | frontend/src/components/DeviationStrip.vue:13、:26-31、:41-42、:62-69、:108-130 | `list` 改用生成的 `ConfigDeviation[]`；渲染后端 `severity`（ignored/forced）徽标做视觉区分；v-for key 从下标换成稳定语义字段 `d.key` |
| H-22 | partial | frontend/src/components/Toast.vue:1-82 | 组件侧实现：① hover 期间到点不移除、移开即消失（等价「hover 暂停」）；② 按 kind+title+message 渲染层去重，连续同文案不再并排堆三条。store 侧「真正暂停定时器 / 插入时去重」需改 store.ts，见 handoff-6.md |

> 另：EmptyState.vue 未分到条目（审计 §4 认定无问题），本 Wave 未改动，diff 为 0。

## 2 关键实现说明（便于评审定位）

1. **H-02**：旧实现 `target = max(instant, prevScale)` 使 `scaleFallAlpha` 数学上不可达（相等时增量恒 0），一次高速流量后 scale 锁死。新实现 `prev + (instant - prev) * alpha`，`alpha` 取 rise/fall，外层 `Math.max(WAVE.minScale, …)` 防跌破下限；采样循环（`pushVisualSamples`）与 `resetWaveState` 未动。
2. **S-08**：旧 `stopWaveSampling` 只在 `sampleTimer !== null` 时把 refCount 归零 → 多调一次停表后 refCount 变 -1 且永久保留。现改为先钳位再加判断，start/stop 严格配对；只剩 App.vue 一处调用方，无行为变化。
3. **H-03**：两个动作都不再「先 await 确认框、后占坑」。Overview 用独立标记而不是 `isRestarting`，因为后者驱动「重启中」文案与转圈，确认框还开着时不该点亮。
4. **H-14/H-15**：都是「以后端快照为准」的收敛——删掉前端对 `desired*` / `mode` 的猜测式写入，失败时补一次 `GetAppState().catch(() => null)`；只有拿不到快照才退回本地回滚，保证离线/后端不可用时 UI 仍可用。
5. **H-21**：`severity` 文案走 `localized('deviation.severity', …)`，与既有 field/reason 同一套「按 code 取 i18n、缺 key 回落原始 code」逻辑，所以 Wave 2 补 key 前后都能显示；徽标样式是本组件 `<style scoped>`（不动全局 style.css，避免与 fix-7 冲突）。
6. **H-22**：store 的 `showToast` 用 `setTimeout` 到点 `dismissToast`，组件无法暂停定时器，因此改为维护一份展示镜像：条目被 store 移除时若正被 hover 就冻结保留，鼠标离开再同步消失；去重在同一次同步里完成（保留最早一条占位，其余仍在 store 里各自到点消失）。副作用：重复文案到来会让可见时长「续期」到后一条的到点时刻，接近审计建议的「复用现有 id 并重置定时」。
7. **H-18**：只删不加——本 Wave 不引入新动效；导航项仍由 `v-show` 控制，DOM 结构与删除前一致（TransitionGroup 不产生包裹元素）。

## 3 回归风险（≤3 条）

1. **波形自适应（H-02）**：scale 现在会缓慢回落（`scaleFallAlpha=0.025` / 900ms）。长时间低流量后突发高速，前几秒高段会被 ratio 上限压平（旧实现是「一次高速后永久压平」，属修掉的 bug）。若观感不可接受，调 `WAVE.scaleRiseAlpha` 即可，不涉及接口。
2. **Toast 展示镜像（H-22）**：`Toast.vue` 不再直接渲染 `globalState.toasts`，改渲染本地镜像。若 Wave 2/其他代理改动 `store.showToast`（例如插入时去重、或新增 `globalState.toasts` 的其他写者），需同步检查本文件；目前 `dismissToast` 语义未变，close 按钮与到点消失都仍走 store。
3. **失败路径多一次 IPC（H-14/H-15）**：切换失败后会额外调用一次 `GetAppState()`（失败已 `catch` 兜底，不会抛）。后端不可用时该调用会静默失败，UI 退回「保持乐观值」，与旧行为一致但不再写 `desired*`。
