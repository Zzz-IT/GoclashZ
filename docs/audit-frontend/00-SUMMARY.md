# 前端代码审计总报告 · GoclashZ v3（2026-11-18）

> 方式：Agent Teams 6 个只读审计代理并行分片 + 主控跨切面取证与逐条复核。
> 性质：只读。全程未修改任何源码/配置（本目录下仅有新增报告）。
> 证据等级：A = 代码或脚本输出可直接确证；B = 代码 + 运行时推断；C = 假设待实测。

---

## 0 结论摘要

| 项 | 值 |
|---|---|
| 审计范围 | frontend/src 全部 28 个 .vue + 全部 .ts（约 1.99 万行）+ 4 个 CSS + 8 个语言包；对照 frontend/bindings（108 方法 / 61 事件）与 Go 侧实现 |
| 分片报告 | 6 份（01~06）：138 条编号问题 + 9 条冗余条目 |
| P0 | **0 条**（无一击崩溃 / 数据损坏 / 安全提权级问题） |
| P1 | 27 条原始 → **20 条去重后** |
| P2 | 111 条原始 → 约 95 条去重后 |
| 静态基线 | vue-tsc 0 错；settings-manifest 103 锚点通过；i18n 对齐通过（188 死键 warn） |
| 误报排除 | 6 条（含主控自己初判被分片代理反证 1 条，见 §6） |

**一句话总结**：类型层干净、无崩溃级缺陷；问题集中在四类 —— ① 同一后端配置的多写者互相覆盖（behavior / 主题）；② 异步竞态与「确认框之后才上锁」导致的重复执行；③ 静默失败（测速、内核更新检查、表单校验结果被丢弃）；④ 大量死代码与设计系统半途而废造成的长期维护成本。

---

## 1 修复优先级 Top 12（主控已逐条复核）

