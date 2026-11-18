# 独立评审 · 编号 2（组件层：components/*.vue + useTunToggle + settings/manifest）

> 评审代理：reviewer-ui（只读评审，未改任何源码）
> 基线：工作区未提交改动对比 HEAD（含 Wave1/2/3 全部修复）
> 范围：frontend/src/components/ 下全部 .vue（App.vue 除外，27 个）+ composables/useTunToggle.ts + settings/manifest.ts
> 方法：git diff -U6/-U8 逐文件过改动 → 对可疑处读**改后完整上下文** → grep 核对调用方/引用方/i18n 键/事件名/绑定方法 ID。未运行 build / vue-tsc（按要求）。
> 结论：**BLOCKER 0 · MAJOR 1 · MINOR 10**；关键不变量（C-02 / H-03 / G-04 / P-05 / ModernSelect 挂载期不落盘 / 样式 token / 死代码清理 / alertError·storage 收敛）逐条核对通过，见 §2。

## 1. 问题清单

| 级别 | 位置 | 问题 | 影响 | 建议 |
|---|---|---|---|---|
| **MAJOR** | ModernNumberInput.vue:101-124（`cancelEdit` / `onCancelKey` / `onBlur`） | Esc「取消本次输入」**并不能取消**：键入时 `onInput` 已经把新值 `emit('update:modelValue')` 出去；`onCancelKey` 只做 `el.value = String(props.modelValue)`（此刻 modelValue 就等于新值，回填是空操作）+ `el.blur()`，而 `onBlur` 命中 `cancelEdit` 分支后**直接 return，不做 clamp、不发 change**。 | ①注释与 G-07 条目宣称的「还原」不存在，用户按 Esc 以为撤销、实际值仍是刚输入的；②**绕过 clamp 的数据通路**：输入越界值（如 autoDelayTestInterval 输 99999，max=1440）后按 Esc，blur 的 clamp 被吞掉，越界值留在 model 里；Go `normalizeBehavior` 只夹下界（behavior.go:497/501 仅在 <=0 时纠正），上界不夹 ⇒ 之后改任意另一个开关触发保存，越界值会被真落盘。 | Esc 分支改为「按 lastValid（或 clamp 后的值）emit 一次 update:modelValue（不发 change）」再 blur，让“取消”真的回退；或在 `onCancelKey` 里同时 `emit('update:modelValue', lastValid.value)`。 |
| MINOR | Rules.vue:502-525 | `deletingRule` 闸门在 `await showConfirm()`（:513）**之后**才置位（:524），但注释 :503 写「删除期间互斥……也顺手挡住双击」。模态是队列（store.ts:646-648 `if (activeModal) modalQueue.push(entry)`），双击删除按钮会排**两个确认框**。 | 双击=两次确认框；第二个确认后因 `indexOf < 0` 走「规则列表已变化」提示自行取消，**不会删错**（P-01 的文本定位兜住了），但用户会看到两个一样的确认框，且与 H-03/P-23 的「闸门早于确认框」原则不一致。 | 把 `deletingRule = true` 提到 `showConfirm` 之前，取消/失败路径在 `finally` 复位。 |
| MINOR | ModernSelect.vue:54-63（G-08） | 挂载期自动纠错只删了 `emit('change')`，但 **`emit('update:modelValue', options[0].value)` 依旧把第一个选项写进父级 model**。父级普遍 `v-model="behavior.xxx"`，该字段立刻与 `loadedSnapshot` 产生差异、成为「本地脏字段」，下一次任意保存（G-04 的 payload）就会把纠错值静默落盘。 | G-08 只把「一进页面就写盘」推迟成「下次保存时写盘」，审计要求的「不再静默改配置」未完全达成；fix-5-settings.md §4-3 的「behavior 真实值仍是旧值，直到用户手动选一次」与实现不符（值已被改写）。 | 二选一：纠错只改**显示**（内部 displayValue，不 emit）；或父级提供纠正回调并同步基线。至少把注释/报告改成“改写到 model 但不落盘”。 |
| MINOR | Proxies.vue:658-676 | `loadSeq` 的 `if (seq !== loadSeq) return` 是**不可达分支**：`loadRunning`（:664-667）已保证同一时刻只有一个 loadData 在跑，循环内 `seq = ++loadSeq` 必然等于当前 loadSeq。注释 :675「双保险：已有更新的一次请求在外」不成立。 | 死代码 + 误导性注释，让人以为这里挡了并发；真正的去重是 `loadAgain`。 | 删掉 loadSeq/seq（或换成能达的判据），注释改成“在飞去重靠 loadAgain”。 |
| MINOR | Proxies.vue:158-162、637-645 | P-16 失败态判据 `!!String(data.activeConfig || '')` 几乎恒真（激活配置基本不会是空串），于是「有激活配置但解析出 0 个分组」一律显示 `proxies.loadFailedHint`（读取失败）。纯规则 + DIRECT 的合法配置、或离线配置里没有 Selector/URLTest 组时会被误报。 | 空分组页文案由「请先添加配置」变成「读取失败」（fix-3 报告风险 2 已自认，但归因写成“需要后端字段”，前端本可先收紧）。 | 判据先收紧为 `isOffline && isRunning`（真失败分支）或“连续两次 0 组”才报失败；根治仍需后端 `loadError` 字段。 |
| MINOR | Proxies.vue:442-461 | `measuredWidthCache` 只增不减（key=字体+文本），切配置/切分组会持续积累不同节点名。 | 长期运行内存缓慢增长（单条很小），可维护性问题。 | 切换分组（applyInitialData）或超过 N 条时 `clear()`。 |
| MINOR | SettingsTheme.vue:288 | `GLASS_BLUR_MAX` 导入后**全文再无使用**（U-07 把 `trackStyle` 默认值改成 `GLASS_TINT_MAX`，另一调用点显式传 GLASS_DEPTH_MAX）。 | 无效导入（vue-tsc 未开 noUnusedLocals 所以没红）；读者会误以为模糊上限仍参与计算。 | 从 import 列表删掉。 |
| MINOR | Subscriptions.vue:444-446 | `catch (e) { console.log("Import cancelled"); }` 属残留调试代码（HEAD:365 即存在，非本轮引入），且把 `SelectLocalFile` 的**非取消类错误**一并静默吞掉。 | 控制台噪声 + 真失败没有任何用户可见反馈。 | 去掉 console.log，改 `reportError(e,'subs.pickFile')`；用户取消已由空 path 表达，不必靠异常。 |
| MINOR | TrafficCard.vue:66 | `showAlert` 从 store 导入但全文件未使用（HEAD 即如此，非本轮引入）。 | 死导入。 | 从导入列表移除。 |
| MINOR | SettingsBehavior.vue:246-250 | `resetKey` prop 声明后**全文件无读取**（Settings.vue:9 仍在传 `:reset-key="resetVersions.behavior"`），属 G-15 清理 theme 同类项时漏掉的一处（非本轮引入）。 | 消费方为一个无作用的 prop 维护计数器；读者会以为 behavior 页重置靠 resetKey 触发，实际靠 `loadBehavior` 换对象。 | 一并删掉 prop / 父级传参 / resetVersions.behavior，或加注释说明重置由 loadBehavior 完成。 |

