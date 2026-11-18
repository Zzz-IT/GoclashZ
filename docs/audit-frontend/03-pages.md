# 03 · 功能页审计（Proxies / Connections / Rules / Subscriptions / YamlEditor）

> 审计角色：前端只读审计代理 pages-feature（task-3）
> 时间：2026-11-18 20:09:52（Asia/Singapore）
> 基线：工作区当前内容（含未提交修改：locales/*、settings/manifest.ts、SettingsTheme.vue）；未修改任何源码
> 判据：① 代码冗余 ② 逻辑错误 ③ 操作冲突 ④ bug ⑤ 前后端对接

## 1. 审计范围与覆盖度

### 1.1 实际读过的文件

| 文件 | 行数(read 计数) | 覆盖 |
|---|---|---|
| `frontend/src/components/Proxies.vue` | 1469 | 全读（1-460 / 460-939 / 939-1469） |
| `frontend/src/components/Connections.vue` | 739 | 全读（1-380 / 380-739） |
| `frontend/src/components/Rules.vue` | 824 | 全读（1-400 / 400-824） |
| `frontend/src/components/Subscriptions.vue` | 902 | 全读（1-400 / 400-699 / 699-902） |
| `frontend/src/components/YamlEditor.vue` | 583 | 全读（1-380 / 380-583） |

> 说明：任务书写的 1312/663/718/819/523 是"非空行数"（`Measure-Object -Line` 不数空行），read 工具计数含空行。

### 1.2 为判断"前后端对接"额外读过的文件（只读，不改）

- 前端：`App.vue`(60-200, 600-730)、`store.ts`(226-326, 440-505, 505-755, 1120-1210)、`api.ts`(全)、`main.ts`(全)、`bindings/goclashz/{appservice,coreservice}.ts`(签名)、`bindings/.../events.d.ts`
- 后端：`app.go`(300-480, 975-1220)、`core/appcore/delay_manager.go`(190-320, 370-545, 545-625, 676-745)、`core/appcore/connections.go`(1-165)、`core/appcore/config_editor_controller.go`(1-40)、`core/appcore/subscriptions.go`(1-160, 258-275)、`core/appcore/proxy_state.go`(100-165)、`core/clash/{config_editor,sub,config,manager,rule_form,rule_overlay}.go`、`core/tasks/manager.go`(40-160)、`core/traffic/monitor.go`(1-60, 195-222)、`core/clash/api_client.go`(430-485, 495-545)、`v3_events.go`

### 1.3 未覆盖部分

- 5 个页面在真实内核运行 / 内核已退出 / 订阅尚未导入三种运行态下的实测（硬性规则禁跑应用）。所有需要运行时区分的结论均已标 B/C。
- `style.css` / `styles/*.css` 全局样式（不在本人范围，仅在页面 scoped CSS 依赖全局变量时引用）。
- 队友负责的 Settings*/Overview/Sidebar/App.vue 主体 → 只引用与本页对接直接相关的行。

### 1.4 证据等级

- **A**：代码本身可直接确证（两端代码都在，语义匹配/不匹配一眼可见）。
- **B**：代码 + 运行时推断（需要事件时序、Wails 调度、内核返回形态等前提，但前提由代码可读出）。
- **C**：假设，待实测/需后端确认（报告中一律显式标注"待实测"）。

---

## 2. 问题总表（34 条：**P0×0**、P1×6、P2×28；按 ID 递增排列）

> 定级说明：本范围**没有找到符合 P0 判据的现行 bug**。最初列出的"规则删除删错条目"（P-01）在核查模态遮罩行为后降级（见 P-01 详情：全屏 `modal-overlay` 会吃掉双击的第二次点击，且当前 UI 下确认期间没有可并发的规则写者）。P5 类"排序不生效"功能完全失效但无数据损坏，按 P1 记；若主控按发布标准另定，可上抬为 P0。

| ID | 严重度 | 类别 | 位置(file:line) | 一句话问题 | 证据 |
|---|---|---|---|---|---|
| P-01 | P2 | 操作冲突（潜在） | Rules.vue:43,480-506 | 删除按 `originalIndex` 定位、确认后不复核：确认期间列表一旦变化就会删掉**另一条**规则（当前 UI 不可达，见详情） | B |
| P-02 | **P1** | 前后端对接/错误处理 | Proxies.vue:676-687,769-780 | 测速全量的失败原因走 `proxy-test-finished` 的 string payload，前端丢弃；`TestAllProxies` 返回 void，catch 是死代码 → 失败仍然全静默 | A |
| P-03 | **P1** | 操作冲突/状态机 | Proxies.vue:474-484,526-588,665-692 | 测速中点节点 → `loadData()` 重建 localGroups 却不清 `frozenSortedNodes` → 网格继续渲染**旧对象**，之后所有 `proxy-test-start` 都打在无人渲染的新对象上：转圈与进度从此不再更新 | A |
| P-04 | **P1** | bug/泄漏 | Connections.vue:277-312,335-353 | `startMonitor` 在两个 await 之间被卸载打断 → 卸载后仍调用 `StartConnectionMonitor()`，后端 1s 轮询空转、`isMonitoring` 与后端状态分叉 | A/B |
| P-05 | **P1** | 功能/操作冲突 | Subscriptions.vue:248-260 | 订阅排序只改内存数组，**后端没有任何持久化 API**；任何一次 `fetchConfigs` 即回退，功能实际不生效 | A |
| P-06 | **P1** | bug/泄漏 | YamlEditor.vue:367-392 | keydown 监听与 theme MutationObserver 在 `await loadConfig()` **之后**注册，卸载竞态下永久泄漏（含 EditorView 与其闭包） | A/B |
| P-07 | **P1** | 数据丢失 | YamlEditor.vue:5,350-356 + App.vue:646-648 | 只有"重新加载"有未保存确认；点返回箭头 → 直接丢改动（同一状态位 `isModified` 就在手里） | A |
| P-08 | P2 | 冗余/设计缺口 | Rules.vue（全文件无 `Events.On`）、Proxies.vue:765-767、Subscriptions.vue:630-635 | `config-changed` 全仓只有一个 emit 点（保存**激活**配置文本），而两个监听者都只在"另一页"挂载 → 实际不可达；规则变更没有页内刷新通道 | A |
| P-09 | P2 | 操作冲突 | Proxies.vue:526-588（调用点 539/607/626/633/766/791） | `loadData` 无 in-flight 互斥、无序号校验，6 个入口可并发，旧响应能覆盖新结果 | B |
| P-10 | P2 | 逻辑错误 | Proxies.vue:479-483,671 | 冻结快照是"已过滤"的结果：测速中清空搜索，不匹配的节点在整个测速期间消失 | A |
| P-11 | P2 | 冗余 | Proxies.vue:168-169,195-196 | `canScrollLeft`/`canScrollRight` 只写不读（边缘渐隐已删） | A |
| P-12 | P2 | bug | Proxies.vue:212-244 | 自绘滚动条未处理 `lostpointercapture`、无 `touch-action:none` → 拖动态可能卡住/触屏抢滚动 | B |
| P-13 | P2 | bug/冗余 | Proxies.vue:266-276,298-304；Connections.vue:340-345；Rules.vue:512-516 | `onActivated/onDeactivated` 在本应用（无 KeepAlive）永为死钩子；配套的 rAF/setTimeout 未在卸载时清理 | A |
| P-14 | P2 | 性能 | Proxies.vue:431-463,443-463 | adaptive 模式每次依赖变化都对全部节点做 canvas measureText（测速期间高频触发） | B |
| P-15 | P2 | 冗余/性能 | Proxies.vue:528-536 | 每次 `loadData` 都拉一次 `GetAppBehavior()`（6 个入口 ⇒ 重复 RPC） | A |
| P-16 | P2 | 错误态缺失 | Proxies.vue:134-136 + app.go:320/336 | 后端把读取失败折成 `groups:{}`，前端只能显示"无分组"，无错误态 | A |
| P-17 | P2 | 操作冲突 | Connections.vue:282-303 | 初始快照 `GetConnections()` 无条件覆盖期间已到达的事件数据（无序号） | B |
| P-18 | P2 | bug | Connections.vue:113 | `selectedConn.chains.join()` 未兜 null，而同文件 356/394 都写了 `|| []` | B |
| P-19 | P2 | 逻辑错误 | Connections.vue:142,246-267 | `filterMode` 从 localStorage 读出后不校验：非法值 ⇒ 无 tab 高亮、滑块 0 宽 | A |
| P-20 | P2 | 操作冲突 | Connections.vue:283,458-480 | 暂停刷新时关闭连接/全部关闭照常执行，但列表不再更新 ⇒ 已关闭的连接仍在卡片里 | A |
| P-21 | P2 | bug | Connections.vue:328-339 | `#title-extra-target` 只在 mount 时探测一次，晚于首帧渲染则该计数永不出现 | B |
| P-22 | P2 | 冗余 | Rules.vue:326-352,508-510 | `loadRules` 双调用（watch immediate + onMounted） | A |
| P-23 | P2 | 操作冲突 | Rules.vue:448-478,119 | 添加规则无入口互斥（`:disabled` 要等重渲染），同帧连点会重复 AddRuleFromForm | B |
| P-24 | P2 | 操作冲突（防御性） | Rules.vue:489-497 | confirm 后重读 `globalState.activeConfigId` 而非捕获值：一旦出现页内可切换配置的入口，就删到新配置的同序号规则（当前 UI 不可达） | C |
| P-25 | P2 | 前后端对接/逻辑 | Subscriptions.vue:347-353,369-389 | `UpdateSub` 恒新建（id=时间戳），同名会得到两条；URL 无校验只判非空 | A |
| P-26 | P2 | 操作冲突 | Subscriptions.vue:266-282（+335/379/403/482/608/620/633） | `fetchConfigs` 4 个入口并发，无序号，旧列表可覆盖新列表 | B |
| P-27 | P2 | bug | Subscriptions.vue:319-328,614-616 + tasks/manager.go:99 | `isUpdating` 只由事件置位，`subs-update-cancelled` 无人监听 ⇒ 按钮可能永久禁用 | B |
| P-28 | P2 | 操作冲突 | Subscriptions.vue:478-491,266-282 | 删除当前激活配置后 `activeConfigId` 短暂悬空，靠后续 `fetchConfigs` 修；该调用失败则一直悬空 | B |
| P-29 | P2 | 冗余 | YamlEditor.vue:85,252-342,56 | `statusText` 写 7 次读 0 次；`globalState` 导入未使用 | A |
| P-30 | P2 | 前后端对接/错误语义 | YamlEditor.vue:307-329 + config_editor_controller.go:30-32 | 后端"保存成功但重载失败"返回 error → 前端显示"保存失败"并保持 dirty | A |
| P-31 | P2 | 逻辑错误 | YamlEditor.vue:250-259 | 每次 docChanged 都 emit `error:false`，校验失败的红色标记被任意一次键入清掉 | A |
| P-32 | P2 | 性能 | YamlEditor.vue:251 | 每次按键 `doc.toString()` 全量比较，大配置文件卡顿 | B |
| P-33 | P2 | 结构性隐患 | YamlEditor.vue:59-63,263 | 无 `watch(() => props.configId)`：一旦父组件复用实例（加 KeepAlive/改 v-show），保存会把旧内容写进新 id（**当前不触发**，C） | C |
| P-34 | P2 | bug | Proxies.vue:152,279,314,318；Connections.vue:142,269；Rules.vue:143,363 | `localStorage` 读写全部未 try/catch；setup 阶段抛错会让整页白屏 | A |