| # | 问题 | 位置 | 后果 | 改法 | 复核 |
|---|---|---|---|---|---|
| 1 | **AppBehavior 整对象写回，多入口互相覆盖** | store.ts:134-145 / Settings.vue:170-216 / SettingsAbout.vue:355-367 | 开着设置页点侧栏明暗 → 再改任意设置 → 主题被打回旧值；还原备份后第一次保存把还原结果覆盖回旧值。用户观感是「设置随机回退」 | 短期：设置页保存前重读后端、只合并自己脏的字段；根治：后端提供补丁语义（只写传入字段）的 SaveAppBehavior | 5 个分片独立命中（S-02/U-01/G-04/C-03/H-17） |
| 2 | **YamlEditor 未保存即丢** | YamlEditor.vue:5 / App.vue:646-648 | 返回箭头、侧栏切页直接销毁编辑器，改动无声丢失（isModified 就在手里没用） | closeYamlEditor 前 await showConfirm，或切页统一守卫 | 2 个分片（P-07/C-02），主控复核 |
| 3 | **连点确认框 → 执行 N 次破坏性操作** | Overview.vue:195-209 / TrafficCard.vue:153-166 / App.vue:356-377 | 闸门只挡「正在执行」，不挡「正在确认」：连点 2 次 = 2 个确认框 = 重启内核 2 次 / 清零流量 2 次 | 把 in-flight 标记提到 showConfirm 之前，finally 复位 | 分片 H-03，主控复核 3 处 |
| 4 | **更新 chip 让设置页永久停在「关于」** | App.vue:331,375 + Settings.vue:42,248 | 点过一次更新 chip 后，此后每次进设置都落在「关于」页，直到重启 | 进入设置后把 targetSettingsView 复位 main | 分片 H-01，主控复核（该 ref 无第二处写） |
| 5 | **批量测速失败全静默** | Proxies.vue:676-687,769-780 | TestAllProxies 返回 void，catch 是死代码；真正失败原因在 proxy-test-finished 的 string payload 里被丢弃 → 忙/无有效节点/启动失败都无提示 | 监听该 payload，非成功时提示；后端改结构化 payload | 2 个分片（P-02/C-05），主控复核 binding 为 void |
| 6 | **测速中点节点 → 转圈永久失联** | Proxies.vue:474-484,526-588,665-692,742-771 | loadData 重建 localGroups 新对象，但冻结快照仍指向旧对象：渲染旧对象、事件写新对象，转圈与进度不再更新 | loadData 内作废 frozenSortedNodes（一行），或事件按 name 同步冻结快照 | 分片 P-03，主控复核对象归属 |
| 7 | **局域网页重置错段名** | SettingsLan.vue:11,181 / Settings.vue:157,233-235 | 子页 emit 的是 network：确认框写「网络」；递增的是 resetVersions.network，LAN 页不重载 → 重置后任一编辑把旧值（含密码）写回 | 子页 emit lan；Settings.vue:233 的 lan→network 映射本就是为它准备的（当前是死代码） | 分片 G-01，主控复核（映射从未命中） |
| 8 | **波形自动缩放只升不降** | trafficWaveState.ts:67-72 | target = max(instant, prevScale) 使 scale 单调不减，scaleFallAlpha 数学上不可达 → 一次高速流量后波形长期被压平，只有手动重置才恢复 | target 改为 instant 本身（让 fallAlpha 生效），或加慢速衰减项 | 分片 H-02 + S-08，主控复核 |
| 9 | **全局 outline:none !important 打死所有焦点环** | style.css:75-80 vs SettingsTheme.vue 5 处 :focus-visible | !important 规则压过组件普通声明 → 全应用无键盘焦点指示，键盘/无障碍不可用 | 改为 :focus:not(:focus-visible) 只在鼠标态移除，或去掉 !important | 分片 U-02，主控复核层叠规则 |
| 10 | **静默失败：检查内核更新** | store.ts:1065-1067 / SettingsUpdate.vue:87 | core-update-check-error 的错误文本被丢弃、只复位标志 → 检查失败用户零反馈 | 事件里取 data 弹 toast/alert | 分片 C-04，主控复核 |
| 11 | **校验结果被丢弃，非法值照常落盘** | SettingsLan.vue:280-287 / SettingsNetwork.vue:290-317 | saveAndValidate 调 validateCIDR 后不判返回值直接 save；hosts 同理 → 非法 IP 段/非法 hosts 写库并注入运行时 YAML（manifest 却声称校验通过才保存） | 校验失败直接 return 并提示；manifest 描述同步改 | 分片 G-02/G-17，主控复核返回值未被使用 |
| 12b | **批量测速失败（记账口径）** | 同上 §1-5 | 记为「新问题 + 标注 F-A4-10 未修完（回归）」：历史修法给 TestAllProxies 包的 catch 因 binding 返回 void 从未执行过 | — |
| 12 | **订阅排序完全无效** | Subscriptions.vue:248-260 | moveUp/moveDown 只改内存数组，108 个绑定方法里没有任何排序落盘 API → 切页即回退，但 UI 有「完成」按钮暗示已保存 | 后端加 SaveSubOrder，或删掉排序入口 | 2 个分片（P-05/C-01），主控复核 move 只操作本地 ref |

---

## 2 P1 全量台账（去重后 20 条，按主题）

### 2.1 同一后端状态的多写者冲突（最高价值主题）

| ID | 位置 | 问题 | 分片来源 |
|---|---|---|---|
| P1-A | store.ts:134-145 / Settings.vue:202-216 / SettingsAbout.vue:355-367 | behavior 整对象覆盖写（见 §1-1） | S-02 / U-01 / G-04 / C-03 / H-17 |
| P1-B | SettingsLan.vue:11 / Settings.vue:233-235 | LAN 重置错段名（见 §1-7） | G-01 |
| P1-C | store.ts:1185-1217 + useTunToggle.ts:129-134 | pending 看门狗 60s 强清把「用户在确认框前思考」当「卡死」误清：允许重入，且旧流程 finally 的 settleTunIntent 会清掉新流程的 intent overlay | S-01 |
| P1-D | store.ts:180-203 / Overview.vue:256-266 | desiredSystemProxy/desiredTun 被 UI 直写且无 revision 保护（事件路径有门控、UI 直写没有）→ 关闭失败时卡片显示「已关闭」而系统代理实际仍开（证据 C，依赖到达顺序） | S-03 |
| P1-E | Proxies.vue:646-692,769-780 | 「停止测速 → 立刻重测」：上一批迟到的 proxy-test-finished 把新一轮 isTesting 清成 false | C-06 |

