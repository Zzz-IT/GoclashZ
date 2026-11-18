# 跨文件移交（来自 fix-6-cards）

> 本文件只列「前端可控的那一半已经做完、剩余部分在我独占文件之外」的项；我没有改过下列任何文件。

## 1 S-08 后半：采样周期与后端推流同频异相（需 App.vue 或后端配合）

- 现状：`trafficWaveState.WAVE.sampleIntervalMs = 900` 是**独立定时器**，后端流量流固定 900ms（`core/clash` `/traffic?interval=900` + `core/appcore/streams.go` ticker 900ms）。两个等周期定时器必然异相 → 「同一份 latestUpload 被采样两次」与「某两次推流被跳过」交替出现，波形上有平台/突跳点。
- I 已做：`stopWaveSampling` 的 refCount 配对（`trafficWaveState.ts:146-153`）；`updateScale` 回落（H-02）。
- 建议（任选其一，都落在我文件之外）：
  1. **推荐**：App.vue 的 `traffic-data` 回调里改调 `pushTrafficSample(upRaw, downRaw)`（由我在 trafficWaveState.ts 新增导出，事件驱动、彻底消除拍频），并删掉 `start/stopWaveSampling` 的定时器；注意 R-2 里 `updateLatestTraffic` 被 App.vue 与 TrafficCard.vue 各写一次的重复也要一并收敛，否则会双推采样。
  2. 低成本兜底（只需改我文件，但属行为微调，未做）：把 `sampleIntervalMs` 改成与 900ms 不整除的值（如 700ms 或 1200ms），只能降低拍频概率、不能消除。
- 影响面：只有 Overview 首页的流量波形，不涉及数据/网络。

## 2 H-22 后半：toast 定时器真正可暂停 + 插入时去重（需 store.ts，属 fix-1-state）

- I 已做（`Toast.vue`）：hover 期间到点**不消失**（冻结展示，鼠标移开立即消失）、渲染层按 kind+title+message 去重。
- 仍缺（store.ts `showToast`）：
  1. 插入时对「同 kind + 同 message 且仍在展示」的条目直接复用 id 并**重置剩余时长**（当前实现是渲染层隐藏重复项，可见时长会顺延到后一条的到点时刻，语义接近但不如重置精确）；
  2. 若要做「精确的 hover 暂停并续跑剩余时长」，只能在 store 侧管理 `setTimeout` 句柄（暂停/恢复），组件无法操作别的模块的定时器。
- 约束：如果 fix-1-state 决定改 `showToast` 的签名/语义（例如新增 `dedupe: boolean`），请同步告知——`Toast.vue` 现在依赖 `dismissToast(id)` 与 `ToastItem{id,kind,title,message,duration}` 不变。

## 3 H-17 后半：behavior 整对象写的 lost update（需 store.ts，属 S-02 / fix-1-state）

- 现状复核：`Sidebar.vue:70-76` **已经**是 `void setThemeMode(newTheme)`（store 唯一入口），组件侧无本地 Get→改→Save，故我把 H-17 标 skipped。
- 真正的问题在 `store.ts setThemeMode`：它自身仍是「`GetAppBehavior()` 取全量 → 改 `themeMode` → `SaveAppBehavior(全量)`」，与 `Settings.vue` 的整对象保存互相覆盖（S-02）。另外 `setThemeMode` 的 catch 只 `reportError`，用户点一下没反应时没有任何反馈——若要做提示，建议在 store 里 `showToast`，我不便从 Sidebar 侧探测失败（`setThemeMode` 返回 `Promise<void>` 且内部吞错）。

## 4 H-03 的 App.vue chip 部分

- 归 fix-2-shell（审计 §2 H-03 建议的 `globalState.updateBusy` 之类互斥）。我只改了 Overview 与 TrafficCard 两处，未碰 App.vue。

## 5 无需后端改动

H-02/H-14/H-15/H-16/H-18/H-20/H-21 都用现有绑定（`GetAppState`、`GetConfigDeviations` 的 `ConfigDeviation{key,severity,module,field,reason}`）与 `globalState.outboundIPStale` 即可完成，本 Wave 不需要新的 Go 接口或 payload 变更。