---

## 3. 逐条详情

### P-01（P2，潜在）Rules.vue：删除按 index 定位，确认后不复核

**位置**：`frontend/src/components/Rules.vue:36-47`（卡片与删除按钮）、`:480-506`（handleDelete）、`:421-446`（paginatedRules 生成 originalIndex）
**后端**：`app.go:1113-1179` `DeleteRule(id, section, index)`

现状：

```vue
43  <button class="delete-btn" @click="handleDelete(rule.originalIndex)" :title="deleteBtnTitle">
...
480 const handleDelete = async (idx: number) => {
...
488   const ok = await showConfirm(promptMsg, t('rules.deleteConfirmTitle'), true);
489   if (ok && globalState.activeConfigId) {
490     loading.value = true;
491     try {
497       await App.DeleteRule(globalState.activeConfigId, targetSection, idx);
498       await loadRules();
```

先确认**没有** bug 的部分：section ↔ 数组映射全部正确（"subscription" 指向 `ApplyDeleteOnly(workingRules, overlay.Delete)`，与 `GetRulePageData` 返回的 `SubscriptionRules` 同源，`app.go:1034/1038/1150-1156`；"local" 指向 `ExtractRulesFromRootPublic`，`app.go:1124-1132`），且 `originalIndex` 取的是**全量**数组下标而不是分页下标（`Rules.vue:424-444`）。

为什么是问题（分两层）：

1. **缺陷本身**：`showConfirm` 是异步的，结算后直接用渲染时捕获的 `idx` 删；「取出索引」到「使用索引」之间存在窗口，窗口内列表一变就删错条目。后端对越界索引**静默返回 nil**（`app.go:1125`、`:1152`、`:1166`），前端读不出"没删到"，连错了都没有提示。
2. **可达性核查（因此压到 P2）**：当前 UI 下这个窗口里没有写者——
   - 双击删除按钮**不会**产生两次 `handleDelete`：`.modal-overlay` 是全屏 `fixed`、`z-index: 9999`（`style.css:573-585`），双击的第二次点击被遮罩接手，走 `@click.self` → `handleModalCancel`，等于直接取消掉刚开的确认框；
   - 规则页挂载期间没有后台写者：订阅更新/配置切换都必须先离开规则页（`App.vue` 用 `v-if`，回来时 `onMounted` 会重新 `loadRules`），托盘菜单不含配置切换（`tray_runtime_windows.go:126-134`），core 内也没有定时订阅更新（唯一相关 ticker 是自动测速，`controller.go:2303`）。
   ⇒ 这是**潜在缺陷**而非现行 bug（B）。但它是"删错规则"的直通路径，只要有任何一个将来的入口（页内切配置、后台自动更新订阅、多选批量删除）落进来就会触发，护栏成本很低。

修复建议（三层，建议全做）：

```ts
// 1) handleDelete 入口守卫 + 删除期间互斥（顺带修掉 P-23 的同款问题）
let deleting = false;
const handleDelete = async (idx: number) => {
  if (loading.value || deleting) return;      // 新增
  ...
  if (ok && globalState.activeConfigId) {
    deleting = true;
    try { ... } finally { deleting = false; loading.value = false; }
  }
};
```

```vue
<!-- 2) 模板：至少用 loading 挡住连点 -->
<button class="delete-btn" :disabled="loading" @click="handleDelete(rule.originalIndex)">
```

```ts
// 3) 真正治本：确认后重新拉取，并按"规则文本"定位而不是复用渲染时的 index
const target = currentRulesList.value[idx];
if (!target) return;
await loadRules();
const freshIdx = currentRulesList.value.indexOf(target);
if (freshIdx < 0) { showToast(t('rules.ruleChanged'), 'info'); return; }   // 新 key，需补 8 语言
await App.DeleteRule(globalConfigId, targetSection, freshIdx);   // globalConfigId 见 P-24
```

---
### P-02（P1）Proxies.vue：测速全量的失败通道被整条丢弃

**位置**：`Proxies.vue:665-692`（testAllDelays）、`:769-780`（proxy-test-finished 监听）
**后端**：`core/appcore/delay_manager.go:455-463`（busy）、`:489-491`（finishMsg）、`:501-505`/`:508-512`/`:521-524`（三处"测速启动失败/无有效节点"分支）

现状：

```ts
// Proxies.vue:676-687
try {
  await App.TestAllProxies(nodesArray);
} catch (e) {
  reportError(e, 'proxies.testAll');   // ← 永不可达（见下）
  ...
  showToast(t('proxies.speedTestFailed', ...), 'error');
}
```

```ts
// Proxies.vue:769-780 —— payload 被整个忽略
unsubFinish = Events.On("proxy-test-finished", () => {
  isTesting.value = false;
  frozenSortedNodes.value = null;
  localGroups.value.forEach(g => { ... n.testing = false; });
});
```

后端把所有失败**只**表达在事件参数里：

```go
// delay_manager.go:460-463  批测速未能启动（上一个还占着状态机）
m.emit.Emit("proxy-test-finished", ErrDelayTestBusy.Error())
// :501-505  静默内核保障失败
cleanup, _, err := m.ctrl.EnsureDelayCore(ctx); if err != nil { finishMsg = "测速启动失败：" + err.Error(); return }
// :521-524  没有可测节点
if len(targets) == 0 { finishMsg = "没有可测速的有效节点"; return }
```

为什么是问题：

- `App.TestAllProxies(nodeNames: string[] | null): Promise<void>`（`coreservice.ts:264`，Go 侧 `func (a *App) TestAllProxies([]string)`）**没有 error 返回值**，所以 `catch` 只在传输层故障时进；历史修复 F-A4-10 加的 toast 实际从未执行过。
- 前端把 `proxy-test-finished` 一律当"正常结束"：`isTesting=false` + 清除全部转圈。于是"内核启动失败 / 状态机忙 / 无有效节点"三种失败在界面上**与成功完全同形**：按钮弹回、无提示、无日志。
- 稳定复现路径（B）：点「测速全部」→ 立刻点「停止测速」（`stopTesting` 只调 `StopDelayTest()`，不等待）→ 600ms 内再点「测速全部」。新调用在 `cancelBatchAndWait(600ms)` 超时后命中 `m.state != DelayIdle`，发 `finished("DELAY_TEST_BUSY")`；前端 `isTesting` 刚置 true 就被这条事件复位为 false，一次测速都没发生，全程无提示。
- 另一条（B）：内核未运行时点「测速全部」→ `EnsureDelayCore` 失败 → 同样静默。