### 2.2 竞态 / 重复执行 / 泄漏

| ID | 位置 | 问题 | 分片来源 |
|---|---|---|---|
| P1-F | Overview.vue:195-209 / TrafficCard.vue:153-166 / App.vue:356-377 | 确认框之后才上锁 → 连点执行多次（见 §1-3） | H-03 |
| P1-G | Proxies.vue:474-484,526-588 | 测速中 loadData 换对象致转圈失联（见 §1-6） | P-03 |
| P1-H | Connections.vue:277-312 | startMonitor 在两个 await 之间被卸载打断 → 卸载后仍 StartConnectionMonitor()，后端 1s 轮询空转、isMonitoring 与后端分叉 | P-04 |
| P1-I | YamlEditor.vue:367-392 | keydown 与 theme MutationObserver 在 await loadConfig() 之后注册，卸载竞态下永久泄漏（含 EditorView 闭包） | P-06 / C-18 |

### 2.3 静默失败 / 功能失效

| ID | 位置 | 问题 | 分片来源 |
|---|---|---|---|
| P1-J | Proxies.vue:676-687,769-780 | 批量测速失败静默（见 §1-5） | P-02 / C-05 |
| P1-K | store.ts:1065-1067 | 内核更新检查失败静默（见 §1-10） | C-04 |
| P1-L | Subscriptions.vue:248-260 | 订阅排序不落盘（见 §1-12） | P-05 / C-01 |
| P1-M | SettingsLan.vue:280-287 / SettingsNetwork.vue:290-317 | 校验结果丢弃（见 §1-11） | G-02 / G-17 |
| P1-N | SettingsUwp.vue:29,143-146 | 按钮文案「反选」，实际是「全部取消豁免」（一次误点 + 应用 = 清空所有 UWP 豁免）；代码注释显示行为是上一轮有意改的，label 未跟上 | G-03 |

### 2.4 逻辑错误 / 状态残留

| ID | 位置 | 问题 | 分片来源 |
|---|---|---|---|
| P1-O | App.vue:331,375 | 更新 chip 后设置页永久落「关于」（见 §1-4） | H-01 |
| P1-P | trafficWaveState.ts:67-72 | 波形缩放棘轮（见 §1-8） | H-02 / S-08 |
| P1-Q | SettingsAbout.vue:355-367 | 还原备份后前端 behavior 快照不刷新（同 §1-1 根因） | G-05 |

### 2.5 可访问性 / 设计系统 / 国际化

| ID | 位置 | 问题 | 分片来源 |
|---|---|---|---|
| P1-R | style.css:75-80 | 全局 outline:none !important 打死焦点环（见 §1-9） | U-02 |
| P1-S | utils/format.ts:37-50、utils/errors.ts:109 | 工具层硬编码中文（从未/刚刚/分钟前/小时前/天前/（已记录日志）），8 语言下固定中文；是历史 F-A4-16「已修」的漏网点 | U-03 / G-22 |
| P1-T | utils/theme.ts:412-415 + tokens.css | 「圆角」设置只被 7~8 处 var(--ui-radius-*) 消费，实测 163 处 border-radius 声明中仅 7 处走变量 → 设置对按钮/输入框/模态/标签栏完全无效 | U-04 |

---

## 3 前后端对接台账（可量化部分）

### 3.1 方法契约

| 项 | 实测 | 说明 |
|---|---|---|
| binding 方法总数 | 108（AppService 31 / CoreService 62 / UpdateService 15） | 与 api.ts 注释一致 |
| 三命名空间同名方法 | 0（两两交集为空） | 展开合并无静默覆盖风险 |
| 前端实际调用 | 111 个调用点 / 83 个方法，**实参个数与签名 100% 一致**（0 TOO_FEW / 0 TOO_MANY） | 无「调用了不存在的方法」、无返回值类型误用 |
| 方法 ID 唯一性 | 108 个 $Call.ByID 全部互异 | 无 ID 冲突导致的串调用 |
| 从未被调用的方法 | 26 个（24%） | 死 API 或「功能没有 UI 入口」，见 §4.2 |

### 3.2 事件契约

