# 06 · 前后端契约审计（调用点 / 事件 / 模型 / 通知码）

> 审计代理：contract（前端只读审计，6 代理并行之一）
> 目标仓库：D:\GOLANG\GoclashZv3（Windows / Wails v3.1.0 + Vue3 + TS）
> 时间：2026-11-18 20:10 +08:00
> 只读约束：未修改任何源码 / 样式 / 配置 / locales；唯一写入是本文件。

---

## 0. 审计范围、方法、覆盖度

### 0.1 范围（按任务书）

| 面 | 对象 |
| --- | --- |
| 调用侧 | `frontend/src/**` 下全部 `App.*` 调用点与 `Events.On` 监听点 |
| 契约真相 | `frontend/bindings/goclashz/{appservice,coreservice,updateservice}.ts`、`bindings/goclashz/core/**/models.ts`、`bindings/events/index.ts` |
| 后端真相 | `app.go`、`config_editor.go`、`v3_events.go`、`wails_event_sink.go`、`services_v3_generated.go`、`core/appcore/**`（定点）、`core/clash/config.go`、`core/utils/settings.go`、`core/tasks/manager.go`、`core/logger/*` |

额外读了 Wails v3.1.0 源码（`C:\Users\Jiguang\go\pkg\mod\github.com\wailsapp\wails\v3@v3.1.0\pkg\application\{events,event_manager,events_strict,messageprocessor_events}.go`），
用于确认两件此前只有断言没有证据的事：

1. **未注册事件名照样能 Emit**（`validateCustomEvent` 只在 `!ok` 分支打 warning 后 `return nil`），所以 `geo-update-<key>-*` / `notify-error` 这类"没注册"的事件**不会丢**，前端监听是能收到的；反过来"没监听"才是真空洞。
2. 注册事件做**精确类型校验**（`reflect.TypeOf(Data) == typ`，否则 `Cancel()` + 上抛），因此 `v3_events.go` 里的类型声明是硬约束，不是文档。

### 0.2 方法（全部脚本化，可复现）

| 步骤 | 做法 | 产出 |
| --- | --- | --- |
| 命名空间交集 | Node 正则抽三个 binding 文件的 `^export function (\w+)`，求两两/三元交集 | 108 个方法，交集 = 0 |
| ByID 冲突 | 抽 `$Call.ByID\((\d+)` 与函数名配对，找重复 ID | 108 个 ID，重复 = 0 |
| 调用点全量 | 平衡括号扫描 `App\.([A-Z]\w*)\s*\(`，跳过注释行，按顶层逗号数实参 | 111 个调用点 / 83 个方法 |
| 签名对齐 | 解析 `export function N(a?: T, ...): $CancellablePromise<R>` 得参数个数/可选/返回类型 | 0 个实参不匹配 |
| 浮动 Promise | 前缀不是 `await/return/void/=/.catch` 的 `App.X()` | 13 处，其中 4 处真浮动（均为 Promise<void>，见 §1.2 与 C-23） |
| 事件双向差集 | 抽 `Events.On(<字面量>)` 与 Go 的 `\.Emit("<字面量>"`，另补常量/动态族/任务框架 | 监听 44 / 发射 36 + 2 族 |
| 通知码全集 | 抽 `notify(...)` / `setLastErrorCode` 的 code 实参，与 `NOTIFICATION_UI` 对表 | 后端 23 个 code，表内 15（7 个死键） |
| 死绑定 | bindings 方法全集 − 前端调用名集合 | 26 个零调用 |
| 死 import/export | 逐文件把 import 语句剔除后全文计数 | 6 处确认死 import（另 5 处待人工确认）、7 处死 export |

### 0.3 实际读过的文件与行数

**前端（逐行读过脚本区，模板按需定点）**

| 文件 | 总行 | 读过 |
| --- | --- | --- |
| src/api.ts | 17 | 全文 |
| src/store.ts | 1250 | 全文 |
| src/App.vue | 1242+ | script 244–1032 全文；template 1–243、154 / 179–241 定点 |
| src/trafficWaveState.ts | 242 | 全文 |
| src/composables/useTunToggle.ts | 138 | 全文 |
| src/components/Proxies.vue | 1312 | script 141–824 全文 |
| src/components/Subscriptions.vue | 819 | script 198–647 全文；template 90–196 |
| src/components/Rules.vue | 718 | script 129–517 全文；template 60–104 |
| src/components/Connections.vue | 663 | script 127–481 全文 |
| src/components/Overview.vue | 365 | script 72–321 全文 |
| src/components/YamlEditor.vue | 523 | script 43–393 全文；template 5/20–24 |
| src/components/Settings.vue | 240 | script 21–252 全文 |
| src/components/SettingsUpdate.vue | 109 | script 47–112 全文 |
| src/components/SettingsTun.vue | 221 | script 62–221 全文；template 24–43 |
| src/components/SettingsUwp.vue | 227 | script 73–210 全文 |
| src/components/SettingsAbout.vue | 465 | script 201–383 全文 |
| src/components/SettingsDns.vue | 288 | script 172–297 全文；template 83–164 |
| src/components/SettingsNetwork.vue | 435 | script 253–352 全文 |
| src/components/SettingsLan.vue | 388 | script 170–291 全文 |
| src/components/SettingsDiagnostics.vue | 313 | script 145–271 全文 |
| src/components/SettingsBehavior.vue | 333 | script 150–209、250–296 |
| src/components/SettingsTheme.vue | 925 | script 390–453、640–667 |
| src/components/UpdateTaskRow.vue | 182 | script 39–97 全文 |
| src/components/UpdateTaskPanel.vue | 108 | script 14–47 全文 |
| src/components/DeviationStrip.vue | 86 | script 31–94 全文 |
| src/components/ModernSelect.vue | 284 | script 43–176 全文 |
| src/components/TrafficCard.vue | 292 | script 61–167 全文 |
| src/components/Sidebar.vue | 229 | script 45–83 全文 |
| src/components/Toast.vue | 22 | 全文 |
| src/utils/errors.ts | 149 | 全文 |
| src/utils/report.ts | 40 | 全文 |
| src/utils/theme.ts | 393 | 430–455 + 关键引用 |
| src/locales/index.ts | 86 | 全文 |
| 其余 utils / locales / 小部件 | — | 仅 grep 引用，未逐行 |

**后端**

| 文件 | 读过 |
| --- | --- |
| v3_events.go (186) | 全文 |
| wails_event_sink.go (133) | 全文 |
| app.go (1183) | 290–560 / 678–905 / 955–1230 / 1283–1370 / 1441–1492 |
| config_editor.go (38) | 全文 |
| services_v3_generated.go (356) | 方法签名全表 |
| core/appcore/controller.go (2210) | 519–700（AppState + GetAppState）、1590–1670（notify）、2009/2092/2160/2225/2405/2444/2473–2486 / 2557 |
| core/appcore/delay_manager.go (742) | 100–200 / 380–740 |
| core/appcore/subscriptions.go (220) | 150–275 |
| core/appcore/updater_orchestrator.go (483) | 120–250 |
| core/appcore/geo_update_manager.go (276) | 100–280 |
| core/appcore/component_update_state.go (89) | 全文 |
| core/appcore/config_editor_controller.go (27) | 全文 |
| core/appcore/events.go (24) | 全文 |
| core/appcore/outbound_ip.go (412) | 190–270 |
| core/tasks/manager.go (116) | 全文 |
| core/clash/config.go (606) | NormalizeLogLevel / BuildRuntimeConfig 头 / GetNetworkConfig |
| core/utils/settings.go (69) | 全文 |
| core/logger/level.go (33) | 全文 |
| core/logger/buffer.go (290) | LogEntry 定义 |
| hotkey_windows.go / tray_runtime_windows.go | 29–48 / 488–507 |

### 0.4 未覆盖（诚实声明）

1. **样式与 i18n 文案本身**：`style.css` / `styles/*.css` / 8 个 locale 包未逐行读（视觉与文案不属本任务判据）；只核了 `t()` 的返回值语义（影响 `||` 兜底判断）。
2. **`utils/{glass,theme,hoverGuard,scrollFade,edgeFade,icons,format,normalize}`** 只读到与本任务相关的函数，未做独立审计。
3. **`main.ts`（12 行）/ `vite-env.d.ts`** 未读（启动顺序另有代理负责）。
4. **`core/appcore/controller.go` 的 2000+ 行内核控制主体**只读了与本次 5 类判据相关的分支（TUN/系统代理/模式/重启的对外语义），未做并发模型审计（属 A2 代理范围）。
5. `modern-tauri` 无（技术栈是 Wails），不适用。
6. **动态验证 0 次**：未启动应用、未跑 build/tsc（遵守主控约束），所有"运行时才暴露"的结论都标 B/C。

### 0.5 证据等级

- **A**：代码可直接确证（读到了发射点 + 消费点，或读到了签名与调用点的完整文本）。
- **B**：代码 + 运行时推断（结论依赖事件/IPC 顺序、时序窗口、或某个分支在真实配置下可达）。
- **C**：假设待实测（依赖未验证的运行时前提，已在正文写出前提条件）。

---

## 1. 结论（先看这 5 段，它们是任务书点名要的）

### 1.1 命名空间合并对账 —— **零同名导出，零 ByID 冲突**

`frontend/src/api.ts:13-17`

```ts
export const App = {
  ...AppService,
  ...CoreService,
  ...UpdateService
};
```

脚本解析结果：

| namespace | 文件 | export function 数 |
| --- | --- | --- |
| AppService | appservice.ts | 31 |
| CoreService | coreservice.ts | 62 |
| UpdateService | updateservice.ts | 15 |
| 合计 | | **108** |

**交集结论（明确写）**：

- AppService ∩ CoreService = **0**
- AppService ∩ UpdateService = **0**
- CoreService ∩ UpdateService = **0**
- 三元交集 = **0**

因此 `{...a, ...b, ...c}` 不存在"后展开者静默覆盖前者"的调用错服务风险——这一点**不成立，无需修复**。

补充加验（任务书没要求，但同属"静默覆盖"风险）：108 个 `$Call.ByID(N)` 的 N **零重复**（范围 23055341 ~ 4289057202，唯一 108）。即 Wails 生成的调用 ID 不存在跨服务撞车，不存在"A 服务的方法实际调到 B 服务"的可能。

**唯一的残余风险（P2 级、不计入问题数）**：对象展开是**运行时合并**，TS 侧 `App` 的类型是三个 namespace 的交叉类型，编译器**不会**阻止未来某人在 CoreService 里新增一个与 AppService 同名的方法——那时才会静默生效。建议在 `api.ts` 加一句构建期断言（见 §4 冗余表 R-1），或在 CI 里保留本次的脚本检查。

### 1.2 调用点全量对账（111 个调用点 / 83 个方法）

- **实参个数与签名 100% 一致**：0 个 `TOO_FEW`、0 个 `TOO_MANY`。多参数方法（`ApplyUwpExemptionChanges` 2、`DeleteRule` 3、`AddRuleFromForm` 3、`GetOutboundIPForRoute` 2、`SaveConfigText` 2 等 15 个）全部逐一对过。
- **返回值消费无类型误用**：没有"void 方法当对象取字段"、"数组当对象"、"T|null 未判空即解引用"的调用点。
- **但有两处"返回值/载荷被整段丢弃"**：C-04（事件 err 文本丢弃）、C-05（测速 finished 的失败原因丢弃）。这两处**不是类型错误，是信息丢弃**，用户可感。
- **26 个绑定方法前端零调用**（C-07），其中 `RepairRuntimeAssetsAsync` + `GetRuntimeAssetRepairProgress` 是配套的一对死 API（后端注释明说"UI 轮询"）。
- **浮动的 App 调用 4 处**（C-23）：`App.HideMainWindow()` / `App.StartStreamingLogs()` / `App.SyncState()` / `App.StopConnectionMonitor()` 未 `await` 也无 `.catch`。这 4 个都是 `Promise<void>`，Go 侧全部不返回 error，所以**当前不会产生 unhandled rejection**——仅为防御性缺失。

