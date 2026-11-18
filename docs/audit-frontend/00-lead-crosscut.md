# 前端审计 · 主控跨切面报告（2026-11-18）

> 审计性质：只读。未修改任何源码。
> 分工：主控 lead 负责跨切面取证（静态检查 / 事件契约 / 全仓冗余扫描）+ 汇总去重；6 个分片报告见同目录 01~06。
> 证据等级：A = 代码或脚本输出可直接确证；B = 代码 + 运行时推断；C = 假设待实测。
>
> **修订说明（最终版）**：本文 §3.1「模态队列停摆」已被 01-state-core.md（S-07）与主控复核**推翻** —— 不变量 activeModal 非空 等价于 modal.show 为真 成立（show 只由 presentModal 置真），且旧式直写在全仓已 0 处，队列分支不可达。§5 中「TUN 只有单一写入路径」亦需修正：正常路径确为 useTunToggle 单实现，但 store.ts:1015-1031 的通知兜底里仍有一条裸 ToggleTunMode（绕过 pending 闸门）。最终结论以 00-SUMMARY.md 为准。

---

## 0 静态基线（先跑，避免“猜”）

| 检查项 | 命令 | 结果 |
|---|---|---|
| TypeScript 全量类型检查 | vue-tsc --noEmit | **PASS，0 错** |
| 设置清单锚点一致性 | scripts/validate-settings-manifest.mjs | **PASS**，103 个设置项锚点全部就位 |
| i18n 语言对齐 + 字面量 key 存在性 | scripts/check-i18n.mjs | **PASS**，8 语言 / 642 个代码 key / 18 处动态 key 无法静态校验 |
| i18n 死键 | 同上（warn） | **188 个死键**（base 830 键中 188 个无人引用）→ 见 §4.3 |

结论：**没有编译期 / 类型层错误**。所有问题都在运行期语义、并发、冗余与契约层面 —— 这也决定了本轮审计方法必须是“读逻辑 + 对账”，而不是“看编译错误”。

---

## 1 前后端契约：事件通道总账（A 级，脚本对账）

对账口径：**v3_events.go** 里 application.RegisterEvent[...]("name") 为后端权威事件集合（61 个）；**frontend/src** 里 Events.On("name") 为前端订阅集合（44 个）。

### 1.1 注册了但前端从不监听（18 个）

| 事件 | 用户可感后果 | 等级 |
|---|---|---|
| core-update-check-success / core-update-check-cancelled | 前端只在 -start 置位、只在 -error / core-update-none / core-update-available 复位 checkingCoreUpdate。检查被**取消**时不发 none/available → 按钮与转圈**永久停在“检查中”**。 | **P1** |
| geo-update-all-start / -success / -error | 「更新全部 Geo 数据库」**完全没有进度与失败反馈**（UpdateAllGeoDatabasesAsync 立即返回，结果只走事件）。 | **P1** |
| driver-install-start / -error / -cancelled | 前端只监听 driver-install-success；驱动安装**失败或取消时无任何监听者**，只能靠 component-update-tasks-sync 快照间接看到。 | P1 |
| subs-update-cancelled | 订阅批量更新被取消后，前端“更新中”状态无对应复位入口。 | P2 |
| proxy-test-progress | 批量测速进度事件无人消费 → 进度条数据源缺失（需确认是否改用 start/finished 推算）。 | P2 |
| config-file-saved / geo-update-active-sync | 纯死事件：无人监听，且前端已有等价通道（config-changed / 轮询）。 | P2 |
| app-update-check-job-*（3）/ app-update-download-job-*（3） | v3_events.go 注释已声明“前端刻意不消费”→ **不算缺陷**，仅登记。 | — |

### 1.2 前端监听但未注册（1 个）

| 事件 | 说明 |
|---|---|
| geo-update-(key)-success（模板字符串） | v3_events.go 已注释：动态事件名无法静态注册，属**已知设计**，非缺陷。 |

### 1.3 重复订阅（同一事件多个 handler，属冗余）

app-update-downloaded、app-update-error、proxy-delay-update、config-changed、app-update-busy、driver-install-success（3 处）、core-update-none、geo-update-(key)-success，以及 **core-update-success（store.ts 内同一文件注册两次：L1088 复位状态 / L1132 调度 IP 刷新）**。
后者是明确的可合并冗余：同一文件同一事件订阅两次。