| 项 | 实测 | 说明 |
|---|---|---|
| 注册事件（v3_events.go） | 61 | 后端权威集合 |
| 前端监听事件 | 44 | 差集见下 |
| 注册但无监听 | 18 | 其中多数由 component-update-tasks-sync 快照兜底或仅关机路径触发，真实用户可感缺口很小（聚合失败摘要等），应降为 P3 |
| 前端监听但未注册 | 1 | geo-update-(key)-success，动态事件名，v3_events.go 注释已声明为已知设计 |
| 重复订阅同一事件 | 9 组 | 含 store.ts 内 core-update-success 同文件注册两次（L1088 / L1132），可合并 |

**重要修正 1**：Wails v3.1.0 对「未注册事件」只 warning 不丢（events.go:339-344），所以「前端监听的 geo-update-(key)-success 因未注册而收不到」不成立。

**重要修正 2**：注册但「无人监听」不等于「后端白发」。经任务框架（core/tasks/manager.go:91-109 用 name 加 -start/-success/-error/-cancelled 动态发射）核对，core-update-* / driver-install-* / subs-update-* 都有发射端。分片 S 报告称 driver-install-success 无发射端，主控复核为误判（manager.go:109 发射）。

### 3.3 通知策略表脱节

store.ts 的 NOTIFICATION_UI 表 15 项里 7 项后端永不产出（其中 4 项是 silent 级）→ 「A 类静默」整档不可达，本应静默的 ROUTE_SWITCHING / TUN_NOT_READY 全落成 toast（S-04 / C-09）。

### 3.4 模型与契约类型

- 前端在 store.ts:23-50 手抄 UpdateTaskState / OutboundIPResult，未复用 bindings 生成的类型（C-12）：后端改字段时 TS 不会报错。
- 设置页兜底 behavior 对象与 Go Default() 有 3 处值不一致，且缺 4 个后端字段 → Get 失败后任意保存把它们清零（G-06 / U-20）。
- updateStateFromBackend 里约 24 处 PascalCase 兼容分支（rawData.HideLogs 等）全部是死代码（C-10）——Go 侧 json tag 已是 camelCase。

---

## 4 冗余与死代码（用户明确关心的「代码冗余」）

### 4.1 可整块删除的

| 类别 | 内容 | 量 |
|---|---|---|
| 整文件死代码 | utils/normalize.ts（唯一导出零引用，无 import） | 19 行 |
| 死导出 | findSettingById、formatDurationCn、getRecentErrors、GLASS_BLUR_OVERLAY_DEFAULT、GLASS_BLUR_CONTROL_DEFAULT、normalizeGlassBlurStrength | 6 个符号 |
| 冗余导出 | 32 个符号只在本文件内部使用却仍 export | 32 个 |
| 死兼容分支 | modal 旧式直写兼容 4 处（全仓 0 处直写）；updateStateFromBackend 的 PascalCase 分支 24 处 | 约 70 行 |
| 死 CSS | Rules 的 .loading-state、Proxies 的 scrollbar-button/track/thumb、.size-standard 与基础 .node-grid 重复、Sidebar nav-slide 过渡（TransitionGroup + v-show 永不触发）、约 50 行永不匹配兜底块、2 个无消费者 keyframes | 约 200 行 |
| 死 token | tokens.css 14 个自定义属性零消费（含完整 5 档 z-index 体系），同时硬编码 9999/10000 双轨 | 14 个 |
| 死钩子 | 全仓无 KeepAlive，Proxies/Connections/Rules 共 6 个 onActivated/onDeactivated 永不触发 | 6 处 |
| 死状态 | YamlEditor.statusText 写 7 次读 0；Proxies.canScrollLeft/Right 只写不读；node.now 从未读 | 4 处 |
| i18n 死键 | 188 个（跨 8 语言约 1504 行）；其中 tray.* 11 键前端零引用 | 188 键 |

### 4.2 死 API（26 个后端方法前端从未调用）

按语义分组（完整清单在 06-contract.md §3-C-07）：