### 1.3 事件对账（双向差集）

前端监听 44 个名字（43 个字面量 + 1 个模板 `"geo-update-"+key+"-success"`），后端 36 个静态发射名 + 1 个动态族（`geo-update-<key>-{start,cancelled,error,success,busy}`）+ 1 个任务框架派生族（`<task>-{start,error,success,cancelled}`）。

**A. 前端监听 → 后端从不发：0 个。**（无死监听，逐字比对含连字符/大小写）

**B. 后端发 → 前端不监听：23 个名字**（17 个业务事件 + 6 个刻意不消费的内部 job 事件；其中 12 个业务事件历史未点名 → C-08，5 个已被 F-A5-06/F-A5-23 记录）

| 事件名 | 发射点 | 状态 |
| --- | --- | --- |
| `notify-error` | tray_runtime_windows.go:503 | 历史已记录（F-A5-23） |
| `driver-install-error` | component_update_service.go:105/165/188 | 历史已记录（F-A5-06/23） |
| `driver-install-cancelled` | tasks/manager.go:99 | 历史已记录（F-A5-23） |
| `driver-install-start` | tasks/manager.go:91 | 历史未点名（新列，C-08） |
| `geo-update-all-error` | geo_update_manager.go:331 | 历史已记录（F-A5-06/23） |
| `geo-update-all-start / -success` | geo_update_manager.go:264/335 | 历史未点名（新列，C-08） |
| `geo-update-<key>-error` | geo_update_manager.go:203/223 | 历史已记录（F-A5-06） |
| `geo-update-<key>-start / -cancelled / -busy` | geo_update_manager.go:139/154/198/233 | 历史未点名（新列，C-08） |
| `geo-update-active-sync` | geo_update_manager.go:117 | 历史未点名（新列，C-08） |
| `proxy-test-progress` | delay_manager.go:629 | 历史未点名（新列，C-08） |
| `config-file-saved` | config_editor_controller.go:20 | 历史未点名（新列，C-08） |
| `core-update-check-success / -cancelled` | tasks/manager.go:109/99（taskName=core-update-check） | 历史未点名（新列，C-08） |
| `subs-update-cancelled` | tasks/manager.go:99 | 历史未点名（新列，C-08） |
| `app-update-*-job-{start,error,cancelled}` ×3 组 | tasks/manager.go（内部任务名） | **刻意不消费**，v3_events.go:178-179 已注明，不算问题 |

**C. 名称拼写不一致：0 个。**（含 `traffic-stat-mode-changed` / `tun-mips-fallback` / `geo-update-all-*` 这类易错名，逐字一致）

**D. 事件 payload 结构对账（抽查 8 个高频/高风险）**

| 事件 | Go 载荷 | 前端读取 | 结论 |
| --- | --- | --- | --- |
| traffic-data | TrafficSnapshot（8 字段，json camelCase） | `data.up/down/upRaw/...` | ✅ 全对齐 |
| log-message | logger.LogEntry（type/source/payload/time/ts/pid/event/cat/code/corr/durMs/attempt/caller） | `log.ts/cat/code/event/corr/durMs/attempt/caller/payload/type/source` | ✅ 全对齐 |
| proxy-delay-update | map{name,delay,status,message,source[,from]} | 同名字段 | ✅ |
| proxy-state-sync | []ProxyGroupState{name,type,now} | `s.name / s.now` | ✅ |
| component-update-tasks-sync | []UpdateTaskState（11 字段） | `t.key` 建索引 + 全字段 | ✅ |
| core-update-none / -available | map{local,remote[,assetUrl,releaseUrl]} | `data.local/remote/releaseUrl` | ✅ |
| app-update-progress | map{bytesDone,totalBytes,speedBps,etaSec} | 同名字段 | ✅ |
| app-update-downloaded | map{version,path} | `payload.version/path` | ✅ |
| **proxy-test-finished** | **string（"测速完成" / "测速启动失败：…" / "没有可测速的有效节点" / "DELAY_TEST_BUSY"）** | **监听器无参数，整段丢弃** | ❌ **C-05** |
| **core-update-check-error** | **string（err.Error()）** | **只复位布尔，文本丢弃** | ❌ **C-04** |

### 1.4 模型字段对账

对过 9 个模型的双侧定义（bindings/models.ts ↔ Go struct json tag）：

| 模型 | 字段数 | 结论 |
| --- | --- | --- |
| AppState | 25 | ✅ 前端读的每个字段都在 Go 结构里，且 json tag 全 camelCase |
| UpdateTaskState | 11 | ✅ 类型逐个对齐（int64↔number，float64↔number） |
| OutboundIPResult | 11 | ✅ 含 source4/source6/message4/message6/complete |
| TrafficSnapshot | 8 | ✅（`UpRaw float64` ↔ number） |
| logger.LogEntry | 13 | ✅ 可选字段前端全部用 `??` / 真值判断，无直接解引用 |
| Notification | 4 | ✅ level/code/message/source |
| ProxyGroupState | 3 | ✅ |
| DNSConfig / FallbackFilterConfig | 15 / 4 | ✅ 数组或 null 字段在模板里**全部**写了空数组兜底（SettingsDns.vue:83/107/113/119/125/131/158/164），不会对 null 调 join |
| DataDirInfo | 29 | ✅ 诊断页读的字段全在 |

**没有发现字段名拼错、类型不符、可选字段被直接解引用、Go null slice 直接 `.map` 的问题。**

### 1.5 通知 / 错误 code 映射

后端实际会产出的 `Notification.Code` 全集（23 个，扫全部 `notify(...)` 与 `setLastErrorCode(...)` 调用）：

```
warn : SYSTEM_PROXY_STATE_UNKNOWN, ROUTE_SWITCHING, TUN_NOT_READY, HOTKEY_REGISTER_FAILED
info : DEFERRED_RESTART_QUEUED           success: DEFERRED_RESTART_RESUMED
error: STARTUP_RESTORE_{NO_CONFIG,HELPER_REQUIRED,WINTUN_MISSING,FAILED},
       DEFERRED_RESTART_ABANDONED, MODE_SAVE_FAILED, TRAY_COMMAND_FAILED,
       ASSET_WINTUN_MISSING, CONFIG_ROLLBACK_INCOMPLETE, CORE_START_FAILED,
       RUNTIME_OPERATION_FAILED, STARTUP_TASK_{CREATE,DELETE}_FAILED,
       SYSTEM_PROXY_{RECONCILE,SET}_FAILED, TUN_{ADAPTER_NOT_READY,RESTORE_FAILED}
```

`NOTIFICATION_UI`（store.ts:936-955）共 15 条：

| 分类 | 键 | 后端是否产出 |
| --- | --- | --- |
| silent(4) | ASSET_WINTUN_BUSY / ASSET_CORE_PROBE_TIMEOUT / HELPER_WAKE_TIMEOUT / TUN_ADAPTER_PENDING | **全部 0 引用（死键）** |
| toast(7) | ASSET_SEED_UNAVAILABLE / STARTUP_TASK_CHECK_WARNING / STARTUP_TASK_UNHEALTHY | **0 引用（死键）**；后两者实际走独立事件（store.ts:1044/1050） |
| toast(7) | DEFERRED_RESTART_{QUEUED,RESUMED,ABANDONED} / SYSTEM_PROXY_STATE_UNKNOWN | 有效 |
| modal(4) | STARTUP_RESTORE_{4 个} | 有效 |

**结论**：15 个 code 未在表内（含 9 个 error 级），但它们全部走 `resolveNotificationUi` 的 level 兜底 → error→modal、warn/info/success→toast，**结果正确，没有"把错误当静默"的实际后果**。真正的问题是表里 **7 个键永不命中**（其中 4 个还是 silent 级，未来一旦后端补上这些 code，通知会被静默吞掉）→ C-09（P2）。

---

## 2. 问题总表（23 条：P0 0 / P1 6 / P2 17）

> 计数口径：不含 §6 的历史已记录项。C-05 与历史 F-A4-10 相邻但**代码分支不同**（见该条说明），计入新问题。

| ID | 严重度 | 类别 | 位置(file:line) | 一句话问题 | 证据 |
| --- | --- | --- | --- | --- | --- |
| C-01 | **P1** | 前后端对接/逻辑错误 | components/Subscriptions.vue:243-260 | 订阅「排序模式」只排内存数组，后端无落盘接口，切页/重进即还原 | A |
| C-02 | **P1** | bug/操作冲突 | components/YamlEditor.vue:5,350-356；App.vue:646-648 | YAML 未保存修改在「返回」/侧栏切页/关窗时被静默丢弃，无确认 | A |
| C-03 | **P1** | 操作冲突 | store.ts:134-145；Settings.vue:170-216；utils/theme.ts:448-455 | 侧栏明暗开关与设置页「整对象保存」互顶：改任一设置会把明暗/主题顶回旧值 | A |
| C-04 | **P1** | 前后端对接 | store.ts:1065-1067；SettingsUpdate.vue:87 | `core-update-check-error` 的错误文本被丢弃，「检查内核更新」失败零反馈 | A |
| C-05 | **P1** | 前后端对接 | components/Proxies.vue:769-780；delay_manager.go:476-524 | `proxy-test-finished` 携带的失败原因被监听器忽略，批量测速失败静默 | A |
| C-06 | **P1** | 操作冲突 | components/Proxies.vue:646-692,769-780 | 「停止测速→立刻重测」：被取消批次迟到的 finished 事件把新一轮的 isTesting 清成 false | B |
| C-07 | P2 | 冗余/功能缺失 | 见 §3-C-07 | 26 个绑定方法前端零调用（24%），含一对配套死 API | A |
| C-08 | P2 | 前后端对接 | 见 §3-C-08 | 12 个后端事件前端零监听（历史 F-A5-06/23 未点名的部分） | A |
| C-09 | P2 | 逻辑错误/冗余 | store.ts:936-955 | NOTIFICATION_UI 15 条里 7 个 code 后端永不产出（4 个是 silent 级） | A |
| C-10 | P2 | 冗余 | store.ts:325-437 | `updateStateFromBackend` 的 PascalCase 兼容分支约 24 处全是死代码 | A |
| C-11 | P2 | bug | store.ts:346,402,405 | 3 处 `localStorage.setItem` 无 try/catch，异常会中断整段状态同步 | B |
| C-12 | P2 | 冗余 | store.ts:23-50 | 手抄 `UpdateTaskState` / `OutboundIPResult` 契约类型，未复用 bindings | A |
| C-13 | P2 | bug/性能 | App.vue:154,526-539 | 日志列表索引键 + `splice` 裁剪 → 索引键错位、整表重渲染 | A |
| C-14 | P2 | 操作冲突 | SettingsLan.vue:254-278；SettingsNetwork.vue:336-348；SettingsDns.vue:236-244；SettingsTun.vue:192-212 | 四个设置页自动保存无 in-flight 互斥/序号，快速连改可能旧负载后落盘 | B |
| C-15 | P2 | 操作冲突 | components/Proxies.vue:526-588 | `loadData()` 有 6 个并发入口、无序号校验，旧快照可覆盖新结果 | B |
| C-16 | P2 | bug | components/UpdateTaskPanel.vue:39-46 | deep watch 在每次任务变更时新建 3s 定时器且卸载不清理 | A |
| C-17 | P2 | 操作冲突 | components/ModernSelect.vue:55-61 | 挂载即程序化 emit `change`，当前靠各页 loading 闸门挡住，属潜在自动保存源 | B |
| C-18 | P2 | bug/泄漏 | components/YamlEditor.vue:367-386 | onMounted 在 await 之后注册 document 监听与 MutationObserver，卸载竞态下泄漏 | B |
| C-19 | P2 | 逻辑错误 | components/Overview.vue:101-103 | 模式滑块用 findIndex 乘 100 除 3，mode 非三值之一时返回 -1 → 高亮条移出容器 | B |
| C-20 | P2 | 前后端对接/i18n | store.ts:1036-1039；outbound_ip.go:203-233 | 路由切换提示以 warn 通知直出中文技术串到 toast（8 语言界面里出中文） | A |
| C-21 | P2 | 逻辑错误/冗余 | 见 §3-C-21 | `t()` 之后用 OR 兜底中文共 6 处；t() 永不返回空串 → 兜底全是死代码 | A |
| C-22 | P2 | 冗余 | 见 §3-C-22 | 12 处死 import + 7 处死 export（模块内唯一引用点在声明处） | A |
| C-23 | P2 | bug | App.vue:687-695 | `handleClose` 未 try/catch：`GetAppBehavior` 失败时点关闭按钮无任何反应 | B |