> 备注：Proxies.vue:894 的 `proxy-delay-update` 新闸门会顺带丢弃后端并发出的**组级**更新（payload `name`=组名、`from`=节点名）。当前 handler 只处理 `node.testing` 复位，组名不在 `g.proxies` 里本来也匹配不到，故**不是缺陷**；但将来若把该事件用于组级延迟展示，测速期间会被这条闸门吞掉 —— 登记在 §3-5。
## 2. 已确认 OK 清单（关键不变量逐条核对）

| 项 | 证据 |
|---|---|
| **C-02 未保存 YAML 守卫「只弹一次」** | 侧栏：Sidebar.vue:15-18 `$emit('update:activeId')` → App.vue:66 `@update:activeId="handleTabSelect"` → :776-778 `requestTabChange` → :750-773 在切页前判定 `yamlEditorModified`，`yamlLeaveConfirming` 挡重入。返回箭头：YamlEditor.vue:5 `@click="handleBack"` → :454-461 自己 confirm 后 `emit('back')`；App.vue:782-785 `closeYamlEditor` 直接切页（**不二次确认**）。更新 chip：App.vue:416 也走 `requestTabChange`。三入口各一次确认，无双弹也无漏弹。 |
| **H-03 三处防重入闸门** | Overview.vue:201-206 `restartInFlight` 在 `showConfirm`（:207）之前置位、`finally`（:219）复位；TrafficCard.vue:157-163 同型（`resetInFlight`，confirm :164、finally :181）；App.vue:367/392-411 更新 chip 的 `appUpdateActionBusy` 在 `openModal` 之前置位，`onConfirm` finally + `onCancel` 双路复位。三处均满足「闸门早于确认框、finally 复位」。 |
| **G-04 读-合并-写 + 串行链** | Settings.vue:177-200（`loadedSnapshot` / `sameFieldValue` / `pickDirtyFields`）、:240-244（`behaviorSaveChain.then(run, run)` 双分支、链不断）、:246-282（先拍 localSnapshot → 算 dirty → `GetAppBehavior()` 取 latest → `{...latest, ...dirty}` → SAVE；成功基线前进到 localSnapshot，失败 alert + loadBehavior）。基线取「提交前本地快照」而非「保存后 behavior」，Get/SAVE 往返期间的新改动仍算脏、下一次补上 —— 逻辑自洽。 |
| **G-05 还原后真的重拉快照** | SettingsAbout.vue:373 `emit('reload-behavior')` → Settings.vue:13 `@reload-behavior="loadBehavior"` → :205-236 重新 `GetAppBehavior` + 重建 `loadedSnapshot` + `syncThemeFromBehavior`。链路闭合。 |
| **P-05 / C-01 订阅排序** | Subscriptions.vue:253 `SORT_MODE_AVAILABLE = true`（入口 :16 `v-if` 恢复可见）；:266-287 `persistOrder` 串行 + `sortPendingFlush` trailing flush（finally 里同步重启、无 await 间隙）；失败 `reportError + showToast + await fetchConfigs()` 以后端为准；:289-303 moveUp/moveDown 先改内存再 await 落盘。`App.ReorderSubs(localConfigs.map(c=>c.id))` 与 Go `func (a *App) ReorderSubs(ids []string) error`（app.go:843）一致；TS 绑定 `ReorderSubs(ids: string[] \| null)` 的方法 ID **3022631746 经 FNV-1a("main.CoreService.ReorderSubs") 复算一致**（RenameConfig 73568854 / DeleteConfig 551044507 / GetLocalConfigs 4093441616 三个样本全部命中，说明手写 ID 与生成器同源，不会运行时 method not found）。 |
| **ModernSelect 挂载期不再 emit change** | ModernSelect.vue:58-63 只有 `emit('update:modelValue')`；`change` 唯一出口 `selectOption`（:142-146）。`disabled` 在 `toggle()`（:123）拦住展开，About/Behavior 新加的 `:disabled="loading"` 生效。G-09 删掉的 z-index/尺寸规则确被 :86-98 内联 `zIndex:'10001'` + `updatePosition()` 覆盖（teleport 后选择器本就不匹配）。 |
| **ModernNumberInput clamp / Enter / Esc 不破坏输入** | Enter → `onSubmitKey`（:114-116）统一走 blur 提交，只 emit 一次 change；失焦 `commit()`（:72-79）clamp 到 [min,max]、NaN 先回落 min 再回落 `lastValid`，并把显示写回合法值（:110-112）；`step` 改 computed（:44-46）后可响应父级改值。**仅 Esc 分支有问题，见 §1 MAJOR**。 |
| **样式：radius token 化无多值误改** | 全量扫描本批 27 个 .vue 的 `border-radius`：37×`--ui-radius-sm`、15×`--ui-radius-lg`、9×`--ui-radius-md`、7×`--ui-radius-full`，**无一条多值（如 8px 0 0 0）被替换**；剩余裸 px 均为 3/4/6/11/16/24px 与 50%（圆形），不属 8/10/12 体系。token 只在 tokens.css:42-44 定义且值 sm=8/md=10/lg=12（与替换前逐值相同）、full=999px（原 999px 语义不变），全仓无局部重定义 ⇒ 作用域无问题。 |
| **死代码清理后模板无残留引用** | `size-standard`：仅 Proxies.vue:1311 注释；基础 `.node-grid`（:1293-1298）与删除的 `.size-standard` 逐值相同（260px/16px），standard 模式由基础规则兜住。`loading-state`：仅 Rules.vue:752 注释。`nav-slide`：仅 Sidebar.vue:5/206 注释，TransitionGroup 删除不产生 DOM（Fragment），导航项仍是 v-show。节点 `now`：全仓只剩组级 now（模板 :97 / :630），无 `node.now` 读取。 |
| **alertError 与原文案/标题/isDanger 等价** | utils/alertError.ts:22-30 固定 `t(key,{error:displayErrorMessage(e)})` + `t('common.error')` + `true` + `toUserError(e).detail`，与原 6 处逐字模板（Rules 3 / Subscriptions 3）逐项相同；调用点 Rules.vue:261/495/551、Subscriptions.vue:466/489/578 全部带 await，异常仍抛给调用方。 |
| **storage 收敛后键名不变** | HEAD 与工作区对比：Proxies `goclashz_proxyGroup` / `goclashz_proxySortState`、Connections `goclashz_connFilter`、Rules `goclashz_ruleTab` **完全一致**；utils/storage.ts 的 `safeGetString` / `safeSetString` 内部 try/catch 与原内联实现同义（读失败→null，写失败静默）。 |
| **新增 i18n key 8 语言齐全** | 对 8 个语言包做嵌套解析后逐 key 校验：`proxies.loadFailedHint/speedTestBusy/speedTestFailed`、`rules.configSwitched/ruleChanged`、`subs.dupSubWarn/urlInvalid`、`yamlEditor.saveReloadFailed/backConfirmMsg/leaveConfirmMsg/discardConfirmTitle/discardBtn`、`traffic.staleHint/staleShort`、`deviation.severity.*`、`updateTask.panelTitle` 等 **8/8 全部存在**（每包 717 叶子，数量对齐）。组件内再无「用了但未定义」的 `t('...')` 字面量（deviation.field/reason/severity 是前缀族，经 `localized()` 拼接，叶子 8/8 存在）。 |
| **Events.On 与 unsub、addEventListener 与 remove 配对** | Connections 1/1、Proxies 4/4、Subscriptions 4/4、SettingsAbout 1/1、SettingsUpdate 6 callsite → `unsubs` 数组（:107-115 全部 push、:118 forEach 全解）、SettingsTun 同型（:217/:220）、Subscriptions addEventListener 3/3（:711-713 与 :747-749）。 |
| **定时器清理** | Proxies `sliderRaf`（:269-285 记 id，:981 卸载 cancel）；Connections `titleTargetTimer`（:340-352，卸载 :392-395 清）；Rules `searchTimer`（:331 卸载清）；UpdateTaskPanel 单 `clearTimer` + `onUnmounted(stopClearTimer)`（:44-68）；YamlEditor `modifiedRecheckTimer`（:257-263 取消函数、卸载 :477 调用）。 |
| **组件卸载后不写状态** | YamlEditor `disposed`（:87、:293、:471-478）；Connections `monitorGen`（:290、:311-337 两处 await 后判 gen；finally 只在 gen 匹配时写 isLoading）；DeviationStrip `disposed`（:77-86）。 |
| **H-02 / H-20 / S-08 与 store 接口对齐** | TrafficCard 删掉的 `updateLatestTraffic` watch 确认有唯一写者 App.vue:1009（traffic-data 事件），波形采样不变；`globalState.outboundIPStale` 在 store.ts:314/793/808/880/885/907 有写、TrafficCard.vue:102/137 消费，字段名与语义一致。 |
| **H-14 不再由前端改权威字段** | useTunToggle.ts:133-140 只 `settleTunIntent()` + 失败时 `refreshState()`，删除 `globalState.desiredTun=false`；Overview.vue:274-279 同型；`settleSystemProxyIntent/settleTunIntent`（store.ts:232-242）只清 controlOverlay 不写后端。 |
| **P1-E 测速批次隔离** | `proxy-test-start` 只在本轮测速中受理（Proxies.vue:874-888），`testBatchNodes` 收集本批节点，`proxy-delay-update` 只认本批（:894）；`proxy-test-finished` 对本批未开测的成功通知整条丢弃（:916）；`stopTesting` 先作废批次再发 RPC（:771-774）。后端 `SilentUI` 确实不发 start/finished（delay_manager.go:105、460、497、592-594），自动测速不会污染批次判定。 |
| **P-02 结构化 payload 双兼容** | Proxies.vue:755-769 先读 `{ok,message}`（v3_events.go:80 已注册 `map[string]any`、delay_manager.go:463/498 已发结构化），再回退字符串「测速完成」；`ok=false` 时 `DELAY_TEST_BUSY` 单独 info toast，其余 reportError + error toast。 |
| **G-01 / G-02 / G-17 / G-18 设置页语义** | SettingsLan.vue:11/185 发 `'lan'`、Settings.vue:299 把 lan 映射成后端 network 且 `resetVersions.lan` 自增；Lan :251-268 保存前三条 CIDR 整体复校、不通不落盘；SettingsNetwork.vue:343-354 hosts 校验拦截；SettingsUwp.vue:146-151 批量操作作用于 `filteredUwpApps`（同一批对象引用，改内存即改源），文案键 `invertSelect` 8 语言已改成「全部不选」。 |
| **G-06 / U-20 兜底值与 Go 默认一致** | Settings.vue:173 兜底 behavior 的 `logLevel:'error'/delayRetention:true/updateInterval:1/subUA:'clash-verge'/activeMode:'rule'` 与 behavior.go Default() 一致；SettingsNetwork.vue:267-290 补的 5 个 LAN 字段与 `NetworkConfig` 的 json tag（config.go）一致、`tcpKeepAliveInterval` 15→30 与 Go 默认 30 一致。 |
| **G-07 / G-13 / G-14 / G-19 / G-21** | G-13 三页控件 `:disabled="loading"` 覆盖（Behavior 16 处 / Theme 12 处 / About 4 处，逐条看到）；G-14 `handleReset` 补 `isDanger=true`（Theme:647-653）并补 `colorDelay=false`（:663）；G-19 语言只写 behavior + emit save（About:48-55/234-241），setLocale 由 saveBehavior 成功分支统一执行（Settings.vue:271-273；Go 侧 language 有 normalize 默认 zh-CN 不会为空，故 v-model 不会把空值纠错成首项）；G-21 `refreshRuntimeAssets` 已包 try/catch（SettingsUpdate:76-94）。 |
| **P-01 / P-22 / P-23 / P-24 / P-26 / P-27 / P-28** | P-01 删除改按规则文本定位（Rules:502-547，`rulesOf` 与 `currentRulesList` 共用同一取值逻辑 :312-326）；P-22 删除 `onMounted(loadRules)` 后 `watch(activeConfigId,{immediate:true})`（:353-366）仍覆盖首拉；P-23 添加规则同步 in-flight（:477-489）；P-28 删除激活配置时先本地清空三字段（Subs:561-570）；P-26 `fetchConfigsSeq` 序号 + finally 只由最后一次收 loading（:271-300）；P-27 `isUpdating` 入口同步置位 + `subs-update-cancelled` 监听（v3_events.go:177 已注册、tasks/manager.go:99 有发射端）+ 传输层失败复位。 |
| **P-04 / P-13 / P-17 / P-18 / P-19 / P-20 / P-21** | Connections 的 monitorGen / connEventSeq / `chains || []` / filterMode 合法值校验 / 暂停态本地移除 / 标题锚点有限重试全部就位；P-13 删除的两对 onActivated/onDeactivated 确为死钩子（App.vue 全 v-if 分支、无 KeepAlive），mounted/unmounted 覆盖同一语义。 |
| **YamlEditor P-06 / P-07 / P-29~P-33** | 全局 keydown 与 MutationObserver 已提到 `await loadConfig()` 之前注册（:449-471），`disposed` + `loadSeq` 双闸（:281、:293、:325）；`emitStatus` 单一出口（:241-248）；docChanged 只置脏（:266-278）+ 180ms 延迟比较只允许「变干净」；保存前精确比较（:335-340）；`isSavedButReloadFailed` 命中才清 dirty（:334-388）。**唯一可测缺口：后端若不保留「保存成功…重载」措辞会退回旧提示（方向安全，fix 报告已登记）。** |
| **H-21 / H-22** | DeviationStrip 用生成类型 `ConfigDeviation`（models.ts 有 key/severity/module/field/reason；Go 三处构造均已填 Severity），v-for key 换 `d.key`（Go 的三个 key 互不相同）；Toast 改为展示镜像 + hover 冻结 + 内容去重，store 侧 `showToast` 已自带同文案续期（store.ts:1345-1350）、无就地改字段，故按 id 序列 watch 足够，二者叠加不产生重复/丢条。 |
| **无残留调试/临时文件** | 本批 27 个 .vue + useTunToggle + manifest 中：`debugger` / `TODO` / `FIXME` / `ts-ignore` 0 处；`console.*` 仅 Subscriptions.vue:446 一处（§1 MINOR，HEAD 即存在）；新增 util（alertError / storage / clipboard）均有真实消费者，无「建了不用」；`.fixbackup/` 已在 .gitignore:59，不构成仓库残留。 |