- **有功能但 UI 无入口**：RepairRuntimeAssets / RepairRuntimeAssetsAsync / GetRuntimeAssetRepairProgress（运行组件修复与异步进度）、RepairStartupTask / GetStartupTaskInfo（启动项健康查看与修复，而前端却在监听 startup-task-check-warning 事件 → 只报警无处置入口）、UninstallHelperService（装了 helper 无法从 UI 卸载）、IsConfigEditable / GetEditableConfigs / GetConfigFilePath。
- **被新 API 取代的旧接口**：AddRule / SaveCustomRules / GetCustomRules / SyncRules / SaveRuleSection（规则引擎 V2 迁移残留）、GetCoreVersion（已由 GetRuntimeAssetStatus 取代）。
- **正常**：StartClash / ShowMainWindow / ToggleMainWindow / FlushFakeIP / GetProxyDelay / GetOutboundIP / SearchLogs / SaveUwpExemptions / RepairCoreLayout / RepairDataDir*（由托盘或后端内部调用）。

### 4.3 复制粘贴型冗余（建议抽公共层）

| 重复实现 | 分布 | 建议 |
|---|---|---|
| 选项卡滑块（DOM 量取 + 首次动画抑制）| Proxies / Connections / Rules / App.vue(日志) 4 份同构 | composables/useTabSlider.ts |
| 加载-错误-try/finally 三件套 | 6 个页面 11 处 | composables/useAsyncAction.ts（天然修掉多处入口互斥缺失）|
| 错误提示模板（toUserError + displayErrorMessage + showAlert）| 9 处 | alertError(e, key) |
| 设置子页骨架 + scoped 样式 | 11 个 Settings*.vue 约 500 行 | 统一 .ui-setting-* 样式层 |
| 复制到剪贴板 | 3 套（失败文案一处硬编码 ✗）| utils/clipboard.ts |
| 清理窗口切换后 hover 残留 | 2 套（App.vue 与 hoverGuard.ts）| 只留 hoverGuard |
| 8 个语言包 × 188 死键 | 1504 行 | 删死键；check-i18n.mjs 对动态 key 是盲区，188 条里约 143 条是误报，需先修脚本口径 |

---

## 5 其他值得记的 P2（按主题抽样，完整见各分片）

- **存储**：store.ts 模块级 5 处 getItem + 3 处 setItem、页面 localStorage 读写（Proxies/Connections/Rules）全部无 try/catch → 隐私模式/配额满会中断状态同步甚至白屏（S-05 / C-11 / P-34 / U-09）。
- **日志面板**：2000 行 + v-for 用索引键 + 每 16ms 全量 filter（H-05/C-13/U-23）；新日志无条件滚到底导致无法向上阅读（H-04）；silent 级被 normalizeLogType 归成 info（S-09）；LogSource 有 std/core/app 三值但筛选器缺 std（H-07）。
- **格式化**：formatEtaTime 无小时档 → 3600s 显示 60m 0s、86400s 显示 1440m 0s（U-06）。
- **错误分类**：errors.ts:18 的 /helper|Helper/i 过宽，含 helper 的超时/占用错误都被显示成「需要 Helper 服务」（U-05）。
- **ModernSelect**：挂载期自动纠错 emit change → 直接触发父级保存（G-08/C-17）；Teleport 到 body 后 3 条样式永不生效（G-09）。
- **ModernNumberInput**：输入中间态不 clamp、NaN 回落 min ?? 0、step 非响应式、无 Enter/Esc（G-07）。
- **UpdateTaskPanel**：deep watch 每次任务同步排一个 3s 定时器且卸载不清理，清理时会把 error 任务一并删掉（G-10/C-16）。
- **性能**：resize 每事件一次 Window.IsMaximised IPC 无节流（H-19）；Proxies adaptive 模式对全部节点 canvas measureText（P-14）；每次 loadData 多拉一次 GetAppBehavior（6 入口，P-15）。
- **可维护性**：Proxies 注释与实现矛盾 3 处（轨道高 36+4px 但已 scrollbar-width:none 等），Connections 注释称配合 KeepAlive（无 KeepAlive）—— 比死代码更容易误导后来者（R-9 / P-13）。

---

## 6 误报与推翻记录（审计可信度）