---


唯一的模型层缺陷是**重复定义**：`store.ts:23-36` 手抄了一份 `UpdateTaskState`、`store.ts:38-50` 手抄了一份 `OutboundIPResult`，而 bindings 里已有导出（`bindings/goclashz/core/appcore/models.ts` 与 `core/sys/models.ts`）→ C-12。

## 3. 逐条详情

> 每条：位置 → 现状（代码摘录，均 ≤15 行，行号为真实文件行号）→ 为什么是问题（影响 + 触发路径）→ 修复建议。

### C-01 订阅「排序模式」不落盘：一次切页即还原

**位置**：`frontend/src/components/Subscriptions.vue:241-260`（配合 template 14-18 / 57-59）

```vue
241| const isSortingMode = ref(false);
243| const toggleSortMode = () => {
244|   isSortingMode.value = !isSortingMode.value;
245|   activeMenu.value = null;
246| };
248| const moveUp = async (index: number) => {
249|   if (index <= 0) return;
250|   const items = [...localConfigs.value];
251|   [items[index - 1], items[index]] = [items[index], items[index - 1]];
252|   localConfigs.value = items;
253| };
```

```vue
57| <div v-if="isSortingMode" class="sort-controls">
58|   <button class="arrow-btn" :disabled="index === 0" @click.stop="moveUp(index)" ...>
59|   <button class="arrow-btn" :disabled="index === localConfigs.length - 1" @click.stop="moveDown(index)" ...>
```

**为什么是问题**

1. `moveUp` / `moveDown` 被声明成 `async`（历史上应该 await 过一个保存接口），但**函数体里没有任何后端调用**；`toggleSortMode`（"完成"按钮）也只翻转布尔。
2. 契约层根本没有这个能力：108 个绑定方法里只有 `GetLocalConfigs()`（只读；AppService 31 + CoreService 62 + UpdateService 15 全表已核对），没有"保存顺序"的写接口。
3. 列表来源是后端：`fetchConfigs()` 每次都用 `await App.GetLocalConfigs()` 整体替换 `localConfigs`（:266-282），而 `config-changed`（:630-635）、重命名/导入/删除后都会调它 → 排序被覆盖。
4. 组件生命周期：Subscriptions 挂在 `App.vue:99-101` 的 `v-else-if="currentTab === 'subs'"` 里，切到任何别的 tab 都销毁重建 → 排序 100% 丢失。

**用户可感后果**：用户点「排序」→ 用上下箭头把常用订阅挪到最前 → 点「完成」→ 去代理页看一眼再回来，顺序原样恢复。整段交互是空操作。

**修复建议**

- 方案 A（最小改动，建议先做）：**先隐藏该功能**，避免"看起来能排、实际不生效"。删掉 template :14-18 / :57-59 与 :241-260，或加 `v-if="false"` 并留 TODO。
- 方案 B（补契约）：后端已有现成能力可复用 —— `core/clash/sub.go` 的 `ReplaceSubIndex(indexSnapshot)`（当前被 DeleteConfig 的回滚路径使用）。包装成：

```go
func (a *App) SaveSubOrder(ids []string) error { return a.core.SaveSubOrder(ids) }
```

  前端在 `toggleSortMode` 关闭排序模式时 `await App.SaveSubOrder(localConfigs.value.map(c => c.id))`，失败则 showAlert 并重新 fetchConfigs。

---

### C-02 YAML 未保存修改被静默丢弃（返回 / 切页 / 关窗三条路）

**位置**：YamlEditor.vue:5、YamlEditor.vue:350-356、App.vue:646-648、App.vue:650-659

```vue
5 | <button class="back-btn" @click="$emit('back')" :title="t('common.back')">
```

```ts
350| const handleReload = async () => {
351|   if (isModified.value) {
352|     const ok = await showConfirm(t('yamlEditor.reloadConfirmMsg'), t('common.notice'));
353|     if (!ok) return;
354|   }
355|   loadConfig();
356| };
```

```ts
646| const closeYamlEditor = () => {
647|   currentTab.value = 'subs';
648| };
650| watch(currentTab, (newVal) => {
651|   if (newVal !== 'yaml-editor') {
652|     editingConfigId.value = '';
```

**为什么是问题**

编辑器自己已经知道"有未保存修改"（`isModified`，标题栏状态会显示「未保存」），`handleReload` 也实现了确认，但**三条离开路径全都没走这个确认**：

1. 点左上「返回」→ :5 直接 `$emit('back')` → App.vue:646 直接换 tab → 组件销毁。
2. 点侧栏任意菜单 → App.vue:61-67 的 `@update:activeId="val => currentTab = val"` 直接改 tab，YamlEditor 连事件都收不到。
3. 点窗口关闭按钮 → App.vue:687-695 直接 `HideMainWindow()` 或 `Application.Quit()`。
4. 除 Ctrl+S 手动保存外没有任何自动草稿/恢复。

**用户可感后果**：改了几十条规则行，误点返回或想先看一眼代理页 —— 内容整段消失，无提示、无草稿、无撤销。这是本报告里唯一的**用户数据丢失**路径。

**修复建议**

在 App.vue 收敛成一个"离开 yaml-editor 之前的确认"：

```ts
// App.vue
const yamlDirty = ref(false);           // handleYamlStatusChange 里同步 payload.modified
const requestLeaveYamlEditor = async (nextTab: string) => {
  if (currentTab.value === 'yaml-editor' && yamlDirty.value) {
    const ok = await showConfirm(t('yamlEditor.leaveConfirmMsg'), t('common.warning'), true);
    if (!ok) return;
  }
  currentTab.value = nextTab;
};
```

- `handleYamlStatusChange` 已经在收 `payload.modified`（App.vue:661-665），直接复用，无需新事件。
- Sidebar 的 `@update:activeId` 改调 `requestLeaveYamlEditor`；返回按钮由 App.vue 传一个处理函数代替裸 `@back`。
- `handleClose` 与 `Application.Quit` 前同样先过这道闸（窗口 X 的拦截需 Wails `OnBeforeClose`，属主控统筹）。

---

### C-03 两处写同一份 AppBehavior：设置页整对象保存会顶掉侧栏刚改的明暗/主题

**位置**：store.ts:134-145（侧栏路径）、Settings.vue:170-216（设置页路径）、utils/theme.ts:448-455、app.go:538-541（整对象替换语义）

```ts
// store.ts —— 侧栏明暗开关：读最新 → 改一个字段 → 整体写回
134| export async function setThemeMode(mode: ThemeMode): Promise<void> {
135|   applyThemeToApp({ ...themeState.config, mode });
136|   try {
137|     const current: any = await App.GetAppBehavior();
138|     if (!current) return;
139|     current.themeMode = themeState.config.mode;
140|     await App.SaveAppBehavior(current);
```

```ts
// Settings.vue —— behavior 是挂载时的快照，只在 loadBehavior/reset 时刷新
170| const loadBehavior = async () => {
172|     const value = await App.GetAppBehavior();
183|       behavior.value = value;
202| const saveBehavior = async () => {
204|     await App.SaveAppBehavior(behavior.value);   // 整份快照写回
```

```go
// app.go —— 整对象替换，不做字段级 merge
538| func (a *App) SaveAppBehavior(config AppBehavior) error {
539|   if err := a.core.SaveAppBehavior(config); err != nil {
540|     return err
```

**为什么是问题**

`SaveAppBehavior` 是**整对象替换**，而 Settings.vue 的 `behavior` 是**长生命周期快照**：进设置页时 `GetAppBehavior` 一次（:172），之后除 `loadBehavior()`（reset 或重进设置页）外不再刷新。侧栏抽屉是常驻组件（App.vue:61），`setThemeMode` 写的是"最新值"。两条写路径互不知情。

触发路径（无需竞态，100% 可复现）：

1. 进「设置」（快照 `themeMode="light"`）。
2. 点侧栏明暗按钮 → 界面变暗，后端 `themeMode="dark"`。
3. 停在「设置 → 常规/行为」，随便拨一个开关（SettingsBehavior.vue 每个 `@change` 都 `$emit('save')`）。
4. `saveBehavior` 把快照整份写回 → 后端 `themeMode` 又变回 `"light"`。
5. 下次进设置页（`loadBehavior` → `syncThemeFromBehavior`）或重启 → **界面变亮**。

同根因的第二个入口：SettingsTheme.vue:407 的 `applyThemePreview` 调 `applyThemeToApp(config.value, props.behavior)`，而 `writeThemeToBehavior`（theme.ts:448-455）**把预览值直接写进 props.behavior**。于是"拖动色环预览但没点保存就离开"，之后在别的设置页拨一个开关，会把**未保存的预览值**落盘。

**用户可感后果**：用户已报过的症状——"改了个设置，明暗自己翻回去了"。store.ts:127-133 的注释说明这个问题修过，但只修了"侧栏 → 后端"，没修"设置页快照 → 后端"。

**修复建议**

照 SettingsLan.vue:254-269 已有的正确写法（读最新 → 只覆盖本页字段）改：

```ts
// Settings.vue
const saveBehavior = async () => {
  try {
    const latest: any = await App.GetAppBehavior();   // 先取最新
    const next = { ...latest, ...behavior.value };     // 本页字段覆盖，其余保留最新
    await App.SaveAppBehavior(next);
    behavior.value = next;                             // 快照同步刷新
```

更彻底的做法是后端提供字段级 patch（`SaveAppBehaviorPartial(map[string]any)`），前端只发本页改动的键——取舍见 §7-Q3。

---

### C-04 检查内核更新失败：错误文本被丢弃，UI 零反馈

**位置**：store.ts:1064-1067、SettingsUpdate.vue:87

```ts
1064| Events.On("core-update-check-start", () => { globalState.componentUpdate.checkingCoreUpdate = true; });
1065| Events.On("core-update-check-error", ({ data: err }: { data: string }) => {
1066|   globalState.componentUpdate.checkingCoreUpdate = false;
1067| });
```

```ts
87| const handleUpdateCore = () => { if (!globalState.componentUpdate.checkingCoreUpdate && !globalState.componentUpdate.updatingCore) App.CheckCoreUpdateAsync().catch((e: unknown) => reportError(e, 'update.checkCore')); };
```

**为什么是问题**

- 后端链路：`CheckCoreUpdateAsync`（updater_orchestrator.go:158）→ `Tasks.Run("core-update-check", true, fn)` → fn 里 `CheckLatestCore` 失败直接 `return err`（:166-169）→ tasks/manager.go:103 执行 `m.emit(name+"-error", err.Error())`，即 `core-update-check-error` **带着完整错误文本**（超时 / 403 / DNS 失败）。
- 前端监听器把 `err` 解构出来后**完全没用**，只复位布尔；全前端仅此一处监听，SettingsUpdate.vue 也没有第二消费者。
- SettingsUpdate.vue:87 的 `.catch()` 永远不触发：Go 侧 `CheckCoreUpdateAsync()` 无返回值，Wails 恒 resolve。

**用户可感后果**：断网/被墙时点「检查内核更新」，按钮转 12 秒（`context.WithTimeout(12s)`）后恢复，界面一个字都没有。用户无法区分"已最新"和"检查失败"，只会反复点。日志里只有一行 task failed（用户看不到）。

