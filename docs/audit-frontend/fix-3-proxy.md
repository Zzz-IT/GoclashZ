# fix-3-proxy · 修复记录（Proxies.vue / Connections.vue）

> 代理：fix-3-proxy（Wave 1） · 时间：2026-11-18（Asia/Singapore）
> 独占文件：`frontend/src/components/Proxies.vue`、`frontend/src/components/Connections.vue`（只改了这两个源码文件）
> 未改：后端 Go（本 Wave 不改）、`frontend/src/locales/*`、其它组件、`utils/*`、依赖

## 1. 改动总表（18 条）

| ID | 状态 | 改动位置(file:line) | 一句话说明 |
|---|---|---|---|
| **P1-G**(P-03) | fixed | Proxies.vue:567-588, 650 | 新增 `refreshFrozenNodes(prev)`：loadData 重建 `localGroups` 后用**新对象**重建冻结快照（顺序沿用旧快照、按名字迁移 `testing`、新节点追加末尾），消除"渲染旧对象、事件写新对象"的失联 |
| **P1-E**(C-06) | partial | Proxies.vue:741-747, 769-789, 794-796, 873-875, 891-892, 913 | 引入批次隔离：`testBatchStarted`/`testBatchNodes`；start 只在测速中受理、update 只认本批开测过的节点、finished 在"本批还没开测"时丢弃迟到成功通知，不再复位新一轮 isTesting |
| P-02 | fixed | Proxies.vue:752-767, 906-939 | `readTestFinish()` 双兼容读 payload（**优先 `{ok,message}` 结构化字段，回退旧 string**）；ok=false 时 `showToast`+`reportError`，`DELAY_TEST_BUSY` 走专用文案 |
| P-04 | fixed | Connections.vue:296-299, 304, 331-337, 343-344, 351 | `monitorGen` generation 守卫：两个 await 之间被卸载/停止则不再调用 `StartConnectionMonitor()`，finally 也不再写 isLoading |
| P-09 | fixed | Proxies.vue:653-682 | loadData 加 in-flight 去重（并发时置 `loadAgain`，结束后补跑一次、不丢刷新）+ 请求序号双保险；6 个入口仍统一走它 |
| P-10 | fixed | Proxies.vue:494-540, 800 | 拆出 `sortedNodes`（排序、不过滤）+ `applyNodeFilter`（渲染层过滤）；冻结快照改存**未过滤**列表，测速中清空搜索不再丢节点 |
| P-11 | fixed | Proxies.vue:181-182, 204-207 | 删除只写不读的 `canScrollLeft`/`canScrollRight` 与 `updateTrackOverflow` 里两行赋值，注释同步改成"两件事" |
| P-12 | fixed | Proxies.vue:21, 249-256, 1065-1067 | 自绘滚动条补 `@lostpointercapture`（复位 dragging，先判 `hasPointerCapture` 再 release，避免 NotFoundError）+ `touch-action:none` |
| P-13 | partial | Proxies.vue:274, 284-287, 972-977；Connections.vue:378-384 | 删本页 4 个永为死的 onActivated/onDeactivated；Proxies 的 `resetSlider` rAF 记 id 并在 onUnmounted `cancelAnimationFrame`。**Rules.vue 的那对留在 handoff** |
| P-14 | fixed | Proxies.vue:439-462 | `measureNodeTextWidth` 按「字体+文本」缓存宽度（`measuredWidthCache`），测速期间不再对全部节点重测 canvas 文本 |
| P-15 | fixed | Proxies.vue:548-559, 968 | `GetAppBehavior` 从 loadData 移出为 `loadBehavior()`，只在 onMounted 调一次（原来每次选节点都多一次 RPC） |
| P-16 | partial | Proxies.vue:136-138, 165-166, 638-642 | 新增 `loadError` 错误态：空态文案区分「读取失败」与「没有分组」；判据用 `isOffline+isRunning` 或「有激活配置却 0 分组」两个前端信号。根治需后端带 field（见 backend-needs） |
| P-17 | fixed | Connections.vue:293-295, 311, 327, 334-337 | `connEventSeq`：事件到达即自增，快照返回后若期间有新事件则丢弃快照，旧快照不再覆盖新事件 |
| P-18 | fixed | Connections.vue:113-114 | 详情页 `(selectedConn.chains \|\| []).join(' ➔ ')`，与同文件 isDirect/getProxyName 写法一致，null 不再抛 TypeError 白屏 |
| P-19 | fixed | Connections.vue:152-158 | `readFilterMode()` 校正 `goclashz_connFilter`：只接受 all/proxy/direct，非法值回落 'all'（不再无高亮 + 0 宽滑块） |
| P-20 | fixed | Connections.vue:501-511, 518, 532 | 暂停刷新态下关闭单个/全部后本地同步移除卡片（`dropConnectionsLocally`），已关闭连接不再滞留在列表里；非暂停态保持原行为 |
| P-21 | fixed | Connections.vue:361-376, 388-391 | `refreshTitleTarget(retry)` 按 Settings.vue applyAnchor 写法做有限次重试（6×120ms，找到即停），卸载时清 timer，不再"探测一次失败就永久不出计数" |
| P-34 | fixed | Proxies.vue:153-160, 163, 293, 321, 325；Connections.vue:140-147, 158, 285 | 两页所有 localStorage 读写改为局部 `readStorage`/`writeStorage`（内部 try/catch），setup 阶段不再可能因存储异常白屏 |