| 结论 | 谁提出 | 谁推翻 | 依据 |
|---|---|---|---|
| 模态队列可停摆、排队 showConfirm 永不 settle | 主控初判（00-lead-crosscut §3.1） | state-core（S-07）+ 主控复核 | 不变量 activeModal 非空 等价于 modal.show 为真 成立（show 只由 presentModal 置真、settleModal 置假），旧式直写在全仓已 0 处，队列分支不可达。结论：系统健全，历史 U4/F-A4-01 已修复 |
| driver-install-success 无发射端（死监听） | state-core | 主控 | core/tasks/manager.go:109 用 name 加 -success 动态发射，组件更新事务 autoSuccess=true |
| 局域网页重置实际正确（233 行已映射） | 主控初判 | settings-ui（G-01） | 子组件 emit 的是 network 而非 lan，映射永不命中；递增的也是 resetVersions.network → LAN 页不重载。代理正确，主控初判错误 |
| TUN 有两条互相打架的写入路径 | 主控初判 | 主控 + state-core | 正常路径只有 useTunToggle 一个实现；但 store.ts:1015-1031 的通知兜底里有一条裸 ToggleTunMode 调用（绕过 pending 闸门），属次要双实现，记 P2 |
| ModernSelect 监听器泄漏 / 托盘多写者 / modal 互顶 | settings-ui 初判 | settings-ui 自推翻（§5） | 见 04-settings.md §5 |
| api.ts 展开合并会静默覆盖 | 常见风险假设 | 主控 + state-core 脚本 | 108 导出 / 108 distinct，两两交集为空 |

---

## 7 建议修复批次

| 批次 | 内容 | 预估 |
|---|---|---|
| **B1 一行级修复**（先做，风险最低） | §1-4 chip 复位、§1-6 loadData 作废冻结快照、§1-7 emit lan、§1-8 scale 目标、§1-10 事件取 data、§1-9 去掉 outline:important | 6 处小改 |
| **B2 加锁/守卫** | §1-3 三处把闸门提到 showConfirm 之前、P1-I YamlEditor 监听移至 await 之前并加卸载判据、P1-H startMonitor 卸载守卫 | 5 处 |
| **B3 静默失败补提示** | §1-5 proxy-test-finished payload、§1-11 校验返回值、P1-N UWP 文案对齐 8 语言 | 4 处 + 1 组文案 |
| **B4 契约补强（需后端配合）** | behavior 补丁语义接口（§1-1、P1-Q）；订阅排序落盘或删入口（§1-12）；proxy-test-finished 结构化 payload；NOTIFICATION_UI 与后端 code 对齐 | 4 个后端改动 |
| **B5 冗余清理** | §4.1 表逐项（188 i18n 死键需先修 check-i18n 动态 key 口径）；§4.3 抽公共层 | 可分 3~4 次 PR |
| **B6 设计系统** | 圆角 token 铺开（163 处）、z-index token 落地、11 个设置页样式合并 | 需专项 |

验收口径：每批改完跑 node scripts/validate-settings-manifest.mjs、node scripts/check-i18n.mjs、vue-tsc --noEmit（当前三者全绿，是可用基线）；B4 涉及事件契约的改动需同步 v3_events.go 注册。

---

## 8 分片报告索引

| 文件 | 范围 | 条数 |
|---|---|---|
| 01-state-core.md | store.ts / trafficWaveState / searchState / api.ts / main.ts | 11 |
| 02-app-shell.md | App.vue / Sidebar / Overview / TrafficCard / DeviationStrip / Toast / EmptyState / useTunToggle | 23 |
| 03-pages.md | Proxies / Connections / Rules / Subscriptions / YamlEditor | 43 |
| 04-settings.md | 13 个 Settings*.vue / manifest / ModernSelect / ModernNumberInput / UpdateTask* | 24 |
| 05-utils-style.md | utils/* / styles/* / style.css / locales + 全前端冗余普查 | 23 |
| 06-contract.md | 全部 App.* 调用点 / bindings / Go 实现 / 事件 / 模型字段 | 23 |
| 00-lead-crosscut.md | 主控跨切面取证（静态基线、事件总账、命名空间、冗余普查） | — |

> 注：01~06 中 P0 均为 0；各分片内部的重复条目已在本总报告中去重合并（如 behavior 覆盖冲突 5 处合并为 P1-A）。