**修复建议**

```ts
1065| Events.On("core-update-check-error", ({ data: err }: { data: string }) => {
1066|   globalState.componentUpdate.checkingCoreUpdate = false;
1067|   const text = String(err || '');
1068|   reportError(new Error(text || 'core-update-check-error'), 'component.checkCore');
1069|   showToast(t('settings.update.checkFailed', { error: displayErrorMessage(text) }), 'error', { duration: 5000 });
1070| });
```

需新增 i18n key `settings.update.checkFailed`（A4 的 i18n 治理已接入 build，8 语言同步即可）。

---

### C-05 「测速全部」启动失败静默：proxy-test-finished 的载荷被整个忽略

**位置**：Proxies.vue:769-780（消费侧）、delay_manager.go:476-524（生产侧）

```ts
769| unsubFinish = Events.On("proxy-test-finished", () => {
770|   isTesting.value = false;
771|   frozenSortedNodes.value = null;
774|   localGroups.value.forEach(g => {
775|     if (!g.proxies) return;
776|     g.proxies.forEach((n: any) => { n.testing = false; });
779|   });
780| });
```

```go
476| finishMsg := "测速完成"
478| defer func() {
489|   if !opts.SilentUI {
490|     m.emit.Emit("proxy-test-finished", finishMsg)
491|   }
498| }()
500| cleanup, _, err := m.ctrl.EnsureDelayCore(ctx)
501| if err != nil { finishMsg = "测速启动失败：" + err.Error(); return }
508| topo, err := buildDelayTopology()
509| if err != nil { finishMsg = "读取代理拓扑失败：" + err.Error(); return }
521| if len(targets) == 0 { finishMsg = "没有可测速的有效节点"; return }
```

**为什么是问题**

- app.go:1289 是 `go a.core.Delay.TestAllProxies(...)` —— **协程 + 无返回值**，前端那个 `await App.TestAllProxies()` 恒 resolve；真正失败只能通过 `proxy-test-finished` 的字符串载荷表达。
- 前端监听器**不接参数**，四种失败原因（内核起不来 / 拓扑读不到 / 无可测节点 / DELAY_TEST_BUSY）全部丢弃，只做 UI 复位。

**与历史 F-A4-10 的关系（明写，避免重复计数）**：F-A4-10 修的是 `await App.TestAllProxies()` **被 reject** 的分支（现 Proxies.vue:678-687 已有 reportError + toast）。但该分支在 Go 侧恒不触发（void + go 协程），**真正的失败分支今天仍然静默**。本报告按新问题记 C-05，请主控决定是"重开 F-A4-10"还是记为派生条目。

**用户可感后果**：内核没起来时点「测速全部」→ 按钮从"停止"变回"测速本组"，卡片延迟值毫无变化，没有任何提示。用户只会认为"点了没反应"。

**修复建议**

```ts
769| unsubFinish = Events.On("proxy-test-finished", ({ data: msg }: { data: string }) => {
770|   isTesting.value = false;
771|   frozenSortedNodes.value = null;
      /* ...原有清理... */
781|   const text = String(msg || '');
782|   if (text && text !== '测速完成') {
783|     reportError(new Error(text), 'proxies.testAllFinished');
784|     showToast(t('proxies.speedTestAborted', { error: text }), 'error', { duration: 5000 });
785|   }
786| });
```

更彻底的做法：后端把 finishMsg 改成 `map{ok bool, message string}` 并同步改 `v3_events.go:79` 的 `RegisterEvent[string]`（类型必须一致，否则事件会被 wails cancel），属跨文件改动，见 §7-Q2。

---

### C-06 「停止 → 立刻重测」：迟到的 finished 事件把新一轮测速状态清掉

**位置**：Proxies.vue:638-663、Proxies.vue:769-780；后端 app.go:1289、delay_manager.go:428-433

```ts
638| const toggleTestAllDelays = async () => {
639|   if (isTesting.value) {
640|     await stopTesting();
641|     return;
642|   }
643|   await testAllDelays();
646| const stopTesting = async () => {
647|   try { await App.StopDelayTest(); }
649|   catch (e) { reportError(e, 'proxies.stopTest'); }
651|   finally { isTesting.value = false; frozenSortedNodes.value = null; /* ...清 testing... */ }
```

```go
1289| func (a *App) TestAllProxies(nodeNames []string) {
1290|   go a.core.Delay.TestAllProxies(a.ctx, nodeNames)
1291| }
428| func (m *DelayTestManager) TestAllProxies(ctx context.Context, nodeNames []string) {
430|   m.cancelBatchAndWait(ctx, 600*time.Millisecond)   // 先等旧批次的 defer 跑完
432|   m.TestAllProxiesWithOptions(ctx, nodeNames, manualDelayOptions(len(nodeNames)))
```

**为什么是问题**

`StopDelayTest`（delay_manager.go:296-303）只做 `cancel()`，**不等待**；前端 `stopTesting` 也不等待，`finally` 立刻 `isTesting=false`。此时被判取消的批次 A 仍在 defer 里活着，会发一次 `finished(A)`（`Emit` 在 `close(done)` 之前，:490 → :497）。

连点两下的时序：

1. t0：点停止 → A 被 cancel，`isTesting=false`（UI 已复位）。
2. t1：再点开始 → 前端 `isTesting=true` 并调 `TestAllProxies` → 后端在 `cancelBatchAndWait` 里等 A 的 defer → A 发 `finished(A)`。
3. t2：`finished(A)` 到达前端 → 监听器把 `isTesting` 清成 false，并抹掉所有节点 `testing` —— **而批次 B 才刚开始**。
4. 结果：按钮从"停止测速"变回"测速本组"；用户再点，`TestAllProxies` 又会 `cancelBatchAndWait` 把 B 取消再起 C。

证据等级 B：第 3 步依赖 Wails IPC 到 JS 的投递顺序（emit 顺序确定，跨事件名的到达顺序未实测）。

**用户可感后果**："想重测一遍"的常见操作（停止 → 再点一次）会让按钮状态错乱，可能连续取消/重启批次，表现为"转圈闪一下就没了""按钮点了没反应"。节点卡片会被后续 `proxy-test-start` 重新点亮，属**状态错乱而非功能全废**。

**修复建议**

前端加 epoch 守卫（改动最小）：

```ts
let testEpoch = 0;
const testAllDelays = async () => {
  const myEpoch = ++testEpoch;
  isTesting.value = true;
  /* ... */
};
// 监听器里第一行：
// if (myEpoch !== testEpoch) return;   // 迟到的旧批次结果直接丢弃
```

或后端给 `finished` 带 batch id（可与 C-05 的载荷改造一次做完）。

---

### C-07 26 个绑定方法前端零调用（占 108 的 24%）