---

## 2 前后端契约：服务方法命名空间（A 级，脚本对账）

**frontend/src/api.ts** 用 { ...AppService, ...CoreService, ...UpdateService } 展开合并 3 个 binding 命名空间。

- 实测导出方法数：AppService **31** + CoreService **62** + UpdateService **15** = **108**（与 api.ts 注释一致）。
- **两两交集为空**（A∩C、A∩U、C∩U 均为空）→ **不存在同名方法被静默覆盖的风险**。
- vue-tsc 通过 → 前端所有 App.Xxx(...) 调用点在**方法名层面**全部存在、参数个数与类型与签名匹配。

结论：本轮前后端对接的问题**不在“名字 / 类型”层，而在“语义 / 并发 / 事件”层**（见 §1 与 06 分报告）。

---

## 3 操作冲突与状态机（主控亲审 store.ts）

### 3.1 【已撤回】模态队列存在“停摆”路径

**store.ts:559-566 / 590-595** 的排空判据同时要求 activeModal === null **且** globalState.modal.show === false：

    function showNextModal() {
      if (activeModal || globalState.modal.show) return;   // 双重闸门
      ...
    }
    function drainModalQueue() {
      if (activeModal) return;
      if (globalState.modal.show) return;                  // 同上
      ...
    }

- 只要出现 show === true 而 activeModal === null（settleModal 已把 activeModal 置空，但 onConfirm 回调里同步再置 show=true；或外部置位），队列就**停止出队**。
- 5s 巡检 reconcileModalDisplacement()（L601）在 !activeModal 时只调 drainModalQueue() → 被同一闸门挡住 → **队列里的 showConfirm Promise 永不 settle**。
- 后果：调用方 finally 不执行 → tunPending / systemProxyPending 卡 true；watchdog 60s 后强复位（有兜底，但用户已经“点了没反应”60 秒）。
- 归属：与 01-state-core 分报告交叉验证（需给可复现触发路径）。

### 3.2 “旧式直写兼容”已成死防御（冗余约 40 行，A 级）

全仓 grep globalState.modal：**除 store.ts 管理器自身与 App.vue 模板只读渲染外，没有任何组件再直写 globalState.modal**。但 store.ts 仍保留整套兼容分支：

- enqueueModal 的 legacyOccupied 判据（L638）
- reconcileModalDisplacement() 全文（L597-621）
- confirmModal() / cancelModal() 的 legacy 分支（L660-662、L673-675）
- showNextModal / drainModalQueue 里对 modal.show 的判空（正是 §3.1 停摆风险的来源）

建议：要么删掉 legacy 分支（单一真相），要么把 show 改为 computed(() => !!activeModal)，从根上消除“两个真相”。

### 3.3 localStorage 写入保护不均衡（P2，A 级）

updateStateFromBackend 内 4 处 localStorage.setItem **无 try/catch**：L346（goclashz_hideLogs）、L402 / L405（goclashz_activeConfigId）、L870（goclashz_outboundIP）；而同文件 applyThemeToApp（L116-120）、setProxyCardMode（L208-212）都有保护。
后果：隐私模式或配额满时，异常会**中断整个状态同步** —— 异常点之后的字段（activeConfigName、version、language 等）本次不更新，且异常冒泡进 Events.On 回调。

### 3.4 initStore 的幂等标记早于副作用（P2，A 级）

    export async function initStore() {
      if (storeInited) return;
      storeInited = true;      // 先置位
      startPendingWatchdog();
      try { await App.GetAppState() } catch { ... }
      Events.On(...)           // 之后才注册 13 处订阅
    }

若中途抛错（Events.On 名冲突 / payload 类型不符在 v3 会拒发并抛错），storeInited 已为 true → **永不重试、后续订阅永久缺失**，症状是“界面不跟随后端状态”。

### 3.5 出站 IP 检测：启动期双路并发（P2，A 级）

initStore 末尾直接 refreshOutboundIP()（无 expectedRoute），而 updateStateFromBackend 的状态变化又会 scheduleRouteAwareIPRefresh。两者都会打 GetOutboundIPForRoute（后端真实探测网络），靠 ipDetectSeq 丢弃旧结果 —— 功能正确，但**启动时多一次昂贵探测**，且首次结果可能被丢弃导致 IP 显示延迟。