修复建议：

```ts
unsubFinish = Events.On("proxy-test-finished", ({ data: reason }: { data?: string }) => {
  const msg = String(reason ?? '');
  isTesting.value = false;
  frozenSortedNodes.value = null;
  localGroups.value.forEach(g => { g.proxies?.forEach((n: any) => { n.testing = false; }); });
  // 后端把成功写成 "测速完成"，其余都是原因文本
  if (msg && msg !== '测速完成') {
    showToast(t('proxies.speedTestFailed', { error: msg }), 'error');
  }
});
```

并同步：`testAllDelays` 里的 `catch` 保留（传输层仍可能失败），但注释要改掉"失败有 toast"的错误前提。**根治需要后端配合**（见 §7 移交项 1）：把 `proxy-test-finished` 拆成 finish/failed，或 payload 改为 `{ok:boolean, reason:string}`。

---

### P-03（P1）Proxies.vue：测速中 loadData 覆盖 → 冻结快照指向旧对象，进度反馈永久失联

**位置**：`Proxies.vue:474-484`（frozen 分支）、`:526-588`（loadData）、`:595-619`（selectNode）、`:665-692`（testAllDelays 冻结）

现状：

```ts
// 474-484
if (isTesting.value && frozenSortedNodes.value) {
  if (searchQuery.value) { ...return frozenSortedNodes.value.filter(...) }
  return frozenSortedNodes.value;          // ← 渲染的是旧对象
}
```
```ts
// 526-588  loadData 每次全量重建（新对象、testing:false）
localGroups.value = processedGroups;
// 595-619  点任意节点都会走一遍 loadData
await App.SelectProxy(groupName, nodeName);
await loadData();
```

为什么是问题：`frozenSortedNodes` 是在测速开始时对 `filteredAndSortedNodes` 拍的**数组快照**，元素是当时那批节点对象；而 `loadData()` 会整体替换 `localGroups`（新对象）。`loadData` 有 **6 个入口**（mount:809、**selectNode:607**、watch(isRunning):626、watch(activeConfigId):633、config-changed:766、proxy-state-sync 未知组:791），它不检查 `isTesting`、也不清 `frozenSortedNodes`；`frozenSortedNodes` 只在 `watch(currentGroup)`(280)、`stopTesting`(653)、`proxy-test-finished`(771) 三处清空。

**最可达的触发路径（A/B）**：组测速进行中，用户点任意一张节点卡 → `selectNode` → `await loadData()` → `localGroups` 换成新对象，而 `filteredAndSortedNodes` 继续返回旧的 `frozenSortedNodes`。此后：
- `proxy-test-start` / `proxy-delay-update` 处理器遍历的是 **localGroups 里的新对象**（:744-750、:756-762），而模板渲染的是 **frozen 里的旧对象** ⇒ 转圈状态与"正在测哪个"从此与界面失联，直到测速结束；
- 延迟数字还能更新，因为那走的是全局 `globalState.proxyDelays`（按 name 取），于是表现成"数字在变、转圈不转"，用户会以为卡死或以为测速没启动；
- 若触发源是配置切换（tray 之外还有 `proxy-state-sync` 未知组，:790-792），旧对象来自**上一个配置**，网格会继续显示不属于当前配置的节点。

修复建议：

```ts
// loadData 赋值前后：拓扑/对象重建时必须作废冻结快照（或直接终止测速 UI 态）
if (isTesting.value) frozenSortedNodes.value = null;
localGroups.value = processedGroups;
```
并且 `frozenSortedNodes` 的清理逻辑抽成 `invalidateFrozen()`，在 `loadData` 里统一调用，避免下次再漏（P-10 的修法也在这里）。

---
### P-04（P1）Connections.vue：startMonitor 的 await 竞态 → 后端监控空转

**位置**：`Connections.vue:277-312`、`:335-353`

现状：

```ts
const startMonitor = async () => {
  if (isMonitoring.value) return;
  isMonitoring.value = true;
  unsubMonitor = Events.On("connections-update", ...);     // 先订阅
  try {
    isLoading.value = true;
    const initData = await App.GetConnections();           // ← 可达 2s（localAPIClient Timeout 2s）
    if (initData && Array.isArray(initData.connections)) connections.value = initData.connections;
    await App.StartConnectionMonitor();                    // ← 卸载后仍会执行
  } catch (e) { reportError(e, 'connections.startMonitor'); }
  finally { isLoading.value = false; }
};
const stopMonitor = () => { if (!isMonitoring.value) return; isMonitoring.value = false;
  if (unsubMonitor) { unsubMonitor(); unsubMonitor = null; }
  App.StopConnectionMonitor(); };
onUnmounted(() => { stopMonitor(); ... });
```

为什么是问题：本应用**没有 KeepAlive**（`App.vue:93-117` 全部 `v-if`），切页即卸载。若用户在 `GetConnections()` 返回前离开（`core/clash/api_client.go:40-42` 的 client Timeout 是 2s，内核忙时会真等到超时），`onUnmounted → stopMonitor()` 时 `App.StartConnectionMonitor()` 还没被调用，后端 `m.cancel` 还是 nil ⇒ Stop 是空操作；随后挂起的 `StartConnectionMonitor()` 落地，后端开始**每秒一次**的 `GetSnapshot()`（`connections.go:120-153`）并 `Emit`，而 JS 侧监听已在 `stopMonitor` 里退订。表现：离开连接页后仍持续 1s/次 HTTP 轮询与事件投递，`isMonitoring=false` 与"后端正在轮询"分叉；下次进页 `Start` 因 `m.cancel != nil` 直接 return（幂等，不报错），所以不会自愈成错误，只是白耗。

修复建议（await 之后复核"我还是不是活着"）：

```ts
const monitorGen = ref(0);
const startMonitor = async () => {
  if (isMonitoring.value) return;
  isMonitoring.value = true;
  const gen = ++monitorGen.value;
  unsubMonitor = Events.On("connections-update", ...);
  try {
    const initData = await App.GetConnections();
    if (gen !== monitorGen.value || !isMonitoring.value) return;   // 已卸载/已停止 → 不再启动
    connections.value = initData.connections;
    await App.StartConnectionMonitor();
  } catch ...
};
const stopMonitor = () => { monitorGen.value++; ... };
```

---

### P-05（P1）Subscriptions.vue：排序功能不落盘（实际不生效）

**位置**：`Subscriptions.vue:248-260`（moveUp/moveDown）、`:266-282`（fetchConfigs）

现状：

```ts
const moveUp = async (index: number) => {
  if (index <= 0) return;
  const items = [...localConfigs.value];
  [items[index - 1], items[index]] = [items[index], items[index - 1]];
  localConfigs.value = items;          // 只改内存，没有任何后端调用
};
```
```ts
const fetchConfigs = async () => {
  configsLoading.value = true;
  try { localConfigs.value = await App.GetLocalConfigs() || []; ... }  // 顺序被后端 index.json 覆盖
```

为什么是问题：

- 全前端无任何"保存订阅顺序"的调用；`coreservice.ts` 里也没有 `*Order*`/`*Reorder*`/`*Move*` 方法（已 grep 确认），`clash.SaveIndex` 写的顺序就是 `SubIndex` 原顺序。**功能完全无效**，只是把当前视图挪一下。
- 更糟的是"排序模式内被自动回退"：`fetchConfigs` 由 4 类入口触发（`config-changed`:630、`subs-update-success`:619、各写操作后、mount），用户在排序模式里调好顺序，只要后台来一条 `config-changed`（例如别处改了配置、或更新订阅完成），顺序**在用户还停留在排序模式时**就跳回原样。
- `moveUp/moveDown` 声明为 `async` 却无 await（小冗余）。

修复建议（二选一，需主控/后端决策）：

1. 后端加 `SaveSubOrder(ids []string)`（写 `SubIndex` 顺序 + `SaveIndex`），前端在"完成"按钮里调用，并在排序模式下暂停自动刷新（`if (isSortingMode.value) return;`）。
2. 短期止血：在 `toggleSortMode` 退出时若顺序有变化，提示"顺序仅本次会话有效"——但更建议直接隐藏排序入口，避免"看起来能用"。

---

### P-06（P1）YamlEditor.vue：卸载竞态导致全局监听/观察者永久泄漏

**位置**：`YamlEditor.vue:367-392`

现状：

