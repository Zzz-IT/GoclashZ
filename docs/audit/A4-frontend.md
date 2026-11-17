# A4 前端提示面与错误处理审计

> 生成：2026-11-16；范围：frontend/src/**（全部 .vue / .ts，含 locales）
> 审计性质：只读。未修改任何源码/配置，未运行应用、未构建、未 kill 进程。
> 证据等级：A = 代码可确证；B = 代码 + 运行时状态推断；C = 假设待实测。
> 本报告与 docs/SELF_HEAL_AND_LOGGING_PLAN.md（下称「主方案」）独立核对，冲突处在 §3 单列。

---

## 0 覆盖范围与方法

### 0.1 读过的文件（逐行）

| 类别 | 文件 | 读法 |
|---|---|---|
| 提示面核心 | frontend/src/store.ts（931 行） | 全文 |
| 提示面核心 | frontend/src/components/Overview.vue（390 行） | 全文（template+script+style） |
| 提示面核心 | frontend/src/components/SettingsTun.vue（193 行） | 全文 |
| 全局壳 | frontend/src/App.vue（1086 行） | 全文（script + 两处模态 template；style 略读） |
| 页面 | Settings.vue、Connections.vue、Rules.vue、Proxies.vue、Subscriptions.vue、YamlEditor.vue、SettingsAbout.vue、SettingsUpdate.vue、SettingsUwp.vue、TrafficCard.vue、SettingsTheme.vue、SettingsLan.vue、SettingsNetwork.vue、SettingsDns.vue、SettingsBehavior.vue、SettingsHome.vue、DeviationStrip.vue、UpdateTaskPanel.vue、Toast.vue | 逐文件读 script 全文 + 关键 template 片段（模态/按钮） |
| 工具 | utils/errors.ts（54 行）、api.ts、locales/index.ts、locales/types.ts、main.ts | 全文 |
| locales | locales/zh-CN.ts 等 8 个文件（各 845 行 / 777 个叶子 key） | 程序化解析（去 export default 后求值）+ 键集构建，非逐行读 |
| 后端对账（只读参照） | app.go（CheckTunEnv:727、GetRuntimeAssetStatus:897、GetDataDirInfo:663、ExportDiagnostics:384）、core/runtimeassets/status.go、check.go 片段、core/appcore/controller.go 片段、supervisor.go 片段、control_coordinator.go 片段、controller_long_connection.go、delay_manager.go、core/sys/helper_client_windows.go、core/clash/setup.go 片段 | 定点读取，用于确认「前端假设的字符串/成本」是否成立 |

未逐个读：Sidebar.vue、ModernSelect.vue、ModernNumberInput.vue、UpdateTaskRow.vue（只读模板头 30 行）、EmptyState.vue、utils/theme.ts、glass.ts、format.ts、normalize.ts、icons.ts、hoverGuard.ts、scrollFade.ts、edgeFade.ts、trafficWaveState.ts、settings/manifest.ts、styles/*.css。这些文件不含 showAlert/showConfirm/showToast/console.error 调用（已用 grep 确认），本次结论不受影响。

### 0.2 覆盖度自评

| 维度 | 覆盖度 | 等级 |
|---|---|---|
| 提示面调用点（showAlert/showConfirm/showToast）计数与逐条归类 | 100% | A |
| 静默吞错（console.error / 空 catch / .catch(()=>{})）计数与归类 | 100%（计数）/ 90%（影响判定） | A/B |
| 错误文案匹配（includes / 正则 / indexOf）清单 | 100% | A |
| 模态状态机（showAlert/showConfirm/直写 modal）语义 | 100% | A（可达性 B） |
| 前端重后端调用（启动/进页）与成本 | 95% | A（成本数值 B，参照 A0 实测 warm 0.06s/61MB） |
| i18n 键集一致性 | 100%（8 语言 × 777 key 程序化比对） | A |
| 文案与后端 code 对齐 | 90% | A（字符串层）/ B（触发路径） |
| 无障碍/焦点陷阱/键盘（模态 a11y） | 0% | — |

### 0.3 手段

1. 全量 grep 计数：showAlert / showConfirm / showToast / console.error / console.warn / includes / catch / .catch(()=>{}) / globalState.modal = / modal-overlay。
2. 逐文件读 script 段（剔除 style 噪声），按「谁调用、失败后用户看到什么」归类。
3. 字符串反查后端：把前端匹配的字面量（wintun_missing、helper_install_required、no active config selected、ErrNoActiveConfig、DELAY_TEST_BUSY、路由切换中、由于长连接…、User cancelled）逐个在 Go 侧 grep，确认「生产方是否真的会产出」。
4. 成本反查：把前端调用的 UI 方法追到 app.go → core，确认是否落到 GetStatus / -v / SCM / Ping。
5. locales 程序化解析：8 个语言包的键集比对 + 与代码中 t(…) 使用点比对。
6. 计划文本独立核对：主方案 §1.4、§11.2、§11.3、§11.5、§11.6 的数字与断言逐条验证。

---

## 1 链路清单表（前端提示面与重调用）

说明：本表把「前端一次用户动作 → 后端调用 → 失败后用户看到什么」作为一条链路。严格度指该动作在后端触发的检查强度（L0 快检 / L1 中检 / L2 深检 / 网络 / 进程）。成本为 warm 估算，B 级：GetStatus 一次 = 61 MB 内核哈希 + 4 个 geo 哈希（约 30 MB）+ 一次 clash.exe -v（60s 缓存）；hashing 参照 A0 实测 61 MB≈0.06s，进程启动≈0.08s，因此 warm 约 0.2~0.5s，冷启动/杀软首扫放大到秒级（未实测，C）。

| # | 入口/场景 | file:line | 严格度 | 单次成本 | 是否阻塞用户 | 误报面 | 触发频率 | 可用 code 替代 |
|---|---|---|---|---|---|---|---|---|
| 1 | 启动串行链：GetAppBehavior → GetAppVersion → CheckTunEnv → CheckCoreAgainstSeed → GetRecentLogs → StartStreamingLogs | App.vue:614-701（641/646/676/679） | L2+进程 | 2~3 次 GetStatus 级 + 2~3 次 -v | 否（await 不锁主线程），但事件订阅被推迟到 656-704 行之后 | helper/适配器瞬时不可用 → 后续订阅全丢 | 每次启动 | 是（boot.phase 事件 + 各自 code） |
| 2 | 启动：initStore → GetAppState + refreshOutboundIP | store.ts:733-742、849、654 | L2（GetAppState 内含 GetLocalCoreVersion:controller.go:572） | 1 次 -v（可能）+ 一次外网 IP 探测 | 否 | 外网探测失败 → IP 显示「检测失败」但无提示 | 每次启动 | 是（sink 事件） |
| 3 | Overview 系统代理开关 | Overview.vue:217-250（229 GetAppState） | L2 | ToggleSystemProxy（15s ctx）+ 1 次 GetAppState | 是（无超时、无进度） | 无活动配置 → 弹「未选择配置」模态；其余走 toUserError | 每次点击 | 是 |
| 4 | Overview TUN 开关（三分支：helper/wintun/通用） | Overview.vue:252-319（267/289/291；268-272、294-298；301-302） | L2（ToggleTunMode 25s ctx） | 1 次 ToggleTunMode + 1 次 GetAppState（失败分支再 2 次 ToggleTunMode + GetAppState） | 是（tunPending 期间二次点击直接 return，无进度提示） | 字符串 includes 三条分支；wintun 分支大小写敏感 | 每次点击 | 是（主方案 §5.2 的 code 表） |
| 5 | Overview 模式切换 | Overview.vue:342-361 | 网络 | UpdateClashMode + 无 catch 的 GetAppState（346） | 否 | GetAppState 抖动 → 误报「切换模式失败」（实际已切） | 每次点击 | 是 |
| 6 | Overview 重启内核 | Overview.vue:180-194 | 进程 | RestartCore（同步）+ showConfirm | 是（isRestarting 有图标态） | 失败 → toUserError 模态（分层正确，可作样板） | 低 | 是 |
| 7 | 全局通知（tray/startup/deferred-action + level=error） | store.ts:749-777（758 showConfirm，769/775 showAlert） | — | 无 | 是（模态） | 只认 3 个 source；其余静默丢弃 | 后台 | 是（notification.code → class） |
| 8 | 长连接延迟 → 强制降级为 toast | store.ts:467-478 | — | 无 | 否 | 中文字面量正则；后端改文案即失效，退回模态 | 每次受限动作 | 是（DEFERRED_* code） |
| 9 | 设置→TUN 打开页面 | SettingsTun.vue:108-140（118 Promise.all，119 覆盖） | L2 ×2 + helper | 2 次全量 GetStatus 并发 + CheckHelperService + CoreSupportsMips | 是（loading 遮罩，无进度） | 第 2 次结果覆盖第 1 次的 wintunError（119） | 每次进页 | 是（GetStatusCached） |
| 10 | 设置→TUN TUN 开关 | SettingsTun.vue:142-156 | L2 | ToggleTunMode（25s ctx） | 是（无 pending 防重入） | catch 直接 showAlert(String(err))，无 code、无 toUserError、无重试引导 | 每次点击 | 是 |
| 11 | 设置→TUN 自动保存（每项 change/blur） | SettingsTun.vue:167-182（168 loading 闸门，176-180 5s 去重） | 写盘 | SaveTunConfig | 否 | 同一失败 5s 内只弹一次 → 用户可能只看到一次但改了 3 个字段 | 每字段 | 是 |
| 12 | 设置→TUN mips 回退提示 | SettingsTun.vue:123-127、App.vue:698-700 | — | 无 | 是（进页即模态） | 回退从未落盘（见 F-A4-05）→ 每次进页重复弹 | 每次进页 | 是 |
| 13 | 设置→局域网 自动保存 | SettingsLan.vue:251-271（268-270 仅 console.error） | 写盘 | GetNetworkConfig + SaveNetworkConfig | 否 | 完全静默：失败无任何 UI，用户以为已保存 | 每字段 | 是 |
| 14 | 设置→网络 自动保存 | SettingsNetwork.vue:333-341（338-340 仅 console.error） | 写盘 | SaveNetworkConfig | 否 | 完全静默（同上） | 每字段 | 是 |
| 15 | 设置→DNS 自动保存 | SettingsDns.vue:235-243（241 showAlert） | 写盘 | SaveDNSConfig | 否 | 每次失败都弹模态（无去重），textarea 每次失焦都弹 | 每字段 | 是 |
| 16 | 设置→行为 保存 | Settings.vue:195-208（203-206 模态 + 回滚 loadBehavior） | 写盘 | SaveAppBehavior（+ 失败后 GetAppBehavior） | 否 | 回滚是「静默撤销用户改动」+ 模态，用户需重做 | 每字段 | 是 |
| 17 | 设置→重置任意模块 | Settings.vue:17（自建模态）+ 211-224 | 写盘 | ResetComponentSettings | 是（自建模态） | 失败 → showAlert(String(error)) 原始文本 | 低 | 是 |
| 18 | 设置→关于 诊断/备份/还原/导出诊断 | SettingsAbout.vue:444-462、378-388、410-442（456/458/459 硬编码中文；458 User cancelled 死判定） | 写盘+对话框 | GetDataDirInfo = GetStatus + GetSeedCatalogStatus | 是（诊断弹窗无 loading） | 取消与失败不可区分（458）；失败文案硬编码中文 | 低 | 是 |
| 19 | 设置→组件与库更新 打开页面 | SettingsUpdate.vue:72-82、94-105（81/82 console.error） | L2 | 1 次全量 GetStatus + CheckHelperService | 是（无 loading 文案，helper 区显示默认值） | helper 状态查询失败 → 显示「未安装/未运行」假结论 | 每次进页 | 是 |
| 20 | 设置→更新 事件型模态（core-version-updated / core-update-none / app-update-busy） | SettingsUpdate.vue:96-103 | — | 无 | 是（模态，且与用户动作无因果） | 后台事件随时顶掉用户正在看的确认框（见 F-A4-01） | 后台 | 是 |
| 21 | 设置→UWP 加载/保存 | SettingsUwp.vue:102-118（116）、155-210（180/182/204/206） | 提权+写盘 | GetUwpApps / ApplyUwpExemptionChanges | 是（保存后模态） | 成功也用模态；全不选按钮取反（F-A4-20） | 低 | 是 |
| 22 | 代理页 打开/状态变化 | Proxies.vue:524-583（527 GetAppBehavior、534 GetInitialData、580-582 console.error） | 网络+文件 | 2 次串行 IPC，每次 state/config/事件变化重跑 | 否 | 加载失败 → 空组列表，无任何提示 | 每次进页 + 每次状态变化 | 是 |
| 23 | 代理页 全部测速 / 单点测速 / 停止 | Proxies.vue:660-684（673 静默）、687-704（694-700 includes(busy)） | 网络（8s） | TestAllProxies / TestProxy | 否 | 全部测速失败无提示无日志；单点把任何含 busy 的错误吞掉 | 高频 | 是（DELAY_TEST_BUSY） |
| 24 | 代理页 切节点 | Proxies.vue:590-614（612） | 网络 | SelectProxy（3s ctx）+ 重新 GetInitialData + IP 刷新 | 否 | 失败 → 原始 String(e) 模态，无 code | 高频 | 是 |
| 25 | 连接页 进入监控 / 关闭连接 | Connections.vue:269-304（300 console.error）、449-471（456/469） | 网络 | GetConnections + StartConnectionMonitor | 否 | 监控启动失败 → 空列表「暂无连接」（假空态） | 每次进页 | 是 |
| 26 | 规则页 加载 / 新增 / 删除 | Rules.vue:286-297（293 console.error）、385-413（407/409）、415-440（435） | 网络+写盘 | GetRulePageData / AddRuleFromForm / DeleteRule | 否 | 加载失败 → 空规则列表；新增失败把原始错误同时塞进 title 与 body | 每次进页 | 是 |
| 27 | 订阅页 切配置 / 更新 / 导入 / 重命名 / 导出 / 删除 / 分享 | Subscriptions.vue:280-311（309）、313-338（331/334）、362-381（376-377）、390-403、410-450（420/434）、467-479（474-475）、496-521（506/512） | 网络+写盘 | 多次 IPC + fetchConfigs（GetLocalConfigs + GetInitialData） | 是（多处模态） | 成功后仍用模态；导出「取消」也执行导出；导入取消与失败都记 console.log | 高频 | 是 |
| 28 | YAML 编辑器 读取/保存/校验/重载 | YamlEditor.vue:263-305（295-301）、307-329（320-325）、331-348、350-356 | 写盘 | ReadConfigText / SaveConfigText / ValidateConfigText | 否 | 校验失败不弹窗（只改状态栏），保存失败弹模态 —— 同类失败两种反馈 | 中 | 是（CONFIG_INVALID） |
| 29 | 主界面 流量重置 | TrafficCard.vue:151-157 | 写盘 | ResetTrafficTotals（无 try/catch） | 否 | 后端失败 → 无提示且波形不复位 | 低 | 是 |
| 30 | 主界面 出站 IP 探测 | store.ts:625-729（663 匹配「路由切换中」） | 外网 | GetOutboundIPForRoute | 否 | 中文字面量匹配；失败只写「网络请求失败」内联文案 | 事件驱动 | 是 |
| 31 | 设置子页偏差条 | DeviationStrip.vue:63-80（69 空 catch） | 读配置 | GetConfigDeviations | 否 | 失败即当「无偏差」，漂移提示消失 | 每个设置子页 | 是 |
| 32 | 更新任务面板 | UpdateTaskPanel.vue:30-36（34 console.error） | 写盘 | ClearFinishedUpdateTasks | 否 | 清理失败 → 任务列表常驻 | 后台 | 是 |

---

## 2 问题清单（按严重度排序）

计数口径统一说明（后文引用）：showAlert 69 处调用 / 66 行；showConfirm 17 处；showToast 6 处；console.error 28 处；此外完全静默：.catch(()=>{}) 5 处、catch(e){} 2 处、catch{} 1 处；直接赋值 globalState.modal 10 处；自建模态 8 处。

### P0

#### F-A4-01 单一模态槽：showAlert/直写 modal 会顶掉 pending 的 showConfirm，导致 Promise 永不 settle、开关永久卡死

- 严重度：**P0**；证据等级：A（状态机可确证）+ B（触发概率未实测）
- **file:line**：store.ts:474-493（showAlert 直接改写 modal）、store.ts:496-507（showConfirm）、App.vue:551-559（handleModalConfirm/Cancel 读全局 onConfirm）、App.vue:731/743/768/778/793/811/828（直接赋值）、Overview.vue:268-272、294-298（await showConfirm 在 try 内）、Overview.vue:314-318（finally）、Overview.vue:253/258（tunPending 闸门）、Overview.vue:218/225（systemProxyPending 闸门）
- **现象**：全局只有一个 modal 对象，onConfirm/onCancel 是可变字段而非按 id 绑定。任一时刻出现第二个提示（后台 app-update-available 事件、其他组件 showAlert、Subscriptions 直写 modal、store.ts:749-777 的通知）就会覆盖 onConfirm。被覆盖的 showConfirm 的 resolve 永远不会被调用，await 之后的代码与 finally 都不执行。
- **影响**：以 Overview TUN 为例，用户点 TUN → 后端返回「需要 helper」→ 弹确认框（tunPending 已为 true，:258）；此时后台更新检查完成，App.vue:731 直写 modal 覆盖；用户点「确定」执行的是更新下载；那个 await 永远悬挂 → :314 的 finally 不执行 → tunPending 恒为 true → TUN 卡片此后每次点击在 :253 直接 return，直到重启窗口。系统代理分支同理（systemProxyPending 恒 true）。这是「点了没反应」的最高优先级来源，且日志里完全无痕。
- **修法**：
  1. modal 增加自增 id 与 ownerToken（store.ts:286-296 的 modal 结构加 id:0）；showAlert/showConfirm 创建时记录 id，onConfirm/onCancel 携带该 id 校验（App.vue:551-559 改为 handleModalAction(id, ok)，id 不匹配直接忽略）。
  2. showAlert 入口加互斥：若当前 modal.show 且 type 为 confirm，把新提示排队（新增 pendingModals: ModalRequest[]，关闭后 drain）而不是覆盖（store.ts:474-493）。
  3. 所有 await 型模态调用方加超时兜底：新增 showConfirmT(msg, title, danger, timeoutMs=30000)，超时 resolve(false)（Overview.vue:182/268/294、Connections.vue:450/462、Rules.vue:423 等）。
  4. 10 处直写 modal 全部替换为 showAlert/showConfirm/showModal（清单见 F-A4-02），并在 store.ts 里把 modal 改成只读 getter 导出，编译期堵住直写。
  5. Overview.vue:314-318 的 finally 增加 tunPending=false 的 watchdog（进入 try 时 setTimeout 兜底 30s 复位），防止任何未预见悬挂。

#### F-A4-02 10 处绕过提示 API 直接赋值 globalState.modal

- 严重度：**P0**（与 F-A4-01 同源，且是它的主要触发者）；证据等级：A
- **file:line**：App.vue:333-350（更新就绪）、731-756（app-update-available）、743-753（下载失败）、768-806（下载完成）、778-789（安装路径空）、793-803（安装器启动失败）、811-821（已最新）、828-838（更新失败）、Subscriptions.vue:420-429（分享链接）、434-449（导出二选一）
- **现象**：这些块手工构造 modal 对象（含 type/isDanger/onConfirm/onCancel），与 store.ts 的 showAlert/showConfirm 是两条并行实现；App.vue:743 与 793 甚至在 onConfirm 内部再次直写 modal（嵌套覆盖）。
- **影响**：① 只要它们与页面的 showConfirm 同时存在就产生 F-A4-01 的悬挂；② detail 字段语义不统一（这些块固定写 detail 空串，会清掉上一次的原始错误详情）；③ 无法统一加 code/复制诊断/排队；④ isDanger 与按钮文案在 App.vue:174 固定为「我知道了」，confirm 与 alert 的交互语义只能靠调用方自觉。
- 修法：新增 store.showModal({title, message, detail, type, isDanger, confirmText, cancelText}) 与 showAlert/showConfirm 同源（store.ts:474-507 抽公共 createModal）；把上述 10 处逐一替换；Subscriptions.vue:445-448（onCancel 也执行导出）改用 showConfirm 后按返回值分支，见 F-A4-19。

#### F-A4-03 TUN 开关两套实现：Overview 有引导+重试+错误分层，SettingsTun 只有一句原文模态

- 严重度：**P0**（同一台机器两种行为，且设置页是主路径）；证据等级：A
- **file:line**：Overview.vue:252-319（三分支、helper 确认、install 后重试 :276、wintun 确认后装驱动并重试 :301-302、通用分支 toUserError :311-312）vs SettingsTun.vue:142-156（:145-150 只判 hasWintun 并弹 missingDriverMsg；:152-155 catch 里 showAlert(t(settings.tun.tunError,{error:String(err)}))，无 code、无 detail、无重试、无 helper 分支）；成功路径差异：Overview.vue:262-263 成功后 GetAppState 回灌，SettingsTun.vue:152 完全不回灌（靠事件）；pending 差异：Overview 有 tunPending 闸门（:253），SettingsTun 无任何防重入。
- **现象**：在设置页开 TUN 遇到 helper 未装/驱动缺失，只会看到一句「开启 TUN 失败：runtime 事务失败于阶段…」；在首页同样的失败会引导安装并自动重试。
- **影响**：用户被引导去首页；设置页路径失败率高、且失败提示不可行动（无 code、无 detail、无复制诊断）。
- 修法：新增 frontend/src/composables/useTunToggle.ts，导出 { pending, toggle(target) }：内部含 ① intent（setTunIntent/settleTunIntent）② pending 闸门 ③ catch 后按 code 分支（helper → showConfirm 安装，wintun → showConfirm 装驱动，其他 → toUserError + detail）④ 成功后 App.GetAppState + updateStateFromBackend。Overview.vue:252-319 与 SettingsTun.vue:142-156 改为调用它；删除 SettingsTun.vue:145-150 的本地 hasWintun 判定（改由 code 决定）。

#### F-A4-04 「缺驱动」分支大小写敏感：真正会出现的 wintun 包装错误漏判，而唯一命中的分支直接强制重装

- 严重度：**P0**（误报+漏报同时存在，正是主方案要根治的点）；证据等级：A（字符串层）+ B（可达性：需 Reconcile 在资产阶段失败）
- **file:line**：Overview.vue:291（msg.includes(wintun_missing) || msg.includes(Wintun)）；生产端：core/appcore/controller.go:1778（缺少 Wintun 依赖: %s (%s)，大写 W）、core/appcore/supervisor.go:167（返回 ErrWintunMissing，正文小写 wintun_missing，:21）、core/appcore/controller.go:661（运行组件不可用: %w 包装 core/runtimeassets/status.go:69 的 wintun 仍不可用: …，小写 w）；消费端后果：Overview.vue:294-309（确认框：直接 App.InstallTunDriverAsync(true) 强制重装 + 重试 ToggleTunMode）
- **现象**：① 小写 wintun 仍不可用（status.go:69，经 controller.go:661 包成 运行组件不可用: wintun 仍不可用: …）匹配不到 includes(Wintun) 也不匹配 wintun_missing → 落到通用分支（:311-312），用户只看到一句人话错误，看不到「装驱动」这个唯一可行动作；② 反过来，任何含大写 Wintun 的错误（含未来新文案）都会被当作「缺驱动」，并弹出一个按钮写着「确定」却会直接执行强制重装的确认框——用户不知道自己点了会重装驱动。
- **影响**：主方案 §1.2-C 的定性方向对（文案猜语义），但具体误报面描述反了：当前真正的病症是「漏判 + 隐藏的强制写操作」，而不是「一切 wintun 文案都被翻译成缺驱动」。详见 §3。
- 修法：Overview.vue:291 删除 includes，改判 code === ASSET_WINTUN_MISSING；把 :294-298 的确认文案改成明示后果（将重新安装 Wintun 驱动并重试）且 isDanger=true；后端在 controller.go:1760-1787 与 supervisor.go:154-175 两条路径都返回带 code 的错误（主方案 §5.2），前端只认 code；小写包装路径（controller.go:655-661）应改成分级降级（主方案 #7）而不是丢弃。

#### F-A4-05 mips 回退从未落盘：每次进设置→TUN 都弹一次「已回退」，配置永久不一致

- 严重度：**P0**（确定性重复弹窗 + 数据未生效）；证据等级：A
- **file:line**：SettingsTun.vue:114-140（loadData：:115 loading=true；:123-127 判定 stack 为 mips 且 !supportsMips → :124 改本地值 → :125 void saveTun() → :126 showAlert）+ SettingsTun.vue:167-168（saveTun 第一行 if (loading.value) return;）
- **现象**：:125 调用 saveTun 时 loading 仍为 true（复位发生在 :132 的 finally），所以 saveTun 在 :168 直接 return，SaveTunConfig 从未被调用；页面内存里是 mixed，磁盘上仍是 mips；下次进页面重复上述流程，重复弹窗。
- **影响**：① 每次进/回设置→TUN 都吃一次模态弹窗（主方案 §9.1 要求模态=0）；② tunConfig 与真实配置长期背离，DeviationStrip 也会持续报偏差；③ 用户如果手动改别的字段，会自动保存 mixed，但没人告诉他。
- 修法：把回退保存移出 loading 闸门——在 SettingsTun.vue:123-127 内改为 await saveTun({ force: true })，并给 saveTun 增加参数 saveTun(opts?: {force?: boolean}) 把 :168 改成 if (loading.value && !opts?.force) return;；showAlert 改为 showToast（一次性提示），并用 sessionStorage 标记已提示，避免每次进页重复。

#### F-A4-06 设置自动保存四套失败处理，其中两处完全静默

- 严重度：**P0**（用户改了配置以为保存成功，实际丢改动——属于静默失败最坏形态）；证据等级：A
- **file:line**：SettingsLan.vue:251-271（:268-270 仅 console.error）、SettingsNetwork.vue:333-341（:338-340 仅 console.error）、SettingsDns.vue:235-243（:239-242 toUserError + showAlert，无去重）、SettingsTun.vue:167-182（:172-181 toUserError + 5s 去重 + detail）、Settings.vue:195-208（:203-206 console.error + showAlert + loadBehavior 回滚）
- **现象**：同一个「改字段即保存」的模式，五个设置页（LAN/网络/DNS/TUN/行为）出现四种反馈：无反馈 / 每次弹 / 5s 去重弹 / 弹+回滚。
- **影响**：LAN 与网络两页的失败对用户完全不可见（无 toast、无内联、无日志文件——只有 DevTools console），而这两页承载 allowLan、lanAuthPass、mixedPort、hosts 等关键项；改错端口导致内核起不来时，用户看到的是「设置页显示已改」+「内核启动失败」，因果链断裂。
- 修法：新增 frontend/src/utils/saveFeedback.ts，导出 saveWithFeedback(fn, opts:{label:string; dedupeMs?:number})，内部 toUserError + toast（B 类）/ showAlert + detail（C 类）+ 失败时 deviationRef.refresh()。替换 SettingsLan.vue:251-271、SettingsNetwork.vue:333-341、SettingsDns.vue:235-243、SettingsTun.vue:167-182、Settings.vue:195-208 五处；统一失败后回读后端值（LAN/网络两页当前失败后仍显示用户输入值，属误导）。

### P1

#### F-A4-07 8 处自建模态与全局模态并存；同一「重置」动作有两种确认 UI

- 严重度：**P1**；证据等级：A
- **file:line**：Settings.vue:17 + :210（confirmReset 用 showResetConfirm 布尔）vs SettingsTheme.vue:634-637（同一语义用 showConfirm）；其它自建：App.vue:186-216（core seed 对比）、Rules.vue:68 起、SettingsAbout.vue:170-205（还原备份）、207 起（数据目录诊断）、SettingsUpdate.vue:39（DB 链接）、40（内核更新确认）、Subscriptions.vue:108 起（导入/重命名/删除）
- **现象**：自建模态没有统一的 isDanger 危险样式（Settings.vue:17 里手写了 red-text-btn，但 Rules/Subscriptions 的删除确认没有）、没有 ESC/遮罩统一语义（Settings.vue:17 点击遮罩即取消；SettingsUpdate.vue:40 点击遮罩等于取消内核更新）、也不参与 F-A4-01 的排队机制。
- **影响**：危险确认的视觉与交互不一致；两套系统同时存在时，自建模态不会被全局模态覆盖（视觉上重叠渲染，App.vue:155 的 overlay 与组件内 overlay 同为 fixed 定位），出现双层遮罩。
- 修法：统一走 showConfirm（Settings.vue:210 改为 const ok = await showConfirm(t(settings.resetConfirmMsg,{name}), t(settings.resetConfirmTitle), true); if(!ok) return;，删除 :17 的 template 与 showResetConfirm）；Rules.vue:69 的「新增规则」是表单弹窗，保留但抽成 components/DialogShell.vue（统一 z-index/ESC/焦点陷阱）；SettingsUpdate.vue:40 的遮罩点击保持显式 cancelCoreUpdateConfirm()，但需在 UI 上写明「点击空白处=取消」。

#### F-A4-08 启动链把重调用串在「事件订阅」之前，且 GetRecentLogs 无兜底 → 订阅可能整段丢失

- 严重度：**P1**；证据等级：A（顺序）+ B（耗时）
- **file:line**：App.vue:614-701：:641 await App.CheckTunEnv() → :646 await App.CheckCoreAgainstSeed() → :676 const history = await App.GetRecentLogs();（无 try/catch）→ :679 StartStreamingLogs → :681 log-message 订阅 → :693 clash-exited → :704 起 app-update 系列；对比 store.ts:733-742（initStore 内部对 GetAppState 有 try/catch）
- **现象**：所有事件订阅都在 676 之后注册。若 GetRecentLogs reject，onMounted 的 async 函数直接中断（未捕获的 rejection），:679-838 全部不执行：日志页永远空白、clash-exited 不处理（内核退出后 UI 仍显示运行中）、应用更新事件全部收不到。
- **影响**：单点失败造成整块功能静默失效，且符合「日志为空 → 无法复盘」这一事故特征；冷启动时 CheckTunEnv（GetStatus + Mips）本身就可能数秒，进一步拉长「UI 已可点但后台事件未接管」的窗口。
- 修法：把 :656-838 的所有 Events.On 与 startWaveSampling/StartStreamingLogs 提到 :634 之前（订阅先于任何 await）；:676 包 try/catch 并在失败时 showToast + console.error；:641/:646 两个重调用改为不 await 的后台函数（内部 catch），或至少 Promise.allSettled 并行以减半串行时间。

#### F-A4-09 SettingsTun 首开页面并发两次全量 GetStatus，且后一次覆盖前一次的错误信息

- 严重度：**P1**；证据等级：A
- **file:line**：SettingsTun.vue:108-112（refreshRuntimeAssets → App.GetRuntimeAssetStatus，内部写 tunStatus.hasWintun/wintunError）、:118（Promise.all([refreshRuntimeAssets(), App.CheckTunEnv(), App.GetTunConfig()])）、:119（tunStatus.value = status ?? {} 整体覆盖）；后端成本：app.go:897-899（GetRuntimeAssetStatus → runtimeassets.GetStatus）、app.go:727-738（CheckTunEnv → GetStatus + CheckHelperService + CoreSupportsMips）；GetStatus 无缓存：core/runtimeassets/status.go:19-42，check.go:128（每次 SHA256 61 MB）、check.go:302（4 个 geo 各算一次）。
- **现象**：进一次设置→TUN 触发两个并发 GetStatus（约 180 MB 读取 + 2 次 -v），且 :119 用 CheckTunEnv 的 map 覆盖 :111 写入的 wintunError 与完整 wintun 对象（CheckTunEnv 返回的 wintun 字段名与 refreshRuntimeAssets 期望的一致，但 ready/hint 语义不同源，:111 的 wintun 不可用 兜底文案会被冲掉）。
- **影响**：页面首屏 loading 期间用户看不到任何进度；重复读放大 IO（冷启动/杀软首扫时最明显）；驱动错误文案丢失后，:23 的红色状态可能显示成默认的「驱动不可用」而不是真实原因。
- 修法：:118 改为 await Promise.all([App.CheckTunEnv(), App.GetTunConfig()]) 并删除 refreshRuntimeAssets() 调用（CheckTunEnv 已含 wintun）；如需 error/hint，改从 CheckTunEnv 返回的 wintun 对象取。:119 改为合并而非覆盖：tunStatus.value = { ...tunStatus.value, ...(status ?? {}) }。

#### F-A4-10 代理页「全部测速」失败完全静默；单点测速用 includes(busy) 吞掉任意含 busy 的错误

- 严重度：**P1**；证据等级：A
- **file:line**：Proxies.vue:660-684（:671-679 catch 只复位 isTesting/frozen/nodes.testing，无 console.error、无提示）、Proxies.vue:687-704（:694-698 msg.includes(DELAY_TEST_BUSY) || msg.includes(已有测速任务) || msg.includes(busy) → return 静默）；后端生产端仅 core/appcore/delay_manager.go:15（fmt.Errorf(DELAY_TEST_BUSY)）
- **现象**：点「测速全部」→ 后端拒绝（例如内核未启动、测速任务冲突、API 不可达）→ 界面按钮恢复原状、卡片不转圈、无任何文案；单点测速遇到任何含小写 busy 的错误（如 proxy api busy）也被吞。
- **影响**：用户反复点击同一按钮；日志里也没有痕迹（这是主方案 §11.3⑩ 所谓全盲区在关键路径上的实例）。
- 修法：Proxies.vue:673-679 的 catch 改为 const u = toUserError(e); showToast(t(proxies.speedTestFailed,{error:u.message}), error)（需补 i18n key）+ console.error；:696-698 改为 if (codeOf(e) === DELAY_TEST_BUSY) { showToast(t(proxies.delayBusy),info); return; }，其余走 toUserError + toast；toUserError 目前无 code 解析（utils/errors.ts:43-54 只有正则），需同步补 parseCode。

#### F-A4-11 完全静默的失败面比主方案统计的 28 处 console.error 更大：另有 9 处零痕迹

- 严重度：**P1**；证据等级：A
- **file:line**：.catch(()=>{}) 5 处 —— SettingsTun.vue:162（InstallTunDriverAsync）、SettingsUpdate.vue:83（CheckCoreUpdateAsync）、85（UpdateCoreComponentAsync）、86（InstallTunDriverAsync）、Subscriptions.vue:321（UpdateAllSubsAsync）；catch(e){} 2 处 —— App.vue:638（GetAppVersion）、Proxies.vue:531（GetAppBehavior/colorDelay）；catch{} 1 处 —— App.vue:624（GetAppBehavior + syncThemeFromBehavior）；另有 console.log 冒充吞错 —— Subscriptions.vue:358（Import cancelled，把导入失败与用户取消混为一谈）
- **现象**：这些 catch 既不提示、也不写日志（前端无落盘通道），失败只存在于被清空的 Promise 链里。
- **影响**：主方案 §11.5 按 console.error 计数得 28，但「无痕失败」实际至少 28+9=37 处；若按 §7#37 只替换 28 处，仍留 9 个盲区（其中 SettingsTun 装驱动、内核更新、订阅全量更新都是用户主动动作）。
- 修法：先落一个 reportError(e, ctx)（新增 frontend/src/utils/report.ts：console.error + 调用后端 App.LogAppError(level, ctx, msg)（如无此方法则由 A2 补）+ 按 class 决定 toast）；替换上述 9 处与 28 处 console.error；Subscriptions.vue:358 改为区分「用户取消（不记录）」与「失败（reportError）」，判据用返回的 path 为空而非 catch。

#### F-A4-12 成功反馈走模态：10 处成功提示违反 store.ts 自己写的策略

- 严重度：**P1**（打断频率高，直接对应主方案 §9.1「模态数=0」）；证据等级：A
- **file:line**：策略声明在 store.ts:900-904；违反点：Rules.vue:407（addSuccess）、Settings.vue:219（resetSuccess）、SettingsUwp.vue:180/204（saveSuccess）、SettingsUpdate.vue:87（helperInstallSuccess/helperInstallPending）、88（helperRestartSuccess）、96（coreUpdateSuccess）、Subscriptions.vue:331（updateSingleSuccess）、506（updateAllSuccessMsg）、TrafficCard.vue:156（resetSuccess）、SettingsAbout.vue:456（诊断导出成功，且硬编码中文）；对照已正确使用 toast 的仅 6 处（Overview.vue:188、Settings.vue:101、SettingsTheme.vue:657、SettingsAbout.vue:382/434、store.ts:476）
- **现象**：成功＝模态，失败（LAN/网络）＝无声，与「成功用 toast、错误用模态」完全相反。
- **影响**：用户每做一次成功操作就要点一次「我知道了」；模态成为噪声源，反过来掩盖真正的错误模态（用户形成无脑点掉的条件反射）。
- 修法：逐个替换为 showToast(t(...), success)（上列 10 处）；SettingsUpdate.vue:87 的 helperInstallPending 属 warn → showToast(..., info) + 在 helper 行内显示状态（:26 已有 helperStatus 渲染）；同时补 store.ts:907-926 的 toast 支持 action 按钮（主方案 §5.3）。

#### F-A4-13 i18n 槽位错位：把原始后端错误塞进 title；同一错误在标题和正文各出现一次

- 严重度：**P1**（用户直接看到 dial tcp/中文路径等原始文本，且标题被撑爆）；证据等级：A
- **file:line**：Rules.vue:234（await showAlert(String(e), t(rules.loadOptionsError, { error: String(e) }))：message 为原始错误，title 为「加载规则选项失败: 原始错误」）、Rules.vue:409（同型：addError 当 title）、Overview.vue:350（showAlert(t(overview.errSwitchMode,{error:String(err)})) 把多行原始错误塞进提示句）、Settings.vue:72（t(common.error) + 冒号 + (e?.message||e)，title 仍是 common.error）、SettingsAbout.vue:449（同型）、SettingsUwp.vue:98（同型）、SettingsUpdate.vue:87（helperInstallPending + 换行 + helperStatus.error 拼在 message 里）
- **现象**：showAlert(message, title, isDanger, detail) 的四个位参被误用：本该 detail 承载原始文本（store.ts:472-473 的注释明确定义了 detail），却塞进 message/title。
- **影响**：模态标题行被长错误撑开（App.vue:158-161 的 h3 无截断）、错误文本无法复制（detail 区才适合等宽展示）、翻译者无法翻译动态错误。
- 修法：Rules.vue:234 改为 const u = toUserError(e); await showAlert(t(rules.loadOptionsError), t(common.error), true, u.detail)；:409 同改（不把 error 传进 title）；Overview.vue:350 改为 await showAlert(t(overview.errSwitchMode), t(common.error), true, u.detail)；Settings.vue:72、SettingsAbout.vue:449、SettingsUwp.vue:98 统一改用 toUserError + detail；同步检查 locales 中所有 Error/Failed 的 {error} 占位（正文句子里保留合理，但 title 里不能有）。

#### F-A4-14 前端用中文字面量正则判断后端语义，后端改文案即静默降级

- 严重度：**P1**；证据等级：A
- **file:line**：store.ts:467-478（LONG_CONNECTION_DEFERRAL_RE = /由于长连接活动而延迟|由于长连接一直存在，已放弃/ → 命中转 toast，否则走模态）；生产端：core/appcore/controller_long_connection.go:37（由于长连接活动而延迟 …）、:172（由于长连接一直存在，已放弃延迟执行的重启任务 …）
- **现象**：判定完全依赖 Go 侧中文格式串；同一机制在 store.ts:663（result?.message?.includes(路由切换中) ← core/appcore/outbound_ip.go:205）也是同型。
- **影响**：任何文案微调（加空格、改标点、未来 i18n 化）都会让「本该静默」的延迟事件退回成模态弹窗，且失败方式是完全静默（用户不会知道判定失效，只看到弹窗变多）。这正是主方案 §5.2 要消灭的模式，但主方案未把这两处列入清单。
- 修法：后端 controller_long_connection.go:37/172 已带事件名（c.notify 第二参有 DEFERRED_* code，见 :172 的 DEFERRED_RESTART_ABANDONED）：改为在错误/通知里透出 code，前端 store.ts:467-478 改为 if (notification.code?.startsWith(DEFERRED_)) showToast(...)；store.ts:663 改为读 result.code === ROUTE_SWITCHING（需后端 outbound_ip.go:205 增加 Code 字段）。

### P2

#### F-A4-15 code 契约不一致：前端匹配的字符串里，有 3 个后端永远不会产出

- 严重度：**P2**；证据等级：A
- **file:line**：① Overview.vue:233/289 msg.includes(ErrNoActiveConfig) —— Go 里 ErrNoActiveConfig 只是标识符（core/appcore/supervisor.go:17 的正文是 no active config selected），错误文本里不会出现该名字（唯一出现是 controller.go:412 的 errors.Is 判断），该分支是死代码；② SettingsAbout.vue:458 String(error).indexOf(User cancelled) === -1 —— 仓库内不存在该字符串，且后端 controller.go:2444-2462 已把用户取消归一化为 nil（cancelled by user 被吞），因此该判断既不会命中也不需要；③ Proxies.vue:696 includes(已有测速任务) —— 同样不存在，实际只有 DELAY_TEST_BUSY（delay_manager.go:15）
- **现象**：防御性猜测被当成契约，读代码的人会误以为后端会返回这些串。
- **影响**：维护成本 + 误导后续改造（若按这些串写 code 映射会漏掉真实 code）。
- 修法：删除 Overview.vue:233/289 的 ErrNoActiveConfig 备用分支、SettingsAbout.vue:458 的 indexOf 判断、Proxies.vue:696 的后两个 includes；在后端补 apperr 时以「实际产出字符串」为准建表（当前实际只有 5 个：no active config selected / helper_install_required / helper_repair_required / wintun_missing / DELAY_TEST_BUSY）。

#### F-A4-16 硬编码文案绕过 i18n（8 语言下固定显示中文）

- 严重度：**P2**；证据等级：A
- **file:line**：SettingsAbout.vue:456（showAlert(诊断信息导出成功)）、:459（showAlert(导出诊断信息失败: 加错误)）；SettingsTun.vue:111（wintun 不可用 兜底串）；Proxies.vue:388（Unknown）
- **现象**：非中文语言下这两处仍是中文；且 :459 不区分 class（失败用 isDanger 默认 false）。
- 修法：为 settings.about.diagnosticsExportSuccess / diagnosticsExportFailed 增加 8 语言 key（zh-CN.ts 等 8 文件 settings.about 段），替换 :456/:459；:459 改为 toUserError + showAlert(u.message, t(common.error), true, u.detail)。

#### F-A4-17 locale 键集：8 语言 777 键完全对齐，但存在 1 个缺键、14 处日文未翻译、141 个死键

- 严重度：**P2**；证据等级：A（程序化比对）
- **file:line**：键集比对结果 8 语言各 777 键、无缺失/多余；缺键：SettingsUwp.vue:151 使用 t(settings.uwp.clearAllConfirm)，该 key 在 8 个语言包中均不存在（代码在 :152 手写回退到 diag.clearAllConfirm，属于「用代码补翻译」）；未翻译：ja-JP 有 14 个值与 zh-CN 完全相同且含中文（common.save=保存、common.success=成功、common.warning=警告、window.min=最小化、overview.statusStopping=停止中...、subs.updating=更新中...、subs.updateSuccessTitle=更新成功、yamlEditor.saving/saveBtn/unsaved、settings.about.diagnosticsNormal 等），en-US/ru-RU/de-DE/fr-FR 各 3 个相同值但均为非中文（如 GeoIP），属正常；死键：141 个已定义键在代码中从未使用，其中包含错误类文案 overview.restartError、overview.errToggleProxy、overview.errToggleTun、rules.loadError、subs.updateError —— 说明「错误文案早就翻译好了，代码却用裸字符串」
- 修法：① 删除 SettingsUwp.vue:150-153 的兼容层，把 settings.uwp.clearAllConfirm 补进 8 个语言包；② 补 ja-JP 的 14 处；③ 用 overview.errToggleProxy/errToggleTun 替换 Overview.vue:237/312 的通用 toUserError 文案（错误分类后按 code 选 key）；④ 其余死键纳入 i18n lint（可用现有 locales/index.ts 的 t() 兜底返回值等于 key 做开发模式 console.warn）。

#### F-A4-18 app-notification 过滤规则过窄，后台失败被整段丢弃

- 严重度：**P2**；证据等级：A（代码）+ B（实际丢弃量）
- **file:line**：store.ts:749-757（只处理 source 属于 {tray, startup, deferred-action} 且 level 为 error 且 message 非空）；对照 controller_long_connection.go:172 的 notify(error, DEFERRED_RESTART_ABANDONED, …, deferred-action) 与 :177 的 notify(success, DEFERRED_RESTART_RESUMED, …, deferred-action)
- **现象**：source 为其它值（如 core/update/config）的 error 通知完全不进 UI，也不进日志（前端无落盘）；success 级通知（如延迟任务恢复）被丢弃，用户以为操作丢了。
- **影响**：后台自愈/延迟动作的结果不可见，与「可回放」目标冲突。
- 修法：store.ts:749 改为按 class 路由：const cls = classOf(notification.code)；C→showAlert，B→showToast + 日志角标，A→只 reportError；success 级一律 showToast（:177 的 DEFERRED_RESTART_RESUMED 正好该显示）。

#### F-A4-19 订阅导出：「取消」路径也会执行导出，且订阅 URL 明文进模态 detail

- 严重度：**P2**；证据等级：A
- **file:line**：Subscriptions.vue:432-450（:445-448 onCancel: async () => { globalState.modal.show = false; await doExport(config.id, false); }）、:410-430（分享：detail 为 config.url 明文）
- **现象**：导出确认框的「取消」不是取消，而是「以不使用自定义规则的方式导出」；点击遮罩同样触发 onCancel（App.vue:155 的 @click.self 对应 handleModalCancel）。
- **影响**：用户以为取消了，实际多出一个导出文件；订阅 URL（含 token）显示在模态详情里，截图/共享时易泄露。
- 修法：Subscriptions.vue:434 改用 store 的 showConfirm（确认=带规则、取消=不导出），或新增 showChoice(title, message, [{text, value}])；分享 :424 用脱敏后的 URL（保留首尾，中间打码），与主方案 §6.5 的 redact 一致。

#### F-A4-20 UWP「全不选」实际是逐项取反

- 严重度：**P2**（非提示面，但同属静默改变用户数据）；证据等级：A
- **file:line**：SettingsUwp.vue:140-146（:144 uwpApps.value.forEach(app => app.isEnabled = !app.isEnabled)）
- **现象**：点击「全不选」后，原本已关闭的应用会被打开，原本打开的被关闭；保存后按差异提交，等于随机改了一部分。
- 修法：:144 改为 app.isEnabled = false；并在保存前把「全部取消」路径（:172-187 的 ClearAllUwpExemptions）与「全不选」按钮语义对齐。

#### F-A4-21 偏差条把「拉取失败」当「无偏差」

- 严重度：**P2**；证据等级：A
- **file:line**：DeviationStrip.vue:63-80（:69-72 catch 内 list.value = []，随后 :74-79 用空列表算签名 → dismissed 复位逻辑仍按空跑）
- **现象**：GetConfigDeviations 失败时，配置「保存了但不生效」的提示直接消失，且无任何日志。
- **影响**：用户以为配置已正确生效；与主方案「B 类需干预但不阻塞」的定位相反（这里变成了完全静默）。
- 修法：:69-72 改为 catch (e) { reportError(e, deviation.refresh); list.value = lastGood ?? []; }（保留上一次结果，别清空），并在 UI 上加一个低调的「无法读取偏差」小标记（复用 :19 的展开区域）。

#### F-A4-22 短命令缺少错误分支（失败即无提示）

- 严重度：**P2**；证据等级：A
- **file:line**：TrafficCard.vue:151-157（await App.ResetTrafficTotals(); resetWaveState(); showAlert(成功) 全裸）、App.vue:541-549（handleClose 里 await GetAppBehavior 无 catch → 关窗失败无反应）、App.vue:693-695（clash-exited 里 App.SyncState() 无 catch）、store.ts:118-129（setThemeMode 用 console.warn 吞）
- **现象**：这类「短命令」普遍不写错误分支。
- 修法：TrafficCard.vue:154 包 try/catch（失败 showToast(...,error) 且不 resetWaveState）；App.vue:543 包 try/catch 并把失败降级为「直接隐藏窗口」；App.vue:695、store.ts:127 改走 reportError。

#### F-A4-23 设置页导航到 UWP 的「取消提权」仍会进入页面（提示与行为不一致）

- 严重度：**P2**；证据等级：B（是否刻意无注释说明）
- **file:line**：Settings.vue:61-84（:63-75：非管理员 → showConfirm；:68 if (restart) {…return;}；:77 之后无论如何 view.value = uwp）
- **现象**：确认框文案问「是否以管理员身份重启」，点「取消」后仍然进入 UWP 页（普通权限）。文案 adminPromptMsg 里确实写了「点击取消可继续在普通权限下查看」，属有意设计。
- **影响**：语义可接受，但按钮标签用的是 common.cancel（取消），与非管理员用户的预期（取消=不进入）有偏差。
- 修法：Settings.vue:63-67 增加 cancelText 参数（需 showConfirm 支持自定义按钮文案，与 F-A4-02 的 showModal 一起做），把取消按钮文案改为「普通权限查看」。

---

## 3 与主方案对应关系

### 3.1 主方案已覆盖、本次独立确认成立的

| 主方案条目 | 本次确认 |
|---|---|
| §1.4 Overview.vue:252-319 任何 TUN 错误三分支→模态 | 成立（Overview.vue:267-313），并补充：真正的漏判在大小写（F-A4-04） |
| §1.4 SettingsTun.vue:142-156 与 Overview 两套逻辑 | 成立（F-A4-03），补充：SettingsTun 还缺 pending 闸门与状态回灌 |
| §1.4 store.ts:749-777 通知只处理 3 个 source→模态 | 成立（F-A4-18），补充：success 级也被丢 |
| §1.4 store.ts:474-507 两套模态、错误路径几乎不用 toast | 成立（F-A4-12：toast 仅 6 处），但更严重：两套模态共享单一槽位会互相顶掉（F-A4-01，主方案未提） |
| §11.3⑦ Overview/SettingsTun 两套 TUN 开关 | 成立（F-A4-03） |
| §11.3⑧ Overview.vue:233 vs :289 复制判定 | 成立，且两处都含死分支 ErrNoActiveConfig（F-A4-15） |
| §11.3⑨ Proxies.vue:696 三连 includes | 成立（F-A4-10），补充：includes(busy) 会吞掉非测速类 busy 错误 |
| §11.3⑩ 28 处 console.error 全盲区 | 成立（F-A4-11），但计数不足：另有 9 处零痕迹 |
| §7#5/#6/#34/#35/#37 改造项 | 方向正确，行号基本准确 |
| §11.6「8 语言文案与 code 对齐 未审」 | 本次补齐（F-A4-17）：键集完全对齐，但缺 1 键、ja-JP 14 处未译、141 死键 |
| §1.2-B/C 判定与文案脱钩的大方向 | 成立 |

### 3.2 主方案遗漏、本次新增（不在 v2 任何章节）

| 新增点 | 位置 | 报告条目 |
|---|---|---|
| 单一模态槽导致 confirm Promise 悬挂、pending 永久卡死 | store.ts:474-507 + Overview.vue:253/258/314 | F-A4-01（P0） |
| 10 处直接赋值 globalState.modal（第三套模态实现） | App.vue 8 处 + Subscriptions.vue 2 处 | F-A4-02（P0） |
| mips→mixed 自动回退从未落盘、每次进页重复弹窗 | SettingsTun.vue:123-127 + 167-168 | F-A4-05（P0） |
| 设置自动保存四套反馈，LAN/网络两页完全静默 | SettingsLan.vue:268、SettingsNetwork.vue:338 | F-A4-06（P0） |
| SettingsTun 首开并发 2×GetStatus 且结果互相覆盖 | SettingsTun.vue:118/119 | F-A4-09（P1） |
| 代理页「全部测速」失败无提示无日志 | Proxies.vue:671-679 | F-A4-10（P1） |
| 9 处零痕迹 catch（.catch(()=>{}) 等） | 见 F-A4-11 清单 | F-A4-11（P1） |
| 成功反馈走模态 10 处 | 见 F-A4-12 清单 | F-A4-12（P1） |
| i18n 槽位错位（错误进 title） | Rules.vue:234/409、Overview.vue:350 等 | F-A4-13（P1） |
| 中文正则耦合（长连接/路由切换中） | store.ts:467-478、663 | F-A4-14（P1） |
| 启动链事件订阅在重调用之后 + GetRecentLogs 无兜底 | App.vue:641/646/676 vs 681-838 | F-A4-08（P1） |
| 8 处自建模态与全局模态并存、重置两种确认 UI | Settings.vue:17 vs SettingsTheme.vue:634 | F-A4-07（P1） |
| locale 缺键/未译/死键 | locales × 8 | F-A4-17（P2） |
| 订阅导出「取消」也导出、URL 明文进模态 | Subscriptions.vue:434-449/420-429 | F-A4-19（P2） |
| UWP 全不选取反 | SettingsUwp.vue:144 | F-A4-20（P2） |
| 偏差条把失败当无偏差 | DeviationStrip.vue:69-72 | F-A4-21（P2） |

### 3.3 主方案说错 / 需要修正的地方（逐条）

| # | 主方案原文 | 实际情况 | 证据 |
|---|---|---|---|
| 1 | §11.2「前端 showAlert 67」 | 69 处调用、分布在 66 行（SettingsUpdate.vue:87 一行 3 处、:88 一行 2 处）。差异来源是按「行」还是按「调用」计。 | grep 全量计数（见 §2 口径） |
| 2 | §11.2「前端 showConfirm 18」 | 17 处（全部为 await 调用；不存在第 18 处，除非把 App.vue 的直写 modal 也算进来，那应是 10 处而非 1 处） | 同上 |
| 3 | §11.2「前端 showToast 7」 | 6 处（store.ts:476、Overview:188、Settings:101、SettingsTheme:657、SettingsAbout:382/434） | 同上 |
| 4 | §11.2「console.error 吞错 28」 | 28 是 console.error 的全部出现次数（含 store.ts:197 的 localStorage 失败、UpdateTaskPanel.vue:34 的清理失败等非用户路径）。真正的无痕失败约 37 处（+9 零痕迹） | F-A4-11 |
| 5 | §11.2「前端错误文案匹配 .includes( 6 处」，并列出 5 行 | ①它列的是 5 行不是 6；②这 5 行内共有 11 个 includes（Overview.vue:233/267/289/291 各 2，Proxies.vue:696 有 3）；③漏了 3 处：store.ts:663（includes 路由切换中）、store.ts:467-478（中文正则）、SettingsAbout.vue:458（indexOf User cancelled）；④另加 utils/errors.ts:12-19 的 8 条正则。合计判定点 9 文件 15 处 | F-A4-14/15、§0 |
| 6 | §1.2-C「Go 侧只有两处会产生这种文案（controller.go:1778、supervisor.go:167）… 一旦上游把别的错误包装进来，前端就无法区分缺驱动和驱动被占用，统一弹模态」 | 方向反了：includes(Wintun) 大小写敏感。被包装后的高频错误是 status.go:69 的小写 wintun 仍不可用，经 controller.go:661 包成 运行组件不可用: wintun 仍不可用: … → 匹配不到，落到通用分支（不是 wintun 分支）。所以现状不是「所有 wintun 错误都误报成缺驱动」，而是「真正的缺驱动/驱动异常有一半漏判，用户拿不到装驱动入口」 | F-A4-04；controller.go:661、status.go:69 |
| 7 | §11.3⑤「helper_client_windows.go:36-43 RPC 默认 deadline 10s → CheckHelperService 的 Ping 就挂在 UI 链上，服务假死会让设置页卡 10s」 | Ping 不是 10s：getMethodTimeout 对 ping 返回 200ms，连接超时 helperConnectTimeout=150ms；10s 只对未列出的方法（default 分支）生效。CheckTunEnv/GetHelperServiceStatus 的实际最坏约 350ms，真正的成本仍在 GetStatus（61 MB 哈希 ×N） | core/sys/helper_client_windows.go:20、27-44、187-197 |
| 8 | §11.3⑫「core 日志逐条 emit → 前端逐条 push + 自动滚动」 | 成立；补充前端事实：App.vue:681-691 已用 100ms 节流滚动（scrollTimer），但数组仍是逐条 push 且只保留 500 条（:683），所以噪声问题在「500 条环被冲掉」而非「滚动卡顿」 | App.vue:681-691 |
| 9 | §1.4「App.vue:698-700 mips fallback → 模态」 | 成立，但同一事件还有第二处：SettingsTun.vue:126（进页即弹），主方案漏；且该「回退」根本没保存（F-A4-05） | SettingsTun.vue:123-127 |
| 10 | §11.6「前端提示面 95% A（67 处 alert 已计数，未逐条读实现）」 | 本次逐条读完 32 条链路、69 调用点；发现计数口径错误（#1/#2/#3）与 1 个 P0 状态机缺陷（F-A4-01），说明「已计数」不能替代「已读实现」 | 本报告 §1/§2 |
| 11 | §5.3 前端改造点只列 6 行（Overview/SettingsTun/store 通知/toast/errors.ts/App.vue:698） | 实际改造面是 32 条链路 + 10 处直写 modal + 8 处自建模态 + 5 个设置页保存反馈 + 10 处成功模态 | §1 |
| 12 | §11.6「提权 / UWP / 开机自启 50% C」 | UWP 页本次读完：除 F-A4-20 取反 bug 外，还有「保存成功用模态」与「清空全部走独立确认」两处一致性问题 | SettingsUwp.vue:140-210 |

结论：主方案的方向与 P0 优先级判断仍然成立（文案匹配必须换成 code、模态必须收敛、后台事件不能无条件弹窗），但它对前端的三组数字（67/18/7）与实际不符，对「wintun 误报面」的机理描述反了，对 helper Ping 的成本高估了一个量级；同时还漏掉了本报告 F-A4-01（P0）、F-A4-05（P0）、F-A4-06（P0）这三处用户可直接感知的缺陷。

---

## 4 未覆盖与待实测

### 4.1 本次未覆盖（诚实清单）

| 项 | 原因 | 补齐成本 |
|---|---|---|
| 模态 a11y：焦点陷阱、ESC 关闭、键盘 Tab 顺序、aria-live | 本次聚焦错误语义，未做无障碍专项 | 2 h |
| Sidebar.vue / ModernSelect.vue / ModernNumberInput.vue / UpdateTaskRow.vue / EmptyState.vue 全文 | grep 确认无提示面调用，未逐行读 | 1 h |
| utils/theme.ts（385 行）、glass.ts、format.ts、trafficWaveState.ts | 与提示面无关 | 1 h |
| styles/*.css 中 .modal-overlay 的层级/z-index 冲突（自建模态叠加是否真的双层） | 需运行验证；本次只从 DOM 结构判断（F-A4-07） | 0.5 h + 实机 |
| 8 语言译文质量（术语一致性、长文案溢出） | 只做了键集与「是否等同中文」的程序化检查 | 每语言 1 h |
| 前端日志落盘通道（当前前端完全没有落盘，reportError 需要后端配合） | 属 A2/A3 范围（后端 logger 落盘） | 依赖后端 |
| Wails v3 下 GetStatus 并发是否真的并行 | 后端 GetStatus 无锁，但 ReadCoreVersion 持 coreVersionCacheMu 跨 -v（check.go:144-197），两路会串行；SHA256 部分并行 → 磁盘竞争。未实测 | 30 min（实机计时） |

### 4.2 待实测（需运行应用，本次禁止）

| 编号 | 待测项 | 判据 | 影响结论 |
|---|---|---|---|
| T-1 | F-A4-01 的模态悬挂 | 在 Overview 点 TUN 触发 helper 确认框的同时，让后台更新检查完成（或手动触发 app-update-available），观察：确认框是否被替换、点击后 TUN 卡片是否永久无响应 | 决定 F-A4-01 是否 P0 维持 |
| T-2 | 冷启动首屏到「事件全部接管」的时延 | 在 App.vue:676 前后加临时计时（只读审计不能改码，需开发配合），或从日志时间戳反推 | 决定 F-A4-08 的实际用户影响 |
| T-3 | 设置→TUN 首开耗时（双 GetStatus） | 用 A0 的方法计时（warm/cold 各 5 次） | 校准 §1 第 9 行的单次成本 |
| T-4 | CheckTunEnv 的 Ping 实测最坏耗时 | helper 未启动/服务假死两种状态下计时 | 反向验证 §3.3 第 7 条（预期 ≤350ms） |
| T-5 | 模态叠加是否真的双遮罩 | 打开 Subscriptions 删除确认的同时触发 app-update-error | 决定 F-A4-07 的严重度 |
| T-6 | mips 回退未落盘（F-A4-05） | 手动把 tun.stack 改成 mips，进设置页看到提示后退出重进，检查 config.yaml 是否仍为 mips | 若配置确实未变，F-A4-05 维持 P0 |
| T-7 | LAN/网络静默保存（F-A4-06） | 把 mixedPort 改成被占用端口并保存，观察是否有任何提示 | 决定是否升级为 P0 |

### 4.3 给主方案的落地建议（前端侧，按依赖排序）

1. 先做 store 层（F-A4-01/02/07）：modal id + 队列 + 禁止直写。这是所有其他改造的地基——不做这个，后面每加一个 code 分支都可能制造新的悬挂。
2. 再做 code 通道（F-A4-04/14/15）：后端 apperr 落地后，前端 utils/errors.ts 增加 parseCode()，Overview/Proxies/SettingsAbout 的 15 处字符串判定一次性删除。
3. 再做反馈分级的批量替换（F-A4-06/10/11/12）：saveWithFeedback + reportError + 成功改 toast 三件套，能一次消掉 3 个 P0/P1。
4. 最后做成本侧（F-A4-08/09）：等后端 GetStatusCached 就绪后，前端所有进页调用改成 await 一个轻接口；启动链的订阅顺序改造不依赖后端，可立即做。

---

*本报告仅新增 docs/audit/A4-frontend.md 一个文件；仓库内其它文件均未被修改（未执行 git 命令，未写入除本文件外的任何路径）。*