---

## 4 冗余与死代码（全仓脚本普查，A 级）

### 4.1 整文件死代码

| 位置 | 证据 |
|---|---|
| **frontend/src/utils/normalize.ts**（18 行） | 唯一导出 normalizeStartupTaskInfo 全仓**零引用**，无任何文件 import 该模块 → **整文件可删**。 |
| **frontend/src/settings/manifest.ts:1013** findSettingById | 零引用。 |
| **frontend/src/utils/format.ts:23** formatDurationCn | 零引用。 |
| **frontend/src/utils/report.ts:38** getRecentErrors | 零引用（reportError 在用，但读取侧没有消费者 → 错误环形缓冲写了没人读）。 |
| **frontend/src/utils/glass.ts:13 / 15 / 44** GLASS_BLUR_OVERLAY_DEFAULT / GLASS_BLUR_CONTROL_DEFAULT / normalizeGlassBlurStrength | 零引用（玻璃参数改为 behavior 驱动后的遗留）。 |

### 4.2 冗余导出（32 个符号只在本文件内部使用却仍 export）

典型：store.ts 的 normalizeOutboundIP / updateProxyDelay / refreshOutboundIP / resolveNotificationUi / startPendingWatchdog；utils/theme.ts 的 BASE_PALETTE / THEME_CACHE_KEY / relativeLuminance / mixHex / shadeHex / systemPrefersDark / AppliedTheme；utils/errors.ts 的 ErrorKind / ErrorLevel / ErrorClass / ERROR_LOGGED_SUFFIX / matchesKnownErrorRule；utils/glass.ts 的 GLASS_BLUR_DEFAULT / GlassStrengthGroups / glassTintChroma / GLASS_TINT_ALPHA / glassTintColor。
影响：对外面积虚高，等于给未来改动留下“外部有人用”的错误信号。

### 4.3 i18n 死键 188 个（脚本已报）

check-i18n.mjs 输出 188 个死键（如 common.close / common.delete / common.edit / connections.* / deviation.field.* / coreSeedPrompt ...）。这是 **跨 8 个语言包 × 188 键 ≈ 1504 行**的纯冗余，同时抬高每次改文案 8 份对齐的成本。

---

## 5 复核对账：已排除的假阳性（防止误报）

| 曾经的怀疑 | 核查结论 | 依据 |
|---|---|---|
| api.ts 展开合并会静默覆盖同名方法 | **不成立** | 三 namespace 方法名两两交集为空 |
| App.vue 事件订阅泄漏 | **不成立** | onUnmounted（L976-1017）逐个 unsub?.()；addEventListener 均有 removeEventListener；scrollTimer / logBatchTimer / appUpdateCheckFailsafeTimer 全部清理 |
| TUN 有两条互相打架的写入路径 | **基本不成立，但有一条次级旁路**（store.ts:1015-1031 通知兜底裸调 ToggleTunMode，绕过 pending 闸门，记 P2） | composables/useTunToggle.ts 是唯一实现（Overview / SettingsTun 共用），tunPending 单飞闸门 + settleTunIntent 在 finally；TunConfig 模型**不含** tunEnabled，SaveTunConfig 不会与 ToggleTunMode 争写同一字段 |
| 前端调用了不存在的后端方法 | **不成立** | vue-tsc 0 错 + binding 方法名全匹配 |

---

## 6 待交叉验证（等分片报告回收后定稿）

1. §3.1 模态队列停摆 —— 需要 01-state-core 给可复现触发路径，确认是否真能发生。
2. core-update-check 取消路径卡死 —— 需 04-settings / 06-contract 确认按钮实际绑定的是 checkingCoreUpdate 还是 componentUpdate.tasks 快照。
3. appLogLevel 取值 warn 与 warning 不一致 —— 需 04-settings 确认 ModernSelect 对未匹配 v-model 值的渲染行为（是否空白）。
4. 未提交改动（set-theme-glassTint 改为 v-if 收起）与设置搜索锚点的交互 —— 默认 graphite 为中性色时该锚点默认不存在，applyAnchor 重试 6 次后回顶并提示 diag.searchHiddenItem（有兜底，属体验缺口，P2）。