```ts
onMounted(async () => {
  await loadConfig();                                   // ← 可能耗时的 IO
  document.addEventListener('keydown', handleKeydown);   // ← 卸载后仍会执行
  themeObserver = new MutationObserver(...);
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
});
onUnmounted(() => {
  editorView.value?.destroy();
  document.removeEventListener('keydown', handleKeydown);
  themeObserver?.disconnect();
});
```

为什么是问题：`onUnmounted` 在 await 期间触发时会先把"还没注册"的东西清理一遍；随后 `onMounted` 的续体继续执行并注册 ⇒ **document 上的 keydown（Ctrl+S）与 documentElement 的 MutationObserver 永久残留**，且都闭包住本组件实例（`editorView`、`props.configId`、`App` 调用），CodeMirror 的 `EditorView` 也会挂在已脱离文档的 `editorContainer` 上不销毁。此外 `loadConfig` 走 `editorContainer.value` 分支新建实例时完全没有"组件还在吗"的判断（`YamlEditor.vue:284-294`）。泄漏的所有者路径：document → keydown listener → handleSave → App.SaveConfigText。触发窗口 = `ReadConfigText` 的耗时（本地磁盘通常 <50ms，但大配置 + 主线程卡顿/冷启动可到数百 ms），所以标 B；代码结构本身是 A 级缺陷（注册在 await 之后、且没有卸载标记）。

修复建议：

```ts
let disposed = false;
onMounted(async () => {
  await loadConfig();
  if (disposed || !editorContainer.value) return;   // 卸载/容器已没了就不再注册
  document.addEventListener('keydown', handleKeydown);
  themeObserver = new MutationObserver(...);
  themeObserver.observe(...);
});
onUnmounted(() => { disposed = true; ... });
```
同时把 `loadConfig` 里"新建 EditorView"的分支改成先判 `disposed` 再 new。

---

### P-07（P1）YamlEditor + App.vue：离开编辑器不检查未保存修改

**位置**：`YamlEditor.vue:5`（`@click="$emit('back')"`）、`:350-356`（只有 reload 有确认）、`App.vue:646-648`（`closeYamlEditor` 直接切 tab）

现状：

```vue
5   <button class="back-btn" @click="$emit('back')" :title="t('common.back')">
```
```ts
// YamlEditor.vue:350-356 —— 只有"重新加载"做了确认
const handleReload = async () => {
  if (isModified.value) { const ok = await showConfirm(t('yamlEditor.reloadConfirmMsg'), t('common.notice')); if (!ok) return; }
  loadConfig();
};
// App.vue:646-648
const closeYamlEditor = () => { currentTab.value = 'subs'; };
```

为什么是问题：`isModified` 与"未保存状态"已经通过 `status-change` 上抛到 App（`yamlEditorModified`），但返回路径两处都没用它：点返回箭头、或侧边栏切到别的页（`App.vue` 的 `watch(currentTab)` 只清理状态，不询问）→ 改动静默丢失。编辑器里改的是用户手工维护的 yaml，丢失不可恢复（无草稿、无落盘 undo）。

修复建议（统一走一个 guard）：

```ts
const handleBack = async () => {
  if (isModified.value) {
    const ok = await showConfirm(t('yamlEditor.backConfirmMsg'), t('common.notice'));
    if (!ok) return;
  }
  emit('back');
};
```
并在 App 的 `watch(currentTab)` 里对 `oldVal === 'yaml-editor' && yamlEditorModified` 做同样拦截（或把当前 tab 回退）。

---

### P-08（P2）Rules.vue：没有任何页内刷新通道；现有 `config-changed` 监听者实际不可达

**位置**：`Rules.vue` 全文件（无 `Events.On` 调用；onMounted:508-510 只 `loadRules()`；watch 只有 activeConfigId:339-352、isRemote:354、ruleTab:362、totalPages:392、searchQuery:318）；相关：`Proxies.vue:765-767`、`Subscriptions.vue:630-635`

为什么是问题（先纠正一个容易误判的事实）：

- `config-changed` 在**全仓只有一个 emit 点**：`core/appcore/config_editor_controller.go:35`，条件是"被保存的配置正是当前激活配置"。`v3_events.go:87` 只有注册。订阅更新（`clash.DownloadSub` 重写 `Subscriptions/<id>.yaml`）**不会**发这个事件。
- 因此现有两个监听者都挂在一个"事件不可能到达"的时刻：`Proxies.vue:765` 与 `Subscriptions.vue:630` 只在各自页面挂载时订阅，而唯一能引发该事件的页面（YamlEditor，`App.vue:103-105`）挂载时它们已被 `v-if` 销毁。两条监听实际是死代码（A）。
- 结论：规则数据没有"变更通知"通道，也没有别处能触发的刷新。`Rules` 目前的正确性完全依赖"页面重新挂载时 `onMounted` 重新拉一次"。只要出现"页内能改同一后端配置"的入口（批量删除、导入规则、订阅后台自动更新、托盘切配置），列表就会陈旧，并顺着 P-01 的索引窗口删错条目。
- 附带的实际问题：`Subscriptions.vue:630-635` 里那段"从事件里同步 `activeConfigId`"的逻辑同样永不执行，是纯冗余。

修复建议（二选一，需与后端一起定）：

1. 让"规则/配置内容已变更"成为一个**页内可用**的通知：后端在 `SaveConfigText` / 订阅更新 / 规则写操作后统一 emit 一个不区分页面的事件（例如 `config-content-changed`，payload 带 id），Rules/Proxies 订阅后各自 `loadRules()`/`loadData()`。
2. 前端短期兜底：在 Rules 的写操作前（P-01 的第 3 层）重新拉一次并对齐索引，这样即使列表陈旧也不会删错。

同时删掉 `Proxies.vue:765-767` 与 `Subscriptions.vue:630-635` 这两段不可达的监听，或按方案 1 改造。

---
### P-09（P2）Proxies.vue：loadData 无并发/序号保护

**位置**：`Proxies.vue:526-588`，调用点 539/607/626/633/766/791。
**现状**：`loadData` 里两次 await（`GetAppBehavior` → `GetInitialData`，后者是两次串行 HTTP，各 2s 上限），无 in-flight 标记也无 `seq`。
**为什么**：同一时刻最多可出现 2-3 个并发 `loadData`（例如托盘切配置时 `activeConfigId` watch + `config-changed` + `proxy-state-sync` 未知组）。先发起的请求如果后返回，会把较旧的全量数据写回 `localGroups`，构成闪回。部分症状被"下一次事件会再拉一次"掩盖（B），但 `selectNode`（用户动作直连）那条路径不会自愈到用户点的那次。
**建议**：

```ts
let loadSeq = 0;
const loadData = async () => {
  const seq = ++loadSeq;
  ...
  const data = await App.GetInitialData();
  if (seq !== loadSeq) return;          // 已有更新的一次在飞
  localGroups.value = processedGroups;
};
```

### P-10（P2）Proxies.vue：冻结快照含搜索过滤结果

**位置**：`Proxies.vue:479-483`、`:671`
**现状**：`frozenSortedNodes.value = [...filteredAndSortedNodes.value]` 记录的是**已应用 searchQuery 过滤**的数组；冻结分支只是在这个子集上再次过滤（`filter(n => n.name.includes(q))`）。
**为什么**：在搜索状态下点「测速全部」，测速期间清空搜索 → `if (searchQuery.value)` 为假，直接返回冻结子集 ⇒ 不匹配的节点在整个测速期间（最长 45s）从网格里消失。
**建议**：冻结不含过滤的快照（`[...activeGroupData.proxies]` 排序后的结果），过滤永远在冻结分支里做。

### P-11（P2）Proxies.vue：canScrollLeft/Right 死状态

**位置**：`Proxies.vue:168-169`（声明）、`:195-196`（唯一写入）。模板无任何引用（grep 确认），注释 `:864` 也写明"不再需要边缘渐隐遮罩"。
**建议**：删除这两个 ref 与 `updateTrackOverflow` 里的两行赋值（保留 `updateRailThumb()`）。

### P-12（P2）Proxies.vue：自绘滚动条的指针捕获边界

**位置**：`Proxies.vue:218-244`；CSS `:892-904`
**现状**：`pointerdown` 里 `setPointerCapture`，`pointerup`/`pointercancel` 复位，但**没有 `lostpointercapture` 处理**；`.proxy-rail-thumb` 也没有 `touch-action: none`。
**为什么**：（a）捕获因元素被 `v-show=false`（`railThumbVisible` 在滚动中可能翻转）或其它原因丢失时，`thumbDragging` 停在 true，`.is-dragging` 高亮常驻；（b）触屏/触控板手势会在第一次 pointermove 前被浏览器当作滚动接管，拖动条不跟手。
**建议**：加 `@lostpointercapture="onThumbPointerUp"`；CSS 加 `touch-action: none;`（仅拖动条）。