状态统计：**fixed 15 / partial 3 / skipped 0 / 驳回 0**（共 18 条）
partial 的三条原因：P1-E、P-13、P-16 都需要跨文件或后端配合才能彻底收口（详见 handoff-3.md / backend-needs-3.md）。

## 2. 关键实现说明（给复核用）

### P1-G 为什么是"重建快照"而不是"直接清空快照"
- 只清空 `frozenSortedNodes` 能恢复"事件写到被渲染对象"，但 **测速期间排序会重新实时生效**，卡片随延迟跳动（这正是冻结快照存在的理由）。
- 现做法：`loadData` 后立刻把快照里的每个节点**换成新 localGroups 里的同名新对象**，`testing` 按名字迁移，旧顺序保留；新出现的节点追加末尾，消失的节点丢弃。渲染对象 === 事件写入对象，且顺序仍固定。

### P1-E 判据为什么不是"批次号"而是"本批是否已开测"
- 后端 `proxy-test-finished` 不带 batch id（本次也没改 Go），cancel 时 `ok` 仍为 true、message 仍是"测速完成"（`delay_manager.go` 只在 4 个启动失败分支置 `finishOK=false`）。
- 后端 `TestAllProxies` 先 `cancelBatchAndWait(600ms)` 等旧批次 defer 跑完才启动新批次，而旧批次的 finished 在 `close(done)` 之前 emit ⇒ **旧 finished 的到达顺序必然早于新批次的第一个 `proxy-test-start`**。据此作废旧通知。
- 残余窗口（600ms 内旧批次未收尾干净）见下方风险 1；根治需后端 batch id。

### P-02 双兼容
- 本工作树的 `v3_events.go:80` + `delay_manager.go` 已被改为 `map[string]any{"ok","message"}`（不是我改的，我未动 Go）。前端因此**优先**读结构化字段，读不到再退回字符串比较（成功文案"测速完成"）。
- 后续 payload 再变，只需改 `readTestFinish()` 一处。

## 3. 自查
- `git diff` 仅涉及 `Proxies.vue`、`Connections.vue`（+ 本目录 4 个 md）；无调试代码、无临时文件、无备份文件、无调试 console。
- 未新增依赖、未改导出签名、未改持久化键名、未改 UI 文案（除新增的 `proxies.loadFailedHint` 一条新 key，见 i18n-needs-3.md）。
- 未运行 pnpm/vite build、vue-tsc、应用本体（按规则由主控统一类型检查）。
- 手工核对：模板引用标识符逐个存在；`catch {}`（ES2019）与 `target: ESNext` 一致；`Events.On` 回调参数注解 `{data?: unknown}` 与 `WailsEventCallback` 协变兼容（与原有 `{data:string}` 写法同类）。

## 4. 回归风险（≤3）

1. **P1-E 依赖事件到达顺序**：若旧批次收尾超过后端 `cancelBatchAndWait` 的 600ms，旧 finished 可能晚于新批次第一个 start 到达，仍会被当成新批次结束（表现为按钮提前弹回"测速本组"，转圈被清；数据不丢、可再点一次）。彻底修复需后端 batch id。
2. **P-16 的失败判定是启发式**：`groups` 为空且（有激活配置 或 内核在跑却回 ISOffline）就显示"读取失败"。若某配置本身不含任何 Selector/URLTest/Fallback/LoadBalance 组（纯规则 + DIRECT 之类），会误显示为读取失败。后端补 `loadError` 字段即可消除。
3. **P-09 的去重语义**：在飞期间调用 `loadData()` 立即返回（刷新由补跑那轮落地），`selectNode` 等调用点的"刷新已完成"时序比原来略微滞后；数据不会丢，只是晚一拍写回。另 P-20 的本地移除只在暂停态生效，恢复刷新后以事件快照为准（可能出现一次覆盖，语义正确）。