**位置**：frontend/bindings/goclashz/*.ts 与 frontend/src/**（脚本对账：bindings 全集 减 App.X( 调用名集合）

```
AppService  (9) : GetStartupTaskInfo, RepairStartupTask, RepairCoreLayout,
                  RepairDataDirMigration, RepairDataDirPermission,
                  ShowMainWindow, ToggleMainWindow, SaveUwpExemptions,
                  UninstallHelperService
CoreService (17): AddRule, FlushFakeIP, GetConfigFilePath, GetCoreVersion,
                  GetCustomRules, GetEditableConfigs, GetOutboundIP,
                  GetProxyDelay, GetRuntimeAssetRepairProgress,
                  IsConfigEditable, RepairRuntimeAssets,
                  RepairRuntimeAssetsAsync, SaveCustomRules,
                  SaveRuleSection, SearchLogs, StartClash, SyncRules
UpdateService (0): 全部 15 个都被调用
```

**为什么是问题**

1. **配套死 API**：RepairRuntimeAssetsAsync 与 GetRuntimeAssetRepairProgress 是明确成对设计的（app.go:894 注释写"UI 轮询 GetRuntimeAssetRepairProgress"、app.go:960 写"前端轮询"），但两个都零调用 → 这套"异步修复 + 进度"能力**完全不可达**；同步版 RepairRuntimeAssets 同样零调用。结果：资产修复只能靠启动期自动维护，用户在诊断页看到"缺失/损坏"**没有任何修复按钮**（SettingsDiagnostics.vue 只展示）。
2. **废弃 API 仍在绑定里**：SyncRules 恒返回"规则同步功能已废弃"（app.go:1000-1002）；SaveCustomRules 对 remote 配置直接报"旧规则保存接口已废弃"（app.go:991-996）；它们与 GetCustomRules / AddRule / SaveRuleSection 已被 GetRulePageData / AddRuleFromForm / DeleteRule 取代，却仍留在 108 个方法里，任何人接到 App. 自动补全就会踩。
3. **配置可编辑性检查没有入口**：IsConfigEditable / GetEditableConfigs 零调用 → 编辑器打开任何配置都直接可编辑，与后端"哪些能改"的判定脱节。
4. **StartClash 零调用**：目前"启动内核"只能通过 TUN/系统代理开关或 SelectLocalConfig 间接触发，没有独立入口。是否为设计意图需主控确认（§7-Q4）。

**修复建议**

分三类处理：(a) 明确废弃 → 从 service 结构体删方法（services_v3_generated.go 一并清理）并重新生成绑定；(b) 应暴露但漏了 UI → 诊断页补"修复运行组件"按钮（RepairRuntimeAssetsAsync + 轮询）；(c) 保留备用 → 在 services_v3_generated.go 对应方法上加 Deprecated 注释，便于下一轮审计直接跳过。

---

### C-08 12 个后端事件前端零监听（历史 F-A5-06 / F-A5-23 未点名的那部分）

**位置**：发射点见下表；消费侧 = 无

| 事件 | 发射点 | 后端语义 | 前端后果 |
| --- | --- | --- | --- |
| proxy-test-progress | delay_manager.go:629 | 批量测速进度 current/total | 批量测速**没有进度条**，只有转圈 |
| config-file-saved | config_editor_controller.go:20 | 配置写盘成功 id | 无消费者（保存反馈全靠 RPC resolve） |
| geo-update-active-sync | geo_update_manager.go:117 | 正在更新的库 key 列表 | 无法显示"哪些库正在下载" |
| geo-update-<key>-start | geo_update_manager.go:139 | 单库开始 | 无 |
| geo-update-<key>-busy | geo_update_manager.go:233 | 同库已在更新，请求被合并 | **用户点"更新"没有任何反应** |
| geo-update-<key>-cancelled | geo_update_manager.go:154/198 | 已取消 | 无 |
| geo-update-all-start | geo_update_manager.go:264 | 全部更新开始 | 「全部更新」点下去无 toast、无进度 |
| geo-update-all-success | geo_update_manager.go:335 | 全部成功 | 成功也无提示（只能看任务行） |
| core-update-check-success | tasks/manager.go:109 | 检查任务成功 | 可接受（随后必发 none/available 复位） |
| core-update-check-cancelled | tasks/manager.go:99 | 检查被取消（连点会取消第一次） | 见下"风险评估"，不卡死 |
| subs-update-cancelled | tasks/manager.go:99 | 订阅批量被取消 | 见下"风险评估"，不卡死 |
| driver-install-start | tasks/manager.go:91 | 驱动安装开始 | isInstalling 由 task-sync 派生，可接受 |

**逐条风险评估（为什么定 P2 而不是 P1）**

- proxy-test-progress：纯体验缺失（无进度条），不影响正确性。
- geo-update-<key>-busy：唯一有"静默"味道的一条。但同库更新中时 isUpdatingDb()（SettingsUpdate.vue:73 读 tasks[key].status）会把按钮置灰，**可达性低**；仍建议补一条 info toast。
- core-update-check-cancelled / subs-update-cancelled：我特意追了会不会把 UI 卡死。
  - checkingCoreUpdate：连点第二次 → 第一任务被取消并发 cancelled（无消费者）→ 但第二任务的 -start 立即把布尔重新置 true，它的成功路径必发 core-update-none 或 core-update-available（updater_orchestrator.go:182/194），error 路径发 -error（已消费）→ **不会卡死**。
  - isUpdating（Subscriptions.vue:219）：subs-update 走 core/tasks.Manager，而 CancelUpdateTask 只操作 UpdateTasks（key 仅 geoip/geosite/mmdb/asn/core-update/driver-install），**用户无法取消 subs-update**，只有退出应用时 ctx 才被取消 → **实际不可达**。
  - 结论：这两条只是契约不完整，不是活 bug，故定 P2 而非 P1。

**修复建议**：按收益排序 —— ① proxy-test-progress 接上（批量测速进度条，成本约 5 行）；② geo-update-<key>-busy 接一条 info toast；③ 其余在 v3_events.go 标注"暂无意向消费者"或从注册表移除（注意 strictevents 构建下打 warning，生产无影响）。

---

### C-09 NOTIFICATION_UI：15 条里 7 条是死键（4 条还是 silent 级）

**位置**：store.ts:936-973

```ts
936| const NOTIFICATION_UI: Record<string, NotificationUi> = {
938|   ASSET_WINTUN_BUSY: 'silent',
939|   ASSET_CORE_PROBE_TIMEOUT: 'silent',
940|   HELPER_WAKE_TIMEOUT: 'silent',
941|   TUN_ADAPTER_PENDING: 'silent',
943|   ASSET_SEED_UNAVAILABLE: 'toast',
948|   STARTUP_TASK_CHECK_WARNING: 'toast',
949|   STARTUP_TASK_UNHEALTHY: 'toast',
951|   STARTUP_RESTORE_HELPER_REQUIRED: 'modal',
965|   const byCode = code ? NOTIFICATION_UI[code] : undefined;
966|   if (byCode) return byCode;
```

**为什么是问题**

- 全仓 grep 这 7 个 code 在 **Go 侧 0 引用**。后两个（STARTUP_TASK_CHECK_WARNING / STARTUP_TASK_UNHEALTHY）的真实通道是独立事件（startup-task-check-warning / -unhealthy，store.ts:1044/1050 已消费），写在这张表里属重复。
- 危险在语义：其中 4 条被映射成 silent（只 reportError、不弹任何 UI）。这些 code 是照着方案错误码表预写的，**一旦后端按方案补上同名 code，通知会被静默吞掉**。
- 反向：后端 23 个真实 code 里 15 个不在表内，但都走 level 兜底（error→modal、其余→toast），**结果正确**，不构成功能缺陷。

**修复建议**：删除 7 个死键，剩下 8 条有效键补注释说明出处（文件:行）；把这批判定迁到 utils/notification.ts 并给纯函数 resolveNotificationUi 补单测（成本极低）。若要保留 silent 分类，应加"仅 debug 日志"开关而不是无条件静默。

---

### C-10 updateStateFromBackend 的 PascalCase 兼容分支是死代码（约 24 处）

**位置**：store.ts:325-437（代表点 :330-335 / :343 / :349 / :354 / :363-364 / :370-378 / :391-437）

```ts
329|   if (rawData.isRunning !== undefined) globalState.isRunning = rawData.isRunning;
330|   else if (rawData.IsRunning !== undefined) globalState.isRunning = rawData.IsRunning;
...
343|   const newHideLogs = rawData.hideLogs ?? rawData.HideLogs;
```

**为什么是问题**：后端 AppState（core/appcore/controller.go:519-556）的 json tag **全部 camelCase**，Wails 序列化后 key 即 camelCase，因此所有 rawData.PascalCase 分支**永不命中**，是纯死代码。副作用有两条：① 制造"契约是灵活的"错觉，后来人会以为要兼容另一种序列化形式；② :363 的 actualSystemProxy ?? systemProxy ?? SystemProxy 让"actual 优先、旧字段兜底"这条本应清晰的语义变模糊。

**修复建议**：删掉全部 else if (rawData.Xxx) 与 ?? rawData.Xxx，直接读 camelCase；并把函数入参从 any 改成 bindings 导出的 AppState（或 Partial<AppState>），让 TS 兜住字段名。注意保留"未定义则不覆盖"的语义（app-state-sync 是完整快照，可整体赋值 + 3 个特例处理）。

---

### C-11 3 处 localStorage.setItem 无 try/catch，异常会中断整段状态同步

**位置**：store.ts:346、store.ts:402、store.ts:405

```ts
343|   const newHideLogs = rawData.hideLogs ?? rawData.HideLogs;
344|   if (newHideLogs !== undefined) {
345|     globalState.hideLogs = newHideLogs;
346|     localStorage.setItem('goclashz_hideLogs', String(newHideLogs));
347|   }
400|   if (rawData.activeConfig !== undefined) {
401|     globalState.activeConfigId = rawData.activeConfig;
402|     localStorage.setItem('goclashz_activeConfigId', rawData.activeConfig);
```

**为什么是问题**：同一文件另有两处**都包了 try/catch**（:116-120 的 applyThemeToApp、:208-212 的 setProxyCardMode），说明作者知道 localStorage 可能抛（隐私模式/配额/策略）。这里 3 处没包，而 updateStateFromBackend 是 app-state-sync 的唯一入口（store.ts:991-993）：一旦 :346 抛异常，后面的 controlRevision / desired* / version / activeConfig / language **全部不执行**，异常从 Events.On 的 handler 冒出（无 catch）→ 状态停更，且 controlRevision 没推进会让后续 revision 比较出错。证据等级 B（需 localStorage 抛异常这个前提，WebView2 下罕见但非零）。

**修复建议**：抽 safeSetItem 统一替换，并给事件回调加顶层 try/catch：

```ts
function safeSetItem(key: string, value: string) {
  try { localStorage.setItem(key, value); } catch { /* 隐私模式/配额满：忽略 */ }
}
// store.ts:991
Events.On("app-state-sync", ({ data: newState }: { data: any }) => {
  try { updateStateFromBackend(newState); } catch (e) { reportError(e, 'state.sync'); }
});
```

---

### C-12 手抄两份契约类型，未复用 bindings 导出

**位置**：store.ts:23-36（UpdateTaskState）、store.ts:38-66（OutboundIPResult）

```ts
23| export type UpdateTaskState = {
24|   key: string; title: string; status: string; stage: string;
25|   progress: number; bytesDone: number; bytesTotal: number;
26|   speedBps: number; etaSeconds: number; error: string;
27|   startedAt: number; finishedAt: number;
28| };
38| export type OutboundIPResult = { preferred: string; ipv4: string; ... };
```

**为什么是问题**：bindings 已有同名导出（bindings/goclashz/core/appcore/models.ts 的 UpdateTaskState、core/sys/models.ts 的 OutboundIPResult）。手抄版一旦与后端不同步，就会出现"编译期通过、运行时字段空"的隐性故障（与 C-10 同类）。而 normalizeOutboundIP(raw: any) 用 any 入参，也主动放弃了这层保护。

**修复建议**：

```ts
import type { UpdateTaskState } from '../bindings/goclashz/core/appcore/models';
import type { OutboundIPResult } from '../bindings/goclashz/core/sys/models';
export type { UpdateTaskState, OutboundIPResult };
function normalizeOutboundIP(raw: Partial<OutboundIPResult> | null | undefined): OutboundIPResult { /* ... */ }
```

注意 store.ts 的 UpdateTaskState 还被 UpdateTaskRow.vue:41 以 import type 引用，改导出位置后要同步改 import 路径（或保持 re-export 即可，如上）。

---

### C-13 日志列表用索引做 key，且被 splice 裁剪

**位置**：App.vue:154（模板）、App.vue:526-539（批量合并 + 裁剪）

```vue
154| <div v-for="(log, i) in filteredLogLines" :key="i" :class="['log-line', log.type]">
```

```ts
529|   logLines.value.push(...logBatch);
531|   const overflow = logLines.value.length - LOG_MAX_LINES;
532|   if (overflow > 0) logLines.value.splice(0, overflow);
```

**为什么是问题**：:532 每次从**头部**裁掉 N 行，而 key 是"当前下标"——Vue 会把旧 DOM 节点全部保留、只更新文本，等于**整表重渲染**（上限 2000 行，每 16ms 一批）。而且 filteredLogLines 是过滤后的派生数组，下标与 logLines 更不对应。视觉不会错（文本会被正确更新），代价是"core 开 debug 时日志页掉帧"——正是 :494-496 注释想解决的问题，说明该优化未到位。

**修复建议**：给每条日志稳定 id（后端 LogEntry 已有 ts+pid，前端入库时补自增 seq）：

```ts
let logSeq = 0;
const enqueueLogLine = (log: any) => { logBatch.push({ ...log, __seq: ++logSeq }); /* ... */ };
// 模板： :key="log.__seq"
```

---

### C-14 四个设置页的自动保存没有 in-flight 互斥 / 序号

**位置**：SettingsLan.vue:254-278、SettingsNetwork.vue:336-348、SettingsDns.vue:236-244、SettingsTun.vue:192-212

```ts
// SettingsNetwork.vue
336| const saveNet = async () => {
337|   if (loading.value) return;
338|   try {
339|     await App.SaveNetworkConfig(netConfig.value as any);   // 整对象写
341|   } catch (e) { /* toast */ }
```

```ts
// SettingsTun.vue —— 每个控件的 @change 都直接 saveTun
212| const saveTun = async () => { await saveTunCore(false); };
192| const saveTunCore = async (force: boolean = false): Promise<boolean> => {
193|   if (loading.value && !force) return false;
195|     await App.SaveTunConfig(tunConfig.value);
```

**为什么是问题**：四个页面都是"改一个控件 → 立刻整对象写盘"，saveX() 之间没有 in-flight 标记也没有请求序号。tunConfig.value 是同一个响应式对象，连续两次修改会并发发出两个 RPC；Wails 侧调用顺序可靠，但 **Go 内部对同一个 user_*.json 是两次独立的原子替换**，若第一次排队/慢，后发的可能先落盘，最终磁盘停在"中间态"。证据 B。

另一个不一致点：SettingsLan.save()（:254-269）已经做对了（先 GetNetworkConfig 再 merge 自己的字段），而 SettingsNetwork.saveNet() 是**整对象写**——同一个 NetworkConfig 上两个页面对"谁的字段"的边界不一致；虽然两页互斥挂载缓解了风险，但语义不统一。

**修复建议**：加"最后一个请求胜出"的写队列（比互斥更符合自动保存语义）：

```ts
let saveSeq = 0;
const saveNet = async () => {
  if (loading.value) return;
  const my = ++saveSeq;
  try {
    await App.SaveNetworkConfig(netConfig.value as any);
    if (my !== saveSeq) return;        // 已有更新的保存，忽略本次回执
  } catch (e) { /* ... */ }
};
```

更根本的做法：后端 SaveNetworkConfig / SaveTunConfig / SaveDNSConfig 接受部分字段（patch），前端只发本页字段，从源头消除整对象覆盖（与 C-03 同一条后端改造，见 §7-Q3）。

---

### C-15 Proxies.loadData 有 6 个并发入口且无序化

**位置**：Proxies.vue:526-588

```ts
526| const loadData = async () => {
529|     const bh = await App.GetAppBehavior();
538|     const data: any = await App.GetInitialData();
540|     if (data && data.groups) {
578|       localGroups.value = processedGroups;              // 整体覆盖
581|       if (!isCurrentValid && processedGroups.length > 0) {
582|         currentGroup.value = processedGroups[0].name;   // 可能把用户在看的分组切走
```

**为什么是问题**：调用者有 6 个 —— onMounted(:809)、watch(isRunning)(:622)、watch(activeConfigId)(:630)、config-changed 事件(:765)、proxy-state-sync 发现未知组(:790)、selectNode 后(:607)。而 GetInitialData 内部要读 YAML + 打内核 API，耗时可达数百 ms。两个请求交错时**先发后到**的快照会覆盖新结果，并把 currentGroup 重置成 processedGroups[0]。证据 B（需两次调用时间重叠；快速切节点 + 切配置时容易撞上）。

**修复建议**：与 C-14 同款"序号 + 丢弃过期结果"；:581-583 的"当前组不存在才回退"本身写法正确，只要保证只有最新快照能写 localGroups：

```ts
let loadSeq = 0;
const loadData = async () => {
  const my = ++loadSeq;
  /* ...两个 await... */
  if (my !== loadSeq) return;      // 迟到的旧快照直接丢弃
  if (data && data.groups) { /* ... */ }
```

---

### C-16 UpdateTaskPanel：deep watch 每次任务变更都新建 3s 定时器，卸载不清理

**位置**：UpdateTaskPanel.vue:39-46

```ts
39| watch(tasks, (newTasks) => {
40|   const hasFinished = newTasks.some(t => t.status === 'success');
41|   if (hasFinished) {
42|     setTimeout(() => {
43|       clearFinishedTasks();
44|     }, 3000);
45|   }
46| }, { deep: true });
```

**为什么是问题**：① 没有 clearTimeout、没有去重：只要列表里存在一条 success，**每一次任务变更（下载进度每秒多次）都会再排一个 3s 定时器**，3 秒内可排十几个，随后连发十几个 ClearFinishedUpdateTasks RPC；② 组件卸载后定时器仍会触发（对已卸载组件无副作用，但 RPC 照发）；③ deep: true 加在"数组 of 对象"的 computed 上，每次进度更新都要深遍历。

**修复建议**：

```ts
let clearTimer: number | null = null;
watch(
  () => Object.values(globalState.componentUpdate.tasks).some(x => x.status === 'success'),
  (has) => {
    if (!has || clearTimer !== null) return;
    clearTimer = window.setTimeout(() => { clearTimer = null; void clearFinishedTasks(); }, 3000);
  }
);
onUnmounted(() => { if (clearTimer !== null) clearTimeout(clearTimer); });
```

（产品层面也建议：成功行 3 秒后自动消失容易让人以为"没成功"，改成用户手动关闭更稳。）

---

### C-17 ModernSelect 挂载即程序化 emit change（潜在自动保存源）

**位置**：ModernSelect.vue:55-61

```ts
55| watch(() => props.modelValue, (newVal) => {
56|   const match = props.options.find(o => o.value === newVal);
57|   if (!match && props.options.length > 0) {
58|     emit('update:modelValue', props.options[0].value);
59|     emit('change', props.options[0].value);       // 自动修正也发 change
60|   }
61| }, { immediate: true });
```

**为什么是问题**：change 在四个页面都等于"落盘"（SettingsTun.vue:27/39、SettingsBehavior.vue:155/170/183 等）。于是"值不在选项里"就会**在用户没动任何东西时触发一次后端写入**，immediate 让它挂载即跑一次。

**当前全部被兜住**（这是定 P2 而非 P1 的理由）：SettingsTun 的 saveTunCore 有 loading 闸门（:193），数据加载期间 loading=true；加载完成后 modelValue 必然已是合法选项（:144-147 另有 mips→mixed 的显式回退）。SettingsDns / SettingsNetwork 的 saveX 同样有 if (loading.value) return。

**风险**：任何人以后把某个 select 的 change 接到"不需要 loading 闸门"的动作（例如立即重启内核），就会变成"打开设置页就自动重启"。

**修复建议**：自动修正分支只 emit update:modelValue，不 emit change（把"值被纠正"与"用户改了值"分开）：

```ts
watch(() => props.modelValue, (v) => {
  if (v === undefined || v === null || v === '') return;   // 未加载完不纠正
  if (props.options.length === 0) return;
  if (props.options.some(o => o.value === v)) return;
  emit('update:modelValue', props.options[0].value);
}, { immediate: true });
```

---

### C-18 YamlEditor 在 await 之后注册全局监听 → 卸载竞态泄漏

**位置**：YamlEditor.vue:367-386（配合 onUnmounted :388-392）

```ts
367| onMounted(async () => {
368|   await loadConfig();                        // 读盘 + 大文件，可能数百 ms
369|   document.addEventListener('keydown', handleKeydown);
370|   themeObserver = new MutationObserver(() => { /* dispatch 到 editorView */ });
382|   themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
386| });
388| onUnmounted(() => {
389|   editorView.value?.destroy();
390|   document.removeEventListener('keydown', handleKeydown);
391|   themeObserver?.disconnect();
```

**为什么是问题**：Vue 的 onMounted(async) 不会因卸载而中断。若用户在 loadConfig 未完成时切走（切 tab 会销毁组件），顺序是 onUnmounted（此时 themeObserver 还是 null、keydown 尚未注册）→ 然后 await 恢复 → 注册 keydown + MutationObserver。这两个监听**永远没人清理**：

- Ctrl+S 会在任何页面调 handleSave()，写的是已销毁组件的旧 props.configId；
- MutationObserver 每次主题变化都 dispatch 到已 destroy() 的 EditorView（:371 的守卫是 if (editorView.value)，而 onUnmounted 没把它置空 → 仍会 dispatch，CodeMirror 抛错）。

**修复建议**：

```ts
let disposed = false;
onMounted(async () => {
  await loadConfig();
  if (disposed) return;                      // 关键
  document.addEventListener('keydown', handleKeydown);
  /* ... */
});
onUnmounted(() => {
  disposed = true;
  editorView.value?.destroy();
  editorView.value = undefined;              // 关键：让 updateListener 的守卫失效
  document.removeEventListener('keydown', handleKeydown);
  themeObserver?.disconnect();
  themeObserver = null;
});
```

证据等级 B（需"加载未完成就切页"这个窗口；慢盘 / 大配置下可达数百 ms）。

---

### C-19 模式滑块：mode 不在三值内时高亮条移出容器

**位置**：Overview.vue:95-103

```ts
95| const modes = computed(() => [ rule / global / direct ]);
101| const sliderStyle = computed(() => ({
102|   left: (modes.value.findIndex(m => m.val === globalState.mode) * 100) / 3 + '%'
103| }));
```

**为什么是问题**：globalState.mode 来自 AppState.Mode = behavior.ActiveMode（controller.go:585），而 UpdateClashMode **不做枚举校验**（controller.go:2225 起直接写入并持久化）。配置里的 mode: script 或任何历史遗留值都会被原样带上来 → findIndex 返回 -1 → left = -33.33% → 高亮块滑出左侧，界面呈现"三个选项都没选中 + 一个色块露在边上"。

证据等级 B（需要 mode 为非三值：当前 UI 只能写三值，但配置导入 / 备份还原 / 旧版本可写入其它值）。

**修复建议**：

```ts
const modeIndex = computed(() => Math.max(0, modes.value.findIndex(m => m.val === globalState.mode)));
const sliderStyle = computed(() => ({ left: (modeIndex.value * 100) / 3 + '%' }));
```

并在 updateStateFromBackend 里对 mode 做白名单归一（['rule','global','direct'].includes(m) ? m : 'rule'）。

---

### C-20 路由切换提示以 warn 通知直出中文技术串

**位置**：store.ts:1036-1039（消费）、core/appcore/outbound_ip.go:198-233（生产）

```go
202| if expectedRoute != "" && expectedRoute != currentRoute {
203|   msg := fmt.Sprintf("路由切换中: expected=%s current=%s", expectedRoute, currentRoute)
205|   c.notifyRouteSwitching(msg)     // → notify("warn","ROUTE_SWITCHING",msg,"outbound-ip")
206|   return OutboundIPResult{ Mode: currentRoute, Message: msg }, nil
```

```ts
1038|   const kind = level === 'error' ? 'error' : level === 'success' ? 'success' : 'info';
1039|   showToast(message || t('common.notice'), kind, { duration: 4200 });
```

**为什么是问题**：ROUTE_SWITCHING 不在 NOTIFICATION_UI 里 → 走 level=warn → toast，把后端的**中文调试串**（expected=proxy current=direct）直接显示在 8 语言界面里。它被 notifyRouteSwitching 节流到 5s 一次，只在"路由切换比预期慢"时出现（TUN 适配器慢、系统代理未生效），属间歇性噪音而非轰炸。注意 isRouteSwitchingMessage（utils/errors.ts:127-129）本来就是为了这条语义写的，说明前端有能力识别，只是通知通道没接。

**修复建议**：① 后端 OutboundIPResult 增 code string 字段（现在只靠中文文案，是 F-A4-14 的遗留）；② 前端在 NOTIFICATION_UI 里加 ROUTE_SWITCHING: 'silent'，不要把内部诊断串推给用户。

---

### C-21 t() 永不返回空串，所以 6 处 OR 兜底是死代码

**位置**：locales/index.ts:64-67（事实依据）+ 下列调用点

```ts
64|   // 3. 兜底返回 path 自身，防止白屏
65|   if (result === undefined) {
66|     return path;
67|   }
```

命中点（全部形如 t('x') 后接 OR 中文默认值）：

| 文件:行 | 表达式片段 |
| --- | --- |
| SettingsAbout.vue:306 | t('settings.about.exportSuccess') 后接 "备份成功导出" |
| SettingsAbout.vue:345 | t('settings.about.restoreConfirmDefault') 后接 '确定要执行数据还原吗？' |
| SettingsAbout.vue:348 | t('settings.about.restoreNotice') 后接 '还原完成后…' |
| SettingsAbout.vue:349 | t('settings.about.restoreConfirmTitle') 后接 "还原备份确认" |
| SettingsAbout.vue:359 | t('settings.about.restoreSuccess') 后接 "数据还原成功！…" |
| SettingsDiagnostics.vue:260 | t('settings.diagnostics.exportSuccess')（同模式，无兜底但共用一条调用链） |

**为什么是问题**：t() 找不到 key 时返回 **key 本身**（非空字符串），所以 OR 右侧永远不生效。两层后果：① 死代码；② 更麻烦的是**掩盖缺键**——key 真缺了，用户会看到字面量 settings.about.exportSuccess，而作者以为有中文兜底。A4 的 F-A4-17 已把 i18n 治理接入 build（scripts/check-i18n.mjs），这类兜底正好削弱该治理。

**修复建议**：删掉所有 OR 中文默认值，直接 t('key')，缺键交给 check-i18n.mjs 在构建期报错。若确需"缺键时中文"，用显式 API（例如 tWithFallback(key, '默认文案')，内部判定 t(key) === key）。

---

### C-22 死 import 与死 export

**位置/清单**

死 import（确证：import 存在、函数体 0 次使用）：

| 文件:行 | 符号 |
| --- | --- |
| App.vue:270 | updateStateFromBackend |
| components/YamlEditor.vue:56 | globalState |
| components/TrafficCard.vue:64 | showAlert |
| components/SettingsHome.vue:64 | type SettingField、type SettingPage |
| components/SettingsTheme.vue:272 附近 | themeConfigFromBehavior 及 4 个 type（需人工确认，脚本可能误报） |

死 export（全仓唯一引用点在声明处）：

| 文件:行 | 符号 | 说明 |
| --- | --- | --- |
| settings/manifest.ts:1013 | findSettingById | 与 SETTING_FIELDS.find 重复的能力 |
| utils/format.ts:23 | formatDurationCn | 无调用者（formatEtaTime 在用） |
| utils/glass.ts:13 | GLASS_BLUR_OVERLAY_DEFAULT | 无引用 |
| utils/glass.ts:15 | GLASS_BLUR_CONTROL_DEFAULT | 无引用 |
| utils/glass.ts:44 | normalizeGlassBlurStrength | 无引用（glass.ts 内部另有实现） |
| utils/normalize.ts:1 | normalizeStartupTaskInfo | **整个文件无调用者** |
| utils/report.ts:38 | getRecentErrors | 环形缓冲只写不读 |

**为什么是问题**：utils/normalize.ts 整个模块无消费者却仍进类型检查与打包；getRecentErrors 是最有价值的那个——reportError 已在收集 50 条记录，但没有任何出口，等于"上报了没人看"（report.ts:37 注释自己也写着"供日志面板/诊断以后接入"）。

**修复建议**：删 normalize.ts、glass.ts 的 3 个死导出、formatDurationCn、findSettingById；**不要删 getRecentErrors**，而是接到诊断包导出（ExportDiagnostics 之后追加前端错误清单，或至少"复制诊断信息"时带上），让 reportError 真正可用。死 import 直接删。

---

### C-23 handleClose 未 try/catch：点关闭按钮可能"没反应"

**位置**：App.vue:687-695

```ts
687| const handleClose = async () => {
688|   clearCtrlHover();
689|   const config = await App.GetAppBehavior();
690|   if (config.closeToTray) {
691|     App.HideMainWindow();
692|   } else {
693|     Application.Quit();
694|   }
695| };
```

**为什么是问题**：GetAppBehavior 是一次 RPC（返回完整 AppBehavior）。若它 reject（后端忙 / 服务重启窗口 / Wails 调用失败），异常从 @click 处理器冒出，**后面两条分支都不执行**——用户点 × 完全没反应，也没有任何提示，最容易被当成"程序卡死"。同文件 App.HideMainWindow() 也是浮动调用（未 await / 未 catch）。

证据等级 B（需要 RPC 失败；GetAppBehavior 本身几乎不会失败，故定 P2）。

**修复建议**：兜底 + 失败时用安全默认（关到托盘，而不是静默）：

```ts
const handleClose = async () => {
  clearCtrlHover();
  let closeToTray = true;                        // 与后端默认值一致
  try { closeToTray = !!(await App.GetAppBehavior())?.closeToTray; }
  catch (e) { reportError(e, 'window.close'); }
  if (closeToTray) void App.HideMainWindow();
  else Application.Quit();
};
```

---

## 4. 冗余对照表

| ID | 重复的东西 | 出现位置 | 建议抽到哪 |
| --- | --- | --- | --- |
| R-1 | 三个 namespace 手工展开合并 | api.ts:13-17 | 直接用 bindings/goclashz/index.ts 已导出的 AppService/CoreService/UpdateService；若坚持聚合，加一行构建期断言（Object.keys 交集为空），把"静默覆盖"变成构建失败 |
| R-2 | 契约类型手抄两份 | store.ts:23-36、store.ts:38-50 | 复用 bindings 导出（见 C-12） |
| R-3 | 「自动保存 + 失败 toast」骨架四份 | SettingsLan.vue:254-278、SettingsNetwork.vue:336-348、SettingsDns.vue:236-244、SettingsTun.vue:192-212 | 抽 composables/useAutoSave.ts（参数：加载闸门、save 函数、i18n 前缀、节流窗口），顺带统一实现 C-14 的序号守卫 |
| R-4 | 「rAF 两帧后解除 loading」注释 + 实现五份 | Settings.vue:197-199、SettingsTun.vue:156-158、SettingsDns.vue:230-232、SettingsNetwork.vue:330-332、SettingsLan.vue:248-250 | 抽 utils/frame.ts 的 afterPaint(fn)；那段长注释只保留在一处 |
| R-5 | 滑块指示器逻辑四份（updateXSlider / resetXSlider / watch / nextTick） | App.vue:456-487（日志页）、Proxies.vue:250-286、Connections.vue:246-269、Rules.vue:152-173 | 抽 composables/useTabSlider.ts（track ref + active el ref + 动画就绪标记） |
| R-6 | localStorage 读写与 try/catch 三种写法并存 | 有保护：store.ts:116-120、:208-212、utils/theme.ts:430-438；无保护：store.ts:346、:402、:405、Proxies.vue:152/279/314/318 | 抽 utils/storage.ts 的 safeGet/safeSet 全量替换（顺带修 C-11） |
| R-7 | core-update-success 在同一个函数里注册两次 | store.ts:1088-1091、store.ts:1132 | 合并为一个 handler（复位 UI + 调度 IP 刷新） |
| R-8 | driver-install-success 三处监听 | store.ts:1133、SettingsUpdate.vue:103、SettingsTun.vue:217 | 三者职责不同（IP 刷新 / 资产版本 / TUN 状态），可保留；但建议 store 统一分发一次，组件订阅 store 而非原始事件 |
| R-9 | 后端行为默认值在前端再抄一份大对象 | Settings.vue:166（36 个键）vs core/appcore/behavior.go 的 Default() | 前端不维护默认值：先 await GetAppBehavior()，拿不到就显示 loading（现在这份默认值会与后端漂移，并掩盖"加载失败"） |
| R-10 | i18n 的 t 被局部变量遮蔽 | store.ts:1059（forEach(t => ...)）、Rules.vue:188（map((t: any) => ...)） | 改名为 task / opt，避免同一作用域里 t 既是 i18n 又是数据 |
| R-11 | 复制失败文案用字符串拼接绕过 i18n | App.vue:580、SettingsDiagnostics.vue:249（均为 t('common.copy') 后接固定符号） | 用已有的 common.copyFailed 或新增 key，去掉硬编码符号 |
| R-12 | 「内核版本字符串」两条来源 | GetCoreVersion（废弃、零调用）vs GetRuntimeAssetStatus().assets.core.version（SettingsUpdate.vue:81 在用） | 删除 GetCoreVersion 绑定（见 C-07） |

---

## 5. 已核查且无问题的点（覆盖度证明）

> 本节是"没提 = 没看"的反证。以下每一项都实际对过代码，结论是**不需要改**。

### 5.1 契约结构层

1. **三 namespace 零同名导出、零 ByID 重复**（§1.1）——展开合并不存在覆盖风险。
2. **111 个调用点的实参个数与签名 100% 一致**（0 个 TOO_FEW / TOO_MANY），可选参数无错位传入。
3. **无返回值类型误用**：没有 void 方法取字段、数组当对象、T 或 null 直接解引用。
4. **事件名双向对账**：前端监听 44 个名字 → 后端 0 死监听；拼写（含连字符、大小写）0 处不一致。
5. **9 个模型全字段对齐**：AppState(25) / UpdateTaskState(11) / OutboundIPResult(11) / TrafficSnapshot(8) / LogEntry(13) / Notification(4) / ProxyGroupState(3) / DNSConfig(15)+FallbackFilterConfig(4) / DataDirInfo(29)。int64 与 number、float64 与 number 均无精度越界（本系统没有超过 2^53 的计时字段）；时间统一为 string 或 int64 秒，未出现 time.Time 序列化歧义。
6. **Go null slice 无炸点**：DNSConfig / FallbackFilterConfig 的 8 处数组或 null 字段在模板里全部写了空数组兜底（SettingsDns.vue:83/107/113/119/125/131/158/164）；component-update-tasks-sync 的数组有 Array.isArray 与 forEach 防护（store.ts:1058）。

### 5.2 后端语义核对（任务书点名的 18 个方法）

7. **DeleteRule 的 index 语义正确**：后端对 local 用 rules[index]、对 subscription 用 visibleRules[index]、对 add/delete 用 overlay 数组下标（app.go:1113-1179）；前端在过滤与分页之后仍通过 originalIndex 传真实下标（Rules.vue:443、:43、:497）——**这是最容易写错的一处，实际写对了**。
8. **AddRuleFromForm 的 section 语义正确**：前端 local/remote 两分支 ↔ 后端 local/subscription/add/delete 四分支（Rules.vue:453-462 ↔ app.go:1057-1090）；BuildRuleRequest 的 type/payload/policy 与 Go struct 完全一致。
9. **TestProxy 是同步方法，前端 await + finally 时序正确**：Go 侧先 emitDelayResult 再 return（delay_manager.go:730-736），前端 finally 复位 spinner 不会抢跑。
10. **TestAllProxies 的单次点击互斥成立**：isTesting 同步置位 + toggleTestAllDelays 分支（Proxies.vue:638-644）；后端 manualDelayOptions 的 SilentUI=false 保证 finished 必发（delay_manager.go:131）。只有"停止后立刻重测"这一种时序有问题（C-06）。
11. **TUN 两个入口已统一**：Overview 与 Settings→TUN 共用 useTunToggle，pending 共用 globalState.tunPending（useTunToggle.ts:65），store 另有 60s 看门狗兜底复位（store.ts:1190-1217）。跨入口重复点击被正确拦住。
12. **系统代理开关的 pending + 三层 intent 模型正确**（Overview.vue:232-267、store.ts:175-203），失败时回滚 desiredSystemProxy。
13. **模式切换有 worker 队列**（Overview.vue:281-320）：modeWorkerActive + pendingModeTarget，连点不会并发调用 UpdateClashMode，失败回滚 previousMode。
14. **SelectLocalConfig 的乐观更新有 nonce 校验**（Subscriptions.vue:284-317），旧请求不会覆盖新选择。
15. **SelectProxy 的失败会被消费**（Proxies.vue:595-619），成功后重新 loadData 而不是本地乐观改 now。
16. **SaveConfigText 的失败处理完整**（YamlEditor.vue:307-329）：状态栏 + emit status-change + showAlert 三处都有；isModified=false 时也不会重复保存。
17. **配置可编辑性后端有把关**：SaveConfigText 走 Strict 路径校验（config_editor_controller.go:13-16），不会出现"改了但写不进去还报成功"。
18. **CheckTunEnv 的 map 载荷与前端取键一致**（SettingsTun.vue:112-121 的 applyTunStatus 同时兼容 GetRuntimeAssetStatus 与 CheckTunEnv），字段 hasWintun/wintun/supportsMips 与 app.go:747-753 一致。
19. **GetOutboundIPForRoute 的 expectedRoute 语义与前端一致**：不匹配时返回 error=nil + Message 带固定中文（outbound_ip.go:202-210），前端 isRouteSwitchingMessage 据此保留旧值（store.ts:859-864），不会把路由切换误判为检测失败。
20. **ExportBackup / ExecuteRestore 的返回值语义与前端判断一致**：Go 返回 SUCCESS / CANCELLED / error（app.go:1441-1492），前端只认 SUCCESS（SettingsAbout.vue:305/357），取消静默、失败走 catch——**不是**"把失败当成功"。
21. **SelectBackupFile / SelectLocalFile 取消时返回空值不报错**（app.go:1476-1482、:845-853），前端都做了路径判空（SettingsAbout.vue:324-327、Subscriptions.vue:357-363）。
22. **GetInitialData 的各失败分支都返回非 nil map**（app.go:317-348），前端 data 与 data.groups 判空成立，Proxies 不会因内核未启动而崩。
23. **GetNetworkConfig 永不返回 nil**：GetNetworkConfig 走 utils.LoadSetting，后者恒返回地址（core/utils/settings.go:18-62）。因此 SettingsLan.save() 的展开合并（:260-268）**不会**退化成只含 LAN 字段——这条专门验证过，是"看似危险实则安全"的典型。
24. **SettingsNetwork 的整对象写不丢字段**：netConfig 在 loadData 时被完整 NetworkConfig 整体替换（:323），不是用本地默认对象拼的。
25. **ApplyUwpExemptionChanges 的差异提交正确**（SettingsUwp.vue:187-200）：用 initialEnabled 快照算 added/removed，空列表单独走 ClearAllUwpExemptions，uwpLoaded=false 时禁止提交——防"没加载出来 = 用户全取消"的误判。
26. **日志级别值域一致**：后端 clash.NormalizeLogLevel 产出 warning/silent（clash/config.go:396-411），前端 logLevelOptions 用的就是 warning（SettingsBehavior.vue:259）；Settings.vue:174 的 warn 转 warning 映射冗余但无害；appLogLevel 用 warn，后端 logger.NormalizeLevel 同时接受 warn 与 warning（logger/level.go:11-12）。两套 normalize 语义不冲突。

### 5.3 生命周期 / 并发 / 兜底

27. **App.vue 的 25 个事件监听全部在 onUnmounted 里 unsub**（App.vue:1002-1016），三个定时器（scrollTimer / logBatchTimer / appUpdateCheckFailsafeTimer）也都有清理。
28. **Proxies / Connections / Subscriptions / SettingsTun / SettingsUpdate / SettingsAbout 的 Events.On 都有对应 unsub**；Proxies 的 trackResizeObserver 有 disconnect（:812-823）。
29. **Toast 容量与定时清理正确**：上限 3 条 + 每条 setTimeout dismiss（store.ts:1240-1244），dismissToast 幂等（:1247-1250）。
30. **模态队列（F-A4-01 的修复）已生效**：单槽 + modalQueue + managed 按钮 + 5s reconcile 兜底 + 60s pending 看门狗。核对了 enqueueModal / settleModal / presentModal 的正常与异常路径，**不会再有 Promise 悬挂**。
31. **IP 检测是 latest-wins 且有 seq 校验**（store.ts:821-925）：seq 不匹配时丢弃旧结果，pendingIPRefresh 在 finally 里立即执行。
32. **updateStateFromBackend 的 revision 单调保护正确**（store.ts:370-380）：只有 revision 不小于当前才接受 desired* 覆盖，用户 intent 由 controlOverlay 优先（:181-183）。
33. **未注册事件不会丢**（推翻旧结论）：Wails v3.1.0 的 validateCustomEvent 对未注册名字只 warn 后 return nil（events.go:339-344），故 geo-update-<key>-* 与 notify-error 这类未注册事件**仍会到达前端**；"注册了但类型不符"才会被 cancel（events.go:356-367）。这条让 §1.3 的"白发事件"判断得以成立。
34. **component-update-tasks-sync 的空列表安全**：ClearFinished 在表空时 emit 空数组（component_update_state.go:76-84），前端 forEach 建空索引表，UI 正常清空。
35. **SettingsDiagnostics 有 inflight 互斥**（:164-183），双击刷新/导出不会并发。
36. **DeviationStrip 有 disposed 守卫**（:42/:65/:68/:71/:90），卸载后不回写状态。

### 5.4 其余判据面（查过但没发现）

37. **定时器泄漏**：全前端 setInterval 只有 2 处（store.ts:1193 看门狗、trafficWaveState.ts:140 波形采样）；前者是设计上的全局看门狗（幂等启动），后者有 refCount + stopWaveSampling。**未发现泄漏**。
38. **v-for key**：除 App.vue:154（C-13）与 DeviationStrip.vue:23（列表 ≤3 项，可忽略）外，其余 v-for 都用业务 id 作 key。
39. **as any 与类型逃逸**：集中在 3 处（Subscriptions.vue 的 fetchConfigs data、SettingsNetwork.saveNet、useTunToggle 的 GetAppState），都是"后端 map[string]any 无法静态类型"的合理妥协，未发现用非空断言掩盖 null 的写法。
40. **localStorage 读取**：模块级 4 处（store.ts:69-78）与工具函数都有 try/catch 或默认值；只有写入的 3 处缺失（C-11）。
41. **OR 与空值合并运算符误用**：只在 C-21 那类"t() 后接中文默认值"上误用；数值/布尔场景都用 ?? 或显式 !== undefined，未发现"0 或 false 被 OR 吞掉"。
42. **大小写比较**：日志级别走 normalizeLogLevel 归一（store.ts:317-323），连接过滤走 toLowerCase（Connections.vue:360-365），事件名比对区分大小写且后端全小写连字符——一致。

---

## 6. 已知问题（历史已记录，不计入本轮 23 条）

### 6.1 本轮发现中与历史重合的部分

| 本报告的观察 | 旧 ID | 旧结论/状态 | 本轮处理 |
| --- | --- | --- | --- |
| geo-update-<key>-error、geo-update-all-error 前端零监听（用户点更新没反应） | **F-A5-06 (P1)** | 历史上已确认"三类更新失败可见性为零" | 计入 §6，不重复计数；C-08 只列历史未点名的 12 个名字 |
| notify-error 前端零监听且未注册（tray 侧仍在发） | **F-A5-23 (P2)** | 已建议"删掉死事件或补消费" | 计入 §6 |
| driver-install-error / -cancelled 前端零监听 | **F-A5-06 / F-A5-23** | 同上 | 计入 §6 |
| 用中文字面量判定后端语义（isLongConnectionDeferral / isRouteSwitchingMessage） | **F-A4-14 (P1，fixed B3)** | 当时"后端补 code 后只改这两个函数体"的做法已落地，但后端**并未真的补 code**（OutboundIPResult 仍只有 message） | 计入 §6；C-20 只报"未被静默降级的那一半"：中文串进了 toast |
| NOTIFICATION_UI 的 silent 分级来源 | **F-A4-18 (P2，fixed B3)** | 当时扩宽了过滤规则，这张表是配套遗留 | 表内死键属新发现（C-09），不计入 §6 |
| "全部测速失败无提示" | **F-A4-10 (P1，fixed B2)** | 修法覆盖的是 RPC reject 分支 | **本报告按新问题记 C-05**（Go 侧该方法恒不 reject，真正的失败分支未修），请主控裁定记账方式 |
| 订阅批量更新失败被吞 | **F-A5-02 (P0，fixed B0)** | 已在 subscriptions.go:86-113 改为逐条收集并返回错误 | 复核确认已修（前端 subs-update-error 已接，Subscriptions.vue:624-628） |

### 6.2 顺带复核：历史问题在当前工作区的真实状态（只读核对，未修改）

| 旧 ID | 本轮核对结果 | 证据 |
| --- | --- | --- |
| F-A4-01 / 02（模态单槽） | **已修**：单槽 + 队列 + managed 回调 + reconcile 兜底（store.ts:484-677） | A |
| F-A4-03（TUN 两套实现） | **已修**：统一到 composables/useTunToggle.ts，SettingsTun 与 Overview 共用 | A |
| F-A4-06（自动保存失败静默） | **已修**：四个页面都有 catch + toast（SettingsLan.vue:271-277 等） | A |
| F-A4-08（启动链订阅顺序） | **已修**：所有 Events.On 已前移到重调用之前，且各自包 try/catch（App.vue:756-962） | A |
| F-A4-19（导出取消仍导出） | **已修**：改为 openModal + onConfirm（Subscriptions.vue:451-461） | A |
| F-A4-20（UWP 全不选） | **已修**：toggleAllUwp 直接赋 val（SettingsUwp.vue:143-146） | A |
| F-A4-21（偏差条把失败当无偏差） | **已修**：失败保留上次结果 + 只 reportError（DeviationStrip.vue:70-74） | A |
| F-A5-06 的 startup-task-* 部分 | **已修**：store.ts:1044-1054 现已监听两条事件 | A |
| F-A1-12（runtime-assets-status 零订阅） | **已修**：App.vue:843-845 现已订阅并写入 globalState.assetStatus | A |
| F-A5-23 的"netify-error 无消费者" | **仍存在**（tray_runtime_windows.go:503 仍在发） | A |
| F-A4-14 的中文语义耦合 | **仍存在**（两处正则未变，后端未补 code 字段） | A |

### 6.3 一条被本轮推翻的旧结论（供主控更新账本）

A5 审计（F-A5-23）曾担心"事件名未注册 → 事件被静默丢弃"。核对 Wails v3.1.0 源码后确认：**未注册名字只会在非 production 且带 strictevents 标签时打一条 warning（events.go:339-344），不会丢**；只有"已注册但类型不符"才会 Cancel + 上抛（events.go:356-367）。
因此"未注册"不是漏事件的原因，真正原因是"没人监听"。这解释了为什么 geo-update-<key>-* 能让前端 geo-update-<key>-success 收到（SettingsUpdate.vue:108 确实能用）。

---

## 7. 需要跨文件 / 后端确认的疑点（移交主控）

| ID | 疑点 | 需要谁确认 | 关联 |
| --- | --- | --- | --- |
| Q1 | **C-05 记账方式**：F-A4-10 的修法只覆盖了 RPC reject 分支，Go 侧真正失败走事件载荷。是"重开 F-A4-10"还是"新增 C-05"？ | 主控 + 账本维护者 | C-05 |
| Q2 | **proxy-test-finished 载荷改造**：把 string 换成结构体需要同时改 v3_events.go:79 的 RegisterEvent[string]、delay_manager.go:461/490 两个发射点、Proxies.vue 监听器。若只改两端不改注册，事件会被 wails cancel（类型校验）。是否纳入本轮修复批次？ | 主控 + 后端代理 | C-05/C-06 |
| Q3 | **AppBehavior / TunConfig / DNSConfig / NetworkConfig 是否补字段级 patch API**：这是 C-03（设置页整对象覆盖）与 C-14（自动保存并发）的共同根治方案，纯前端改只能缓解。是否需要后端排期？ | 主控 + 后端代理 | C-03/C-14 |
| Q4 | **StartClash 零调用 + 诊断页无"修复运行组件"按钮**：是设计意图（不暴露单步启动）还是 UI 漏做？RepairRuntimeAssetsAsync + GetRuntimeAssetRepairProgress 这对 API 明确为 UI 设计却无人调用。 | 主控 + 产品/后端 | C-07 |
| Q5 | **订阅排序是否要真做**：要落盘需后端加写接口（clash.ReplaceSubIndex 已有能力）。是先隐藏 UI 还是补接口？ | 主控 | C-01 |
| Q6 | **geo-update-active-sync / proxy-test-progress 是否接**：前者可驱动"哪些库在下载"，后者是批量测速进度条。属体验增强，需排优先级。 | 主控 | C-08 |
| Q7 | **getRecentErrors 的出口**：reportError 已收集 50 条前端错误但无任何出口。是否并入 ExportDiagnostics 的产物（需后端配合追加文件/字段）？ | 主控 + 后端代理 | C-22 |
| Q8 | **窗口关闭拦截**：C-02（未保存 YAML）与 C-23（handleClose 无兜底）都涉及"点 X 时先确认"。Wails 侧需要 OnBeforeClose / RegisterHook 之类机制，属跨层改动。 | 主控 + Wails 层 | C-02/C-23 |
| Q9 | **一处运行时未验证项**：C-06 依赖"Wails IPC 跨事件名的到达顺序"，C-11 依赖"localStorage 抛异常"，C-19 依赖"mode 出现非三值"。三者都标 B/C，若主控要做实证，建议各写一个最小复现脚本（无需启动完整应用也可用 dev 模式手动触发）。 | 主控（若需升 A） | C-06/C-11/C-19 |

---

## 附：可复现的对账脚本要点（给下一轮留档）

本轮所有结构性结论都由一次性脚本产出，未落库（避免污染仓库）。核心逻辑：

1. **签名解析**：对三个 binding 文件逐行匹配 `export function NAME(params): $CancellablePromise<RET> {`（多行签名先拼接再匹配），按顶层逗号切参数，记 name/svc/params/ret。
2. **调用点解析**：对 frontend/src 下 .ts/.vue 扫 `App\.([A-Z]\w*)\s*\(`，从函数名后的左括号开始做**平衡括号扫描**（处理字符串与模板字面量），再按顶层逗号数实参；跳过以 // 或 * 开头的注释行。
3. **交集/差集**：三个 namespace 两两求交 + 三元求交；bindings 方法全集减调用名集合得死绑定；Events.On 字面量集合与 Go Emit 字面量集合双向求差。
4. **ByID 冲突**：正则抽 `$Call.ByID\((\d+)` 与函数名配对，按 ID 分组找重复。
5. **通知码**：抽 `notify("level","CODE"` 与 `setLastErrorCode("CODE"`，与 store.ts 的 NOTIFICATION_UI 键集求差。
6. **死 import/export**：把 import 语句从源码中剔除后全文正则计数，0 命中即死。

建议主控把 1-6 合成一个 `scripts/check-contract.mjs` 接入 build（与已有 scripts/check-i18n.mjs 并列），把本轮的一次性检查变成 CI 门禁——C-07 / C-08 / C-09 / C-12 / C-22 这几类问题都是"会随时间重新长出来"的。

---

*报告结束。本文件是只读审计产物，未修改任何源码、样式、配置或 locales。*