### P-13（P2）Proxies/Connections/Rules：onActivated/onDeactivated 是死钩子；配套定时器未清理

**位置**：`Proxies.vue:298-304`、`Connections.vue:340-345`、`Rules.vue:512-516`；`Proxies.vue:271-274`（resetSlider 里的 requestAnimationFrame）、`:300`（onActivated 里的 setTimeout 50ms）
**为什么**：`App.vue` 的页面全部是 `v-if` 分支（`:93-172`），**全仓无 `<KeepAlive>`**（grep 确认），Vue 只在 KeepAlive 语境下派发 activated/deactivated ⇒ 这 6 个钩子永不执行。于是注释"切回连接页时恢复更新"、"Give browser time to restore DOM"都是错的；同时 rAF 与 setTimeout 没有任何取消逻辑，卸载后仍会写 `sliderVisible`/`sliderReady`。
**建议**：删掉 6 个死钩子（Connections 的 start/stop 已由 mounted/unmounted 正确覆盖）；`resetSlider` 里的 rAF 记下 id 并在 `onUnmounted` 里 `cancelAnimationFrame`。

### P-14（P2）Proxies.vue：adaptive 模式每次 patch 全量 measureText

**位置**：`Proxies.vue:431-463`（`measureNodeTextWidth` + `adaptiveCardWidth`）、`:465-472`
**为什么**：`adaptiveCardWidth` 依赖 `filteredAndSortedNodes`，后者依赖 `globalState.proxyDelays`；测速期间每个节点一条事件 ⇒ 每帧都可能重算，每次对全部节点做 2 次 `measureText`（每次还重设 `font` 字符串）。节点多（>200）时测速期间明显掉帧（B）。
**建议**：按 `node.name`/`node.type` 做 `Map<string, number>` 缓存（文本不变就不重测）；或把 `adaptiveCardWidth` 的依赖从"含排序/延迟的列表"改为只依赖 `activeGroupData.proxies`。

### P-15（P2）Proxies.vue：每次 loadData 都拉 GetAppBehavior

**位置**：`Proxies.vue:528-536`
**为什么**：`isColorMode` 只需在挂载时或行为设置变更时更新，却被放进 `loadData`，于是 6 个入口（含 config-changed、每次选节点）都多一次 RPC；选节点是高频操作。
**建议**：把 `GetAppBehavior` 移到 `onMounted`，或监听行为变更（`store.ts` 已有 `syncThemeFromBehavior` 入口）。

### P-16（P2）Proxies.vue：无错误态，后端把错误折成空 groups

**位置**：`Proxies.vue:539-540,585-587,134-136`；`app.go:317-347`
**现状**：后端 `GetInitialData` 在 `GetOfflineData` / `clash.GetInitialData()` 失败时返回 `{"groups": {}}`（`app.go:320-325, 336-341`）；前端 `if (data && data.groups)` 为真、keys 为空 ⇒ `localGroups=[]` ⇒ 显示"暂无分组"空态，`catch`/`reportError` 都不会触发。
**为什么**：用户无法区分"真的没有分组"与"读取失败"（配置损坏、内核 API 不通）。
**建议**：请后端在失败分支带 `error` 字段（见 §7 移交项 7），前端据此给出错误态；短期可在 `processedGroups.length === 0 && data.mode` 时提示"未取到代理组"。

### P-17（P2）Connections.vue：初始快照可覆盖新事件

**位置**：`Connections.vue:282-305`
**为什么**：先注册 `connections-update`（每秒 tick），再 `await App.GetConnections()` 并**无条件** `connections.value = initData.connections`。若快照读取期间有新事件到达（后端 1s tick，读取最长 2s），事件写入的新数据会被更快照覆盖一帧（B）。
**建议**：与 P-09 同款序号：事件处理器递增 `snapSeq`，快照返回后 `if (seq !== snapSeq) return;`。

### P-18（P2）Connections.vue：chains 未兜 null

**位置**：`Connections.vue:113` vs `:355-358`（`isDirect`）、`:393-398`（`getProxyName`）都写了 `conn.chains || []`
**为什么**：`ConnectionDTO.Chains []string`（`core/appcore/connections.go:32`）直接来自内核 JSON，nil 会序列化成 `null`。同文件两处兜底说明作者已知风险，模板是唯一漏网处；命中时渲染抛 TypeError，整个详情页白屏（B：取决于内核是否省略该字段）。
**建议**：模板改 `(selectedConn.chains || []).join(' ➔ ')`，或后端 DTO 侧保证非 nil。

### P-19（P2）Connections.vue：filterMode 未校验

**位置**：`Connections.vue:142`、`:11-14`（:ref 回调按三个值匹配）、`:246-267`
**为什么**：值非法时三个 `connTabEl` 赋值都不成立 ⇒ `connTabEl=null` ⇒ `updateConnSlider` 早退、滑块宽 0；`resetConnSlider` 仍把 `connSliderVisible=true` ⇒ 出现 0 宽高亮块且无 tab 是 active。
**建议**：`const saved = localStorage.getItem('goclashz_connFilter'); filterMode = ['all','proxy','direct'].includes(saved) ? saved : 'all';`

### P-20（P2）Connections.vue：暂停态下关闭连接，列表不刷新

**位置**：`Connections.vue:282-284`（`if (isPaused.value) return;`）、`:458-480`
**为什么**："暂停刷新"只丢弃事件，不停止后端监控；关闭全部/单个后没有主动刷新，列表保留已关闭的卡片（直到用户点"继续刷新"），用户会认为关闭失败而再点一次。
**建议**：关闭成功后本地删除该 id（`connections.value = connections.value.filter(c => c.id !== id)`）或把 `isPaused` 复位。

### P-21（P2）Connections.vue：title 计数目标只探测一次

**位置**：`Connections.vue:328-339`（`titleTargetReady` / `refreshTitleTarget`）；目标元素 `App.vue:74`
**为什么**：判定只发生在 mounted 的 `nextTick`，若此刻 `#title-extra-target` 尚未在文档中（切页动画期间被替换），`titleTargetReady` 停在 false 且无重试 ⇒ 计数徽标永久不显示（B/C）。
**建议**：在 `watch(connections.length)` 里懒探测一次，或用 `MutationObserver` 等目标出现。

### P-22（P2）Rules.vue：loadRules 双调用

**位置**：`Rules.vue:339-352`（`watch(..., {immediate:true})`）+ `:508-510`（`onMounted(loadRules)`）
**为什么**：`activeConfigId` 多数情况下启动时已由 `store.ts:71` 的 localStorage 预热写入非空值 ⇒ 挂载时发两次 `GetRulePageData`（并发、无序号，与 P-09 同类）；`loading` 只反映后完成的那次。
**建议**：删掉 `onMounted` 里的 `loadRules()`（watch immediate 已覆盖）。

### P-23（P2）Rules.vue：添加规则无入口互斥

**位置**：`Rules.vue:448-478`（`handleAddFromForm`）、`:119`（提交按钮 `:disabled="!canSubmitRule || loading"`）、`:99`（输入框 `@keyup.enter="handleAddFromForm"`）
**为什么**：函数入口没有 `if (loading.value) return;`；`:disabled` 要等一次重渲染才生效，双击或"回车 + 点击"同帧触发两次时会发两次 `AddRuleFromForm`，产生两条相同规则（B）。
**建议**：入口第一行加守卫，并在函数体最前面同步置 `loading`。

### P-24（P2，C）Rules.vue：确认后重读可变全局配置 id

**位置**：`Rules.vue:489-497`

现状：

```ts
489   if (ok && globalState.activeConfigId) {
491     try {
497       await App.DeleteRule(globalState.activeConfigId, targetSection, idx);
```

为什么是问题：`showConfirm` 是异步的，结算后**重新读** `globalState.activeConfigId`，而不是"用户按下删除时看到的那份配置"。若确认期间激活配置变了，`DeleteRule` 会作用到新配置的同序号规则上。

可达性（因此标 P2 + C）：当前 UI 里切换激活配置只能从订阅页发起（`Subscriptions.vue:286-317`），而订阅页与规则页互斥（`App.vue` 的 `v-if` 一次性只挂载一个），托盘菜单也没有配置切换项；所以现在**不可达**，属于防御性缺陷。一旦将来出现"页内切配置"或"后台切配置"（例如订阅更新后自动切换），它就是删错规则的入口。

修复建议：进入操作时捕获配置 id 并在确认后校验。

```ts
const configId = globalState.activeConfigId;      // 捕获
...
if (ok && configId) {
  if (configId !== globalState.activeConfigId) { showToast(t('rules.configSwitched'), 'info'); return; }   // 新 key，需补 8 语言
  await App.DeleteRule(configId, targetSection, idx);
}
```

---
### P-25（P2）Subscriptions.vue：导入恒为新建；URL 无校验