## 3. 未修 / 移交项（非缺陷，供主控汇总）

1. **C-02 关窗路径**（X / Alt+F4 / 托盘退出）仍无法在 App.vue 内拦截 —— 需后端 before-close 钩子，属 handoff-2 / backend-needs-2，与本次 diff 无关。
2. **P-16 失败态**：前端启发式不可靠（§1 MINOR），根治需 `GetInitialData` 带 `loadError` 字段。
3. **G-08 残留**：见 §1 MINOR，彻底修复需父级配合（纠错只改显示或同步基线）。
4. **P1-E 残余窗口**：旧批次若在 `cancelBatchAndWait`(600ms) 之后才收尾，旧 finished 可能落在新批次第一个 start 之后而被当成本批结束（按钮提前弹回，数据不丢）；根治需后端带 batch id。
5. **Proxies 组级 proxy-delay-update** 在测速期间被新闸门丢弃（当前无消费者），登记给未来改动。
6. **LOW 观察**：SettingsUpdate 的非 SUCCESS/CANCELLED 分支（G-20）是防御性死分支（Go 侧 `ExecuteRestore/ExportBackup` 失败一律走 error），不算缺陷、无需回退。

## 4. 复核范围与自证

- 逐文件看过 diff：Connections / DeviationStrip / ModernNumberInput / ModernSelect / Overview / Proxies / Rules / Settings / SettingsAbout / SettingsBehavior / SettingsDiagnostics / SettingsDns / SettingsHome / SettingsLan / SettingsNetwork / SettingsTheme / SettingsTun / SettingsUpdate / SettingsUwp / Sidebar / Subscriptions / Toast / TrafficCard / UpdateTaskPanel / UpdateTaskRow / YamlEditor / useTunToggle / settings/manifest（28 项）。
- 未改动文件：`EmptyState.vue`（diff 为 0，与 fix-6 报告一致）。
- 未运行 build / vue-tsc / 应用本体（按要求）；以上结论均来自 diff + 改后全文 + grep / 语言包嵌套解析 / FNV 方法 ID 复算。