**位置**：`Subscriptions.vue:347-353`（handleDownload 只判空）、`:369-389`（confirmImport → `App.UpdateSub(finalName, pendingImportPath)`）
**后端**：`app.go:445-447` → `subscriptions.go:33-49` → `clash.DownloadSub(ctx, name, url, "", ua)`；`core/clash/sub.go:51-55`：`id == ""` 时用 `time.Now().UnixMilli()` ⇒ **永远新建**。
**为什么**：`UpdateSub` 的语义是"按名字新建一个订阅"，不是"覆盖同名"（已与后端核对，见 §5）。因此 (a) 同名导入会产生两条同名订阅（id 不同），UI 无法区分；(b) `t('subs.newSubDefaultName')` 是固定默认名，连着导两个远程订阅就是两个同名；(c) URL 只判非空，非法串要等下载失败才报错（错误文案是"订阅配置校验失败"，不是"URL 不合法"）。
**建议**：导入前用 `new URL()` 校验 `http/https`；提交前检查 `localConfigs` 是否已有同名并要求改名（或明确提示"将新增第二条同名订阅"）。

### P-26（P2）Subscriptions.vue：fetchConfigs 多入口并发无序号

**位置**：`Subscriptions.vue:266-282`；调用点 335、379、403、482、608、620、633
**为什么**：每次写操作后、每个事件后都发一次（内部两个 RPC），无 in-flight 合并，旧响应覆盖新列表的窗口存在（B）。
**建议**：`let seq = 0` + 仅最后到达者赋值；或做 `pendingRefresh` 合并（有刷新在飞就置脏，结束后再刷一次）。

### P-27（P2）Subscriptions.vue：isUpdating 缺 cancelled 兜底

**位置**：`Subscriptions.vue:23-26`（`:disabled="isUpdating"`）、`:319-328`（handleUpdateAll 不置位）、`:614-616`（只监听 start）
**后端**：`core/tasks/manager.go:91-110` 三种结局分别 emit `-start` / `-error` / `-success`；被取消时 emit `name+"-cancelled"`（`:99`），**前端无人监听**（`v3_events.go:173-175` 也只注册三个）。
**为什么**：只要出现"已收到 start、任务随后被取消"，`isUpdating` 永久 true ⇒ 按钮永久禁用、图标永久旋转，直到切页重建组件。触发条件窄（需要同帧两次 `UpdateAllSubsAsync`，因为 `Run` 会 cancel 旧任务），标 P2/B。
**建议**：注册 `Events.On('subs-update-cancelled', () => { isUpdating.value = false; })`；并让 `handleUpdateAll` 在调用前同步置 true（不依赖事件往返）。

### P-28（P2）Subscriptions.vue：删除激活配置后 activeConfigId 悬空

**位置**：`Subscriptions.vue:478-491`、`:266-282`
**为什么**：`DeleteConfig` 成功后没直接改 `globalState.activeConfigId`，靠 `fetchConfigs` 的 `GetInitialData` 回填（`:271-276`）。若那次 `GetInitialData` 失败（catch 只 reportError），`activeConfigId` 会一直指向已删除的 id：Rules 继续对该 id 请求，Proxies 拿到空组。后端已保证清空 `Behavior.ActiveConfig`（`subscriptions.go:123-160`），所以只是前端时序问题（B）。
**建议**：删除成功后若 `globalState.activeConfigId === targetId.value`，先本地清空三个 active* 字段，再 `fetchConfigs`。

### P-29（P2）YamlEditor.vue：statusText 死状态 + globalState 死导入

**位置**：`YamlEditor.vue:56`（`import { globalState, showConfirm, showAlert }`，`globalState` 全文未用）、`:85`（`statusText`）、赋值点 252/270/297/310/316/322/337/342
**为什么**：模板（1-41）只渲染 `configName / fileName / totalLines / totalChars`；`statusText` 的 8 次赋值没有任何读者（`emit('status-change')` 用的是各调用点临时拼的字符串）。状态栏实际显示 App 的 `yamlEditorStatus`，所以问题只是纯冗余 + 误导。
**建议**：删 `statusText` 与 `globalState` 导入；把 `emit('status-change', ...)` 收敛成 `setStatus()` 一个函数。

### P-30（P2）YamlEditor.vue：把"保存成功但重载失败"当保存失败

**位置**：`YamlEditor.vue:311-325`；后端 `core/appcore/config_editor_controller.go:24-33`：

```go
if err := clash.SaveConfigText(id, content); err != nil { return err }   // 已写完盘
c.events.Emit("config-file-saved", ...)
if state.ActiveConfig == id {
    if state.IsRunning || state.SystemProxy || state.Tun {
        if err := c.controls.ReloadConfig(ctx, "config-save"); err != nil {
            return fmt.Errorf("保存成功，但重载运行配置失败: %w", err)   // ← error 但文件已保存
        }
    }
}
```

**为什么**：前端 `catch` → `isModified` 保持 true、`showAlert(t('yamlEditor.saveFileError'))`。用户理解成"没保存"，实际已保存；随后重复保存/重新编辑，容易造成"我以为回滚了，其实没有"。证据 A（两端语义明确），触发条件 B（端口冲突、provider 下载失败、TUN 初始化失败等）。
**建议**：需要后端结构化结果（§7 移交项 2）；短期在 catch 里按文案区分，明确写"文件已保存，仅运行配置重载失败"，并把 `isModified` 置 false。

### P-31（P2）YamlEditor.vue：校验错误标记被下一次键入清掉

**位置**：`YamlEditor.vue:245-260`（updateListener 固定 `error:false`）
**为什么**：校验失败 → App 头部显示红色错误；键入任意字符 → `emit('status-change', {error:false})` ⇒ 红标消失但内容仍非法（`yamlError` 也没清）。状态机不一致。
**建议**：`error: !!yamlError.value`（保留到下一次校验/保存），或在 docChanged 时清 `yamlError` 并同步 emit。

### P-32（P2）YamlEditor.vue：每次按键全量 toString 比较

**位置**：`YamlEditor.vue:250-251`
**为什么**：`update.state.doc.toString() !== originalContent.value` 在每次文档变更（含 IME、粘贴大段）都做 O(n) 字符串构建；1 MB 级配置在低端机上可感知（B）。
**建议**：docChanged 时直接置 `isModified=true`，只在 `loadConfig`/`handleSave` 复位；需要"改回原样"精确性时在失焦/保存时再比较一次。

### P-33（P2，C）YamlEditor.vue：无 watch(props.configId)

**位置**：`YamlEditor.vue:59-63`、`:263-305`（loadConfig 只在 onMounted 调）
**为什么**：当前 `App.vue:103-105` 用 `v-if="currentTab === 'yaml-editor'"` 包住组件、切页即销毁，props 在实例生命周期内不会变，**现在不是活 bug**。但这是结构性隐患：一旦改成 `v-show` 或加 `<KeepAlive>`，`SaveConfigText(props.configId, 旧内容)` 会把 A 配置内容写进 B 配置。标 C（假设，需实测才有实际影响面）。
**建议**：加 `watch(() => props.configId, () => loadConfig())`（dirty 时先确认），成本 3 行，防住未来重构。

### P-34（P2）四页：localStorage 全部未 try/catch

**位置**：`Proxies.vue:152,279,314,318`、`Connections.vue:142,269`、`Rules.vue:143,363`
**为什么**：`currentGroup`/`sortState`/`filterMode`/`ruleTab` 的读取都在 setup 顶层，一旦 localStorage 不可用（WebView2 隐私/损坏配置、配额异常），异常直接抛在组件 setup ⇒ 整页白屏；写入点（watch）抛错则只在控制台留痕。证据 A（结构），实际概率低。
**建议**：抽 `utils/storage.ts`（`safeGet`/`safeSet` 内部 try/catch），四处替换。

---

## 4. 冗余对照表

| # | 重复内容 | 出现位置 | 建议 |
|---|---|---|---|
| R-1 | **选项卡滑块（DOM 量取 + 首次动画抑制）**：`tabsTrackRef/xxxTabEl/sliderStyle/sliderReady/sliderVisible/updateXxxSlider/resetXxxSlider` + `:ref` 回调模板 | Proxies 157-161,250-276；Connections 240-267；Rules 146-173；App.vue(logs) 126 + logsSlider* | 抽 `composables/useTabSlider.ts`（4 份逐行同构，仅前缀不同） |
| R-2 | **选项卡按钮 CSS**（`.xxx-tab-btn` / `.active` / `.xxx-tab-slider` + `.animated`，含同样的 transition / height 36px / padding 0 16px） | Proxies 949-1000；Connections 551-592；Rules 555-596 | 合并到 `styles/surfaces.css` 一组 `.ui-tab-*` |
| R-3 | **"加载-错误-try/finally"三件套**（loading=true → await → catch reportError/showAlert → finally false） | Proxies 665-692；Connections 297-311；Rules 326-337/448-478/480-506；Subscriptions 369-389/398-412/478-491；YamlEditor 307-329 | 抽 `composables/useAsyncAction.ts`，顺带统一"入口互斥"，天然修掉 P-01/P-23 |
| R-4 | **错误提示模板**：`const u = toUserError(e); await showAlert(t(key,{error: displayErrorMessage(e)}), t('common.error'), true, u.detail)` | Rules 248-249/473-474/500-501；Subscriptions 341/385/408/487；YamlEditor 301/325 | 抽 `alertError(e, key)`（store 或 utils/errors） |
| R-5 | **错误字符串化两套写法**：`String(e)` 与 `toUserError(e).message` 混用 | `String(e)`：Proxies 617、Connections 465/478、Subscriptions 315/467；同文件其它处用 `toUserError` | 统一走 `toUserError`（F-A4-13/14 同类残留，不计新问题） |
| R-6 | **空态/加载态 4 套实现** | Proxies 134-136 + 1458-1468（手写 div/CSS）；Connections 30-32（手写 loading）+ 695(`.empty-state`) 与 EmptyState 组件并存；Rules 49-58(EmptyState) + 704 `.loading-state`（死 CSS，无引用）；Subscriptions 33-46（手写 loading + EmptyState） | 统一用 `EmptyState.vue` 并加 `variant="loading"`；删 Rules 的死 `.loading-state` |
| R-7 | **Proxies 死代码/死 CSS**：`canScrollLeft/Right`(168-169,195-196)、`::-webkit-scrollbar-button/track/thumb`(932-947，因 886-889 已 `display:none` 永不生效)、`.size-standard` 与基础 `.node-grid` 完全重复(1138-1140 vs 1157-1160)、节点对象里的 `now` 字段(563，模板从未读)、两个 `onMounted`(289/740) | 同左 | 删；`node.now` 若为兼容保留请加注释 |
| R-8 | **YamlEditor**：`statusText` 死状态(85 + 8 处赋值)、`globalState` 死导入(56)、`getTheme/getHighlight` 两层薄包装(224-225) | 同左 | 删；用 `setStatus()` 统一 emit |
| R-9 | **注释与实现矛盾**（比死代码更容易误导后来者）：Proxies 836-841/876-878 说"轨道高 = 按钮 36 + 滚动条 4px"，但 878 行已 `scrollbar-width: none`；864 行"原生滚动条已经回到轨道内"同矛盾；Connections 327"配合 KeepAlive 的生命周期控制"（无 KeepAlive） | Proxies 836-841,864,876-878；Connections 327 | 改注释或删 |

---

## 5. 已核查且**无**问题的点（覆盖度证明）

**Proxies.vue**

- 单点测速与批量测速的并发：后端是两条独立车道（`delay_manager.go:192-193` 的 `sem`(10) 与 `singleSem`(5)），单点同节点去重在 `beginSingleNode`（:410-420），所以"批量测速中点单节点"在**后端**不会互锁（UI 侧的冻结快照副作用见 P-03）；`TestProxy` 返回的 `DELAY_TEST_BUSY` 与前端匹配的字符串一致（`ErrDelayTestBusy = fmt.Errorf("DELAY_TEST_BUSY")`，`delay_manager.go:15` ↔ `Proxies.vue:705`）——F-A4-10 的"泛化 busy"确实已修好。
- `TestProxy` 自己会 `emitDelayResult`（`delay_manager.go:730`），所以前端忽略其返回的 `number` 不会导致延迟不显示；延迟数据由 store 的全局监听入库（`store.ts:1147-1174`），与 Proxies 是否挂载无关。
- `toggleTestAllDelays` 的幂等：`isTesting` 在任何 await 之前同步置位（:668），双击不会发两次 `TestAllProxies`。
- `stopTesting` 的 `finally` 会清 `isTesting/frozen/nodes.testing`（:652-661），`App.StopDelayTest()` 返回 void 不会抛；`proxy-test-finished` 再兜底清一遍（:774-779），双保险成立。
- `Events.On` 的 5 个订阅在 `onUnmounted` 全部 `?.()` 退订（:812-823），无监听泄漏；`ResizeObserver` 也 `disconnect` 了（:813）。
- `formatDelay`/`getDelayColorClass` 对 `delay===null`、`status!=='success'`、`delay===0` 的处理与后端 `DelayResult`（失败恒 `Delay:0`，:349-385）语义一致，无 `NaN`。
- 分组名含特殊字符：`:key="group.name"` + `find(g => g.name === ...)` 精确比较，无选择器注入；localStorage 里的旧组名丢失时被 `isCurrentValid`（:580-583）自动纠正。
- `isManuallySelectableGroup`（Selector/Fallback）与后端 `SelectProxyWithModeSync` 的可选组语义一致；URLTest/LoadBalance 卡片被标 readonly 且 `selectNode` 二次拦截（:595-600），不会打到后端。
- `proxy-state-sync` 的 payload 字段（`name`/`now`，`proxy_state.go:16-19`）与前端读取一致；后端只在哈希变化时发（:120-125），不会造成轮询风暴。
- `gridStyle` 内联样式仅在 adaptive 模式返回，其余模式返回空对象，不会与 `.size-*` 冲突。

**Connections.vue**

- `connections-update` 的 payload（`{connections:[...]}`，`ConnectionsSnapshot`，`connections.go:40-42`）与前端 `data.connections` 一致；后端 `make([]ConnectionDTO,0,n)` 保证是数组而非 null。
- 事件订阅与 `App.StopConnectionMonitor()` 在 `onUnmounted` 成对（:346-353）；`startMonitor` 有 `isMonitoring` 幂等位，Go 侧 `Start` 也幂等（`connections.go:107-112`）。
- 列表 `:key="conn.id"`（:35）而不是 index：连接数变化时不会错位；详情页 `selectedConn` 也按 id 跟随（:287-291），不存在"展开状态串行"。
- `isDirect`（chains 含 DIRECT/REJECT）与后端 chains 顺序（内核返回"从外到内"）一致，过滤语义正确。
- 关闭单个连接用 `selectedConn.id`（:120）而非 index，无错行风险；`showConfirm` 走 store 队列（`store.ts:696-703`），不会互相顶掉。
- DNS 面板的 `requestConnId` 守卫（:431,445,449,452）正确处理了"切到别的连接后旧响应回来"的陈旧响应。
- `EmptyState` + 220ms 延迟（:368-391）避免"卡片消失动画与空态同帧"；`emptyStateTimer` 在 `onUnmounted` 清理。
- `.dark .tag-proxy` 不是死 CSS：`isDark()` 用 `document.documentElement.classList.contains('dark')`（`YamlEditor.vue:222`），`html.dark` 确实存在。

**Rules.vue**

- section ↔ 后端索引映射**全部正确**（逐段核对 `app.go:1117-1175`）：`local`→`ExtractRulesFromRootPublic`；`subscription`→`ApplyDeleteOnly(workingRules, overlay.Delete)`（与 `GetRulePageData` 返回的 `SubscriptionRules` 同源，:1034/1038）；`add`→`overlay.Add`；`delete`→`overlay.Delete`。`originalIndex` 取的是**全量**数组下标而非分页下标（:424-444），这点是对的。
- 不存在"整段覆盖丢规则"：本页从不调用 `SaveRuleSection`/`SaveCustomRules`（grep 全前端无调用点），只用增量接口 `AddRuleFromForm`/`DeleteRule`。
- `AddRuleFromForm` 的请求体 `{type,payload,policy}` 与 `clash.BuildRuleRequest`（`rule_form.go:31-35`，json tag 小写同名）完全对应；`GetRuleFormOptions` 返回的 `types[].{value,label,count,needPayload,needPolicy,payloadLabel,payloadHint}`（`rule_form.go:10-19`）与前端读取的字段一一对应，`needPayload !== false` 的兜底（Rules.vue:202-203）覆盖了字段缺失。
- `splitRuleTopLevel`（:403-419）对 `AND,((NETWORK,UDP),(DST-PORT,443)),REJECT` 这类嵌套括号规则解析正确（depth 计数）；MATCH 规则单独处理 policy（:432-437），`parts.slice(2).join(', ')` 保留 policy 内的逗号。
- 搜索 debounce 300ms（:318-324）+ `currentPage` 复位；`totalPages` 变化自动夹住当前页（:392-396），`|| 1` 兜住空列表，无 off-by-one。
- `searchTimer` 在 `onUnmounted` 清理（:314-316）；`ruleTab`/`searchQuery` 变更都会复位分页。
- `RulePageData` 的 json 字段名（`configType/subscriptionRules/localRules/addRules/deleteRules/effectiveRules`，`rule_overlay.go:32-39`）与前端读取完全一致；后端 nil 切片会变 null，但前端每处都用 `|| []` 兜住（:304-311）。

**Subscriptions.vue**

- `GetLocalConfigs()` 的 `| null` 已在调用处兜住（:270 `await ... || []`）。
- `UpdateSub` 与"同名不覆盖"的事实已核对后端（`sub.go:51-55`，见 P-25）：不存在"静默覆盖已有订阅"的数据丢失——这条我一开始怀疑，读完 Go 后排除。
- `subs-update-*` 三个事件都有监听（:614-628），`isUpdating` 在 success/error 两条路径都会复位；`UpdateSingleSub` 是普通 RPC，**不会**触发这三个事件（`subscriptions.go:51-72` 无 Emit），不存在"单个更新触发『全部成功』toast"的交叉。
- `handleSelectConfig` 的乐观更新有 `currentSelectionNonce` 序号守卫（:284-317），快速连点不同配置不会互相回滚；失败才回滚并提示。
- `maskUrl` 走 `new URL` + 失败兜底，不会因非法 URL 抛错；分享链接只打码展示，剪贴板写完整（符合 F-A4-19 修法）。
- `ExportConfig` 已改成 `openModal({onConfirm})`（:451-461），"取消不导出"正确（F-A4-19 已修）。
- 菜单 Teleport + fixed 定位 + 首帧 `anchorMenuStyle`（:556-581）避免"从右上角飞入"；`scroll/resize/click` 三个监听在 `onUnmounted` 全部移除（:638-646）。
- `configsLoading` 只在"列表为空"时显示占位（:34），不会在已有列表上闪一下 loading。
- `openDeleteModal` 对"当前激活配置"有专门的警告文案（:180-183），删除路径本身会先关系统代理/TUN（`subscriptions.go:117-160`）。

**YamlEditor.vue**

- `ReadConfigText` 返回 `{id,name,type,content,path}`（`config_editor.go:15-21`），前端只取 `name/content`，字段一致。
- `SaveConfigText` 在**写盘前**就做 YAML 语法与引用完整性校验（`config_editor.go:59-86`），所以"保存非法 YAML 把配置写坏"不会发生；`ValidateConfigText`（:102-111）与保存同一套校验，前端"校验"按钮与保存结果一致。
- `props.configId` 在当前挂载方式下（`App.vue:103-105` 的 `v-if` + 每次切回都新建）在实例生命周期内不变，所以没有"外部内容变更覆盖未保存编辑"的活 bug（见 P-33 的结构性隐患）。
- `ValidateConfigText(content)` / `SaveConfigText(id, content)` 的参数顺序与 binding 一致（`coreservice.ts:194,296`）。
- `EditorView` 在 `onUnmounted` `destroy()`、主题 `MutationObserver` `disconnect()`、keydown `removeEventListener`（:388-392）——**正常（非竞态）路径**下清理完整，P-06 只针对 await 竞态。
- 主题切换通过 `Compartment.reconfigure`（:370-381）实现，不重建编辑器、不丢撤销栈。
- `editorBaseSetup` 未引入 `basicSetup`/`drawSelection`，但自定义 keymap 覆盖了 `defaultKeymap/historyKeymap/foldKeymap/completionKeymap/searchKeymap`，功能面无缺失。

---

## 6. 已知问题（历史已记录，不计入新问题数）

| 旧 ID | 旧状态 | 与本轮关系 |
|---|---|---|
| **F-A4-10**（P1，fixed(B2)）代理页「全部测速」失败完全静默 | 已修 | **修复未命中，问题以另一条路径复现 → 见 P-02。** 历史修法给 `App.TestAllProxies` 包了 `catch`+toast，但该 binding 返回 `void`（`coreservice.ts:264`），catch 不可达；真实失败原因在后端 `proxy-test-finished` 的 string payload 里（`delay_manager.go:461/501-511/521-524`），前端监听器不读参数（`Proxies.vue:769`）。建议把 F-A4-10 重新打开并与 P-02 合并处理。单节点那半（`includes('busy')` → `DELAY_TEST_BUSY`）确认已修好。 |
| F-A4-11（P1）完全静默的失败面 | 已修 | 本轮不重复统计"零痕迹失败"，只把仍新出现的静默点（P-02、P-16）按新问题列。 |
| F-A4-12（P1）成功反馈走模态 | 已修 | 核查：Rules 添加成功用 `showToast`（`Rules.vue:471`）、Subscriptions 单条更新用 `showToast`（`:338`），符合现策略。 |
| F-A4-13/F-A4-14/F-A4-15（i18n 槽位/正则语义/code 契约） | 已修 | 残留：`String(e)` 与 `toUserError` 混用（见 R-5），同类已降级为冗余项。 |
| F-A4-01/F-A4-02（模态单槽/直写 modal） | 已修 | 核查：`showConfirm` 队列语义正确（`store.ts:623-645`、`569-587`），本范围所有确认框都走队列，无"顶掉 pending"问题。P-01 的两次确认是队列**正常**出队造成的，根因是前端缺写操作互斥，不是模态缺陷。 |
| F-A4-19（订阅导出取消仍导出/URL 明文） | 已修 | 核查：`handleExport` 已成 `openModal({onConfirm})`（`Subscriptions.vue:451-461`），`maskUrl` 打码正确。 |
| F-A4-08（启动链在订阅前） | 已修 | 与本范围无交集；仅确认 `main.ts:12` `initStore()` 在页面挂载前注册全局事件。 |

---

## 7. 需要跨文件/后端确认的疑点（移交主控）

1. **`proxy-test-finished` 的 payload 契约**（P-02 根治）：当前是 string——成功 `"测速完成"`、失败是 `ErrDelayTestBusy.Error()` 或 `"测速启动失败：…"`/`"读取代理拓扑失败：…"`/`"没有可测速的有效节点"`。前端无法可靠区分成功/失败（只能字符串比对，正是 F-A4-14 描述的坑）。建议改成 `{ok:boolean, reason:string, code:string}`，或拆 `proxy-test-finished`/`proxy-test-failed`。**需后端改动。**
2. **`SaveConfigText` 的部分成功语义**（P-30）：`config_editor_controller.go:24-33` 在"文件已写盘、运行配置重载失败"时返回 error。建议改成 `(saved bool, reloadErr error)` 或错误码 `SAVED_BUT_RELOAD_FAILED`。
3. **`Tasks.Run` 的 "-cancelled" 没人管**（P-27）：`core/tasks/manager.go:99` emit `name+"-cancelled"`，`v3_events.go` 未注册、前端未监听；geo/core-update 等任务共用同一套，建议在 store 统一兜底（或后端把 cancelled 折成 error 语义）。
4. **订阅排序是否保留入口**（P-05）：后端无任何 `*Order*`/`*Reorder*`/`*Sort*`/`*Move*` 方法（`frontend/bindings` 全量 grep 无匹配，已复核 appservice+coreservice+updateservice）。要么后端新增 `SaveSubOrder(ids)`，要么前端删掉"排序"按钮。
5. **`TestAllProxies` 是同步阻塞调用**：`delay_manager.go:428-533` 在调用方 goroutine 里跑完整批测速（最长 `TotalTimeout=45s`），前端 `await` 它要挂住一个 RPC 往返。若 Wails v3 对同一 service 的方法派发是串行的，测速期间同页其它 `App.*` 调用会被排队。**需主控在后端/框架层确认派发模型**（无条件实测，标 C）。
6. **`ConnectionDTO.Chains` 是否可能为 null**（P-18）：取决于内核 `/connections` 是否总返回 `chains` 数组。建议后端在 `toConnectionDTO` 里保证 `append([]string{}, v.Chains...)` 一劳永逸。
7. **错误被折成空数据的两处**（P-16）：`app.go:317-347` 在 `GetOfflineData`/`clash.GetInitialData` 失败时返回 `groups:{}`。建议加 `"loadError":true` / `"error":…` 字段，让 Proxies 能区分"真空"与"读失败"。
8. **死接口面**：`SaveRuleSection`/`AddRule`/`SaveCustomRules`/`GetCustomRules`/`SyncRules` 在 Go 侧存在（`app.go:981-1219`，三个已标 `Deprecated`）但前端零调用；请主控决定是否清理（与 F-A0-03"死接口仍在服务面"同类）。
9. **`GetOfflineData` 把"嵌套组"当叶子节点**（`config.go:114-146` 只写 `type/now/all`，无 isGroup 信息）：Proxies 的协议标签对"组中组"会显示成 `SELECTOR` 之类。视觉小瑕疵，需确认是否接受。
10. **`proxy-state-sync` 的 `ProxyGroupState` 只有 `Name/Now`**（`proxy_state.go:16-20`）：若将来要在事件里同步其它组状态，P-03 的快照失效逻辑需要跟着改；建议后端补一个 `topologyVersion`，前端据此 Invalidate 冻结快照（可同时根治 P-03/P-09）。
