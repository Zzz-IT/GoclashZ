# 前端状态层审计报告（01-state-core）

> 审计对象：GoclashZ v3 前端 **状态层核心**
> 审计代理：state-core（6 代理并行审计中的 1 个，主控 = lead）
> 审计日期：2026-11-18 20:0x（Asia/Singapore）
> 判据：① 代码冗余 ② 逻辑错误 ③ 操作冲突（重点） ④ bug ⑤ 前后端对接
> 声明：本次为只读审计，**未修改任何源码/样式/配置/locales**。工作区未提交修改（locales/*、settings/manifest.ts、SettingsTheme.vue）以当前内容为准。

---

## 1 审计范围

### 1.1 分配给我的文件（逐行读完）

| 文件 | 行数 | 覆盖 |
|---|---|---|
| frontend/src/store.ts | 1250 | **100%**（按 1-300 / 300-500 / 500-700 / 699-900 / 898-1098 / 1097-1250 六段读完） |
| frontend/src/trafficWaveState.ts | 242 | **100%** |
| frontend/src/settings/searchState.ts | 8 | **100%** |
| frontend/src/api.ts | 17 | **100%** |
| frontend/src/main.ts | 14 | **100%** |

### 1.2 为验证「对接」额外读过的文件（只读取证，不属本报告范围）

- frontend/bindings/goclashz/{appservice,coreservice,updateservice}.ts（脚本化提取导出面）
- frontend/bindings/goclashz/core/appcore/models.ts（OutboundIPResult / UpdateTaskState）
- frontend/src/utils/{errors.ts,theme.ts,report.ts,normalize.ts}
- frontend/src/composables/useTunToggle.ts；components/{Toast,Settings,SettingsTheme,SettingsBehavior,SettingsUpdate,SettingsTun,TrafficCard,Proxies}.vue；App.vue（按需定位片段）
- 后端：core/appcore/{controller.go,control_coordinator.go,supervisor.go,outbound_ip.go,updater_orchestrator.go,component_update_state.go,component_update_service.go}、core/tasks/manager.go、v3_events.go、services_v3_generated.go、app.go（按需定位片段）

### 1.3 未覆盖 / 不覆盖

- 不审 App.vue 与各 .vue 组件的 UI 逻辑（归队友）；仅在「跨文件契约」必须时引用其行号。
- 未运行 pnpm build / vue-tsc / vite / 应用本体（主控统一跑），因此**所有运行时行为均为代码推断，无实机验证**。
- 仓库内 0 个 *.test.ts / *.spec.ts，无单测可参考。
- 未深挖 CodeMirror / 滚动 / 玻璃特效等纯展示层。

### 1.4 证据等级

- **A** = 代码可直接确证（读到的行、脚本化 grep 结果）。
- **B** = 代码 + 运行时推断（机制可确证，用户可见结果需满足时序）。
- **C** = 假设待实测（已标注需实机确认）。

### 1.5 历史基线

上一轮审计（2026-11-16）：docs/audit/A4-frontend.md（主题＝提示面与错误处理）、docs/audit/ISSUE_LEDGER.md（118 条全部标记 fixed）。本轮判据不同，**重复项一律进 §6，不计入新问题数**。

---

## 2 问题总表

新问题 **11 条**：P0 = 0，P1 = 3，P2 = 8。

| ID | 严重度 | 类别 | 位置(file:line) | 一句话问题 | 证据等级 |
|---|---|---|---|---|---|
| S-01 | **P1** | 操作冲突/bug | store.ts:1185-1217（+ useTunToggle.ts:129-134、Overview.vue:256-266） | pending 看门狗 60s 强清不区分「卡死」与「用户正看着确认框」，会误清合法等待并允许重入；旧流程 finally 还会清掉新流程的 intent overlay | B |
| S-02 | **P1** | 操作冲突 | store.ts:134-145（+ Sidebar.vue:75、Settings.vue:170-186/204） | setThemeMode 用「整对象 GET→改→SAVE」回写 behavior，与设置页缓存的 behavior 快照互相覆盖 → 侧栏切暗色后在设置里保存任意项，themeMode 被打回旧值 | A（机制）+B（可见结果） |
| S-03 | **P1** | 逻辑/冲突 | store.ts:180-203（+ Overview.vue:256-266、useTunToggle.ts:129-134） | desiredSystemProxy/desiredTun 被前端 UI 直写且无 revision 保护，失败路径把后端已回滚的 desired 再覆盖一次 → 卡片显示「已关闭」而系统代理实际仍开着 | C（依赖事件/RPC 到达顺序） |
| S-04 | P2 | 前后端对接 | store.ts:936-973 | NOTIFICATION_UI 15 项中 7 项后端永不产出；后端真实产出的 13 个 code 未入表（其中 4 个语义错配）→「A 类静默」整档不可达，ROUTE_SWITCHING/TUN_NOT_READY 落成 toast | A |
| S-05 | P2 | bug/健壮性 | store.ts:69-76、343-347、400-406 | 模块级 5 处 localStorage.getItem 与 updateStateFromBackend 内 3 处 setItem 无 try/catch（同文件 117-120、209-212 与 utils/theme.ts 都包了）→ 抛错会中断整次状态同步，模块级抛错会白屏 | A+C |
| S-06 | P2 | 冗余/泄漏 | store.ts:975-994、991-1174；main.ts:12；App.vue:757 | 21 个 Events.On 的 unsub 全部丢弃、无 teardown；initStore 被两处调用（第二处是死调用）；storeInited 在首个 await 前就置 true | A |
| S-07 | P2 | 冗余/死代码 | store.ts:601-621、638-642、653-677、439 | 旧式直写 modal 的兼容分支已不可达（全仓 0 处直写）；settleModal 不清 onConfirm/onCancel 使 legacy 分支拿到 stale handler；DelayRetentionTime 的联合类型被末尾 string 击穿 | A |
| S-08 | P2 | 逻辑/bug | trafficWaveState.ts:9-12、111-151、67-72 | 采样周期 900ms 与后端推流 900ms 同频异相 → 波形重复点/跳采；stopWaveSampling 的 refCount 只在 timer 非空时归零（不配对会漂负）；scaleFallAlpha 不可达 | B |
| S-09 | P2 | 逻辑/冗余 | store.ts:317-323（+ App.vue:396-405） | 日志级别规整两份实现且不一致：store 保留 'silent'，App.vue 的 normalizeLogType 把 'silent' 归成 'info' → 内核级别设 silent 时日志面板仍显示 info 及以上 | A |
| S-10 | P2 | 前后端对接 | api.ts:13-17 | 三门面 spread 合并当前**无重名**（108 导出 / 108 distinct，已脚本验证），但合并没有任何编译期或 CI 不变量保护，未来同名方法会静默覆盖 | A |
| S-11 | P2 | 操作冲突 | store.ts:767-816 | 手动 refreshOutboundIPRouteAware 不取消 route settle 定时器 → 一次点击最多打两次后端；pending 重放再叠加一次（有界，不堆积） | A |

---

## 3 逐条详情

### S-01（P1）pending 看门狗会误清合法等待，导致可重入与 overlay 被旧流程清掉

**位置**：frontend/src/store.ts:1185-1217；受害调用方 frontend/src/composables/useTunToggle.ts:64-135、frontend/src/components/Overview.vue:232-267

**现状**（store.ts:1185-1215）：

```ts
const PENDING_WATCHDOG_MS = 60_000;          // 1185
const PENDING_WATCHDOG_TICK_MS = 5_000;      // 1186
...
window.setInterval(() => {
  const now = Date.now();
  reconcileModalDisplacement();               // 1196
  (['tunPending', 'systemProxyPending'] as const).forEach((key) => {
    if (!globalState[key]) { delete pendingSince[key]; return; }
    const since = pendingSince[key];
    if (since === undefined) { pendingSince[key] = now; return; }
    if (now - since < PENDING_WATCHDOG_MS) return;
    globalState[key] = false;                 // 1211
    delete pendingSince[key];
    if (key === 'tunPending') settleTunIntent();      // 1213
    else settleSystemProxyIntent();
  });
}, PENDING_WATCHDOG_TICK_MS);
```

调用方把 pending 覆盖到了「整个模态交互 + 安装 + 重试」期间（useTunToggle.ts:129-134 的 finally 才复位）：

```ts
    const confirmed = await showConfirm(t('overview.tunHelperConfirm'), t('overview.tunHelperTitle')); // 82
    if (!confirmed) { ... return false; }
    try { await App.InstallHelperService(); } ...   // 88
} finally {
    globalState.tunPending = false;   // 130
    if (next === false) globalState.desiredTun = false;
    settleTunIntent();                // 133
}
```

**为什么是问题**：

1. 后端单次控制命令预算是 **25s**（core/appcore/control_coordinator.go:32 controlCommandTimeout = 25 * time.Second），所以「pending 超过 60s」在实践中只可能是**卡在模态框等用户点击**——而这恰恰是合法的、不该被清理的状态。看门狗用同一把 60s 尺子量「等用户」和「等后端」，判据本身就错。
2. 被误清后 tunPending=false，卡片恢复可点，用户可以再点一次开关 → **第二个 toggle 流程与第一个并发**（第一个还停在 await showConfirm）。
3. 第一个流程最终走到 finally 时会无条件执行 settleTunIntent()（它不知道自己已经不是「当前操作」），把**第二个流程正在显示的 intent overlay 清掉** → 卡片在切换过程中跳回后端 desired 值，随后第二个流程返回再跳一次。systemProxy 侧完全同型（Overview.vue:265 + store.ts:180-203 的单一 overlay 槽）。
4. overlay 本身没有 owner/token 概念（store.ts:175-178 只有两个全局槽），所以「谁清的」无法校验。

**复现路径（B 级，机制确定 / 用户可见结果需实机）**：

1. TUN 关闭状态下，在没有 helper 的机器上点「开启 TUN」→ 后端返回 helper_install_required → 弹「需要初始化后台服务」确认框（此时 tunPending=true、overlay=on）。
2. 不点确认框，离开 61s。
3. 看门狗在 60s 后强制 tunPending=false + settleTunIntent() → 卡片从「开启中」瞬间回落到「关闭」，并且又变成可点击。
4. 再点两次开关（此时 pending=false，第二次点击进入自己的流程），然后回头点掉步骤 1 的确认框「取消」→ 旧流程 finally 的 settleTunIntent() 会清掉新流程的 overlay。

**修复建议**（改 store.ts，调用方零改动即可缓解）：

```ts
// 1) 加 owner token：set 返回 token，settle 只清自己的
let intentToken = { systemProxy: 0, tun: 0 };
export function setTunIntent(value: boolean) {
  const token = ++intentToken.tun;
  controlOverlay.tun = value;
  syncVisibleControls();
  return token;
}
export function settleTunIntent(token?: number) {
  if (token !== undefined && token !== intentToken.tun) return; // 不是我的槽，别动
  controlOverlay.tun = null;
  syncVisibleControls();
}

// 2) 看门狗：有未结算的受管模态时不计时（那是在等用户，不是卡死）
if (globalState.modal.show) { pendingSince[key] = now; return; }
```

> 顺带：建议把「等用户」与「等后端」拆成两个字段（如 tunAwaitingUser），看门狗只对后者计时。

---

### S-02（P1）setThemeMode 整对象读改写 ↔ 设置页 behavior 快照，互相覆盖导致主题回退

**位置**：frontend/src/store.ts:134-145；交叉证据 frontend/src/components/Sidebar.vue:75、components/Settings.vue:170-186、202-204、components/SettingsTheme.vue:344-350、617-626

**现状**（store.ts:134-145）：

```ts
export async function setThemeMode(mode: ThemeMode): Promise<void> {
  applyThemeToApp({ ...themeState.config, mode });
  try {
    const current: any = await App.GetAppBehavior();   // 137 整对象读
    if (!current) return;
    current.themeMode = themeState.config.mode;
    await App.SaveAppBehavior(current);                // 140 整对象写
  } catch (err) { reportError(err, 'theme.saveMode'); }
}
```

设置页自己持有一份 behavior 快照，保存时**整对象**写回（Settings.vue:172 / 204）：

```ts
const value = await App.GetAppBehavior();   // 172
...
behavior.value = value;                     // 183
...
await App.SaveAppBehavior(behavior.value);  // 204
```

而主题页**故意不同步**这份快照（SettingsTheme.vue:600-603 注释：「主题配置不同步：真源是 themeState … 否则过期的 behavior 快照会把用户刚在侧栏做的切换顶回去」），只单向同步 props.behavior → 玻璃。

**为什么是问题**：

- AppBehavior 是**单一 JSON 对象**，两个入口都用「读整份 → 改一个字段 → 写整份」，这是典型 lost update。
- 触发条件非常日常：用户**在设置页**（任意子页，如 TUN/行为/DNS）点一下**侧栏的明暗按钮**（侧栏常驻，永远可点）→ 后端 themeMode 变 dark；此时 Settings.vue 的 behavior.value.themeMode 仍是旧值 light；用户接着勾一个选项触发 saveBehavior() → 整对象写回 → **后端 themeMode 被打回 light**。
- 后果链（都能在代码里对上）：
  1. SaveAppBehavior 末尾会 SyncState()（app.go 注释明说），前端 updateStateFromBackend **刻意不消费 AppState.Theme**（store.ts:337-341），所以界面此刻仍显示暗色，看不出问题；
  2. 用户下次进入设置页 → Settings.vue:247 void loadBehavior() → syncThemeFromBehavior(behavior.themeMode='light')（store.ts:152-158）→ applyThemeToApp → **界面当场翻回浅色**；
  3. 重启后同样回退（后端 theme_setting.txt 由 behavior 派生）。
- 这与 store.ts:127-133 注释里描述的「点一次软件设置，明暗又翻一次」是同一个症状，只是那条路径已被修，这条还在。

**复现路径**：打开设置（任意子页）→ 点侧栏明暗（界面变暗）→ 在该子页改任意一项（如勾选「隐藏日志」触发 save）→ 关闭再打开设置 → 主题回浅色。

**修复建议**（三选一，建议 2）：

```ts
// 方案 1（最小改动，前端兜）：saveBehavior 前重读一次，只覆盖本页真正改过的字段
const latest = await App.GetAppBehavior();
Object.assign(latest, diffFields(behavior.value, pristine.value));
await App.SaveAppBehavior(latest);

// 方案 2（推荐，后端根治）：新增只写主题的接口，避免整对象竞争
// Go: func (a *App) SaveThemeMode(mode string) error
await App.SaveThemeMode(themeState.config.mode);   // store.ts:137-140 整段替换

// 方案 3：SaveAppBehavior 改 PATCH 语义（只接受显式传入的字段），改动面最大
```

---

### S-03（P1）desired* 被前端直写且无 revision 保护，失败回滚被覆盖 → 显示与真实状态不一致

**位置**：frontend/src/store.ts:172-203（overlay 契约）、store.ts:366-380（revision 门控）；写点 frontend/src/components/Overview.vue:256-266、frontend/src/composables/useTunToggle.ts:129-134

**现状**（store.ts:172-183 + 195-203）：

```ts
// The backend owns persisted desired state. This overlay exists only while a
// local command is in flight ... (172-174)
const controlOverlay = { systemProxy: null as boolean | null, tun: null as boolean | null };
function syncVisibleControls() {
  globalState.systemProxy = controlOverlay.systemProxy ?? globalState.desiredSystemProxy;  // 181
  globalState.tun = controlOverlay.tun ?? globalState.desiredTun;
}
export function settleSystemProxyIntent() {
  controlOverlay.systemProxy = null;
  syncVisibleControls();   // 197 → 回落读 desiredSystemProxy
}
```

失败路径直接改后端拥有的字段（Overview.vue:256-265）：

```ts
} finally {
  globalState.systemProxyPending = false;
  if (target === false) {                       // 258 关闭失败也照样执行
    if (globalState.portProxyMode) globalState.desiredPortProxy = false;   // 260
    else globalState.desiredSystemProxy = false; // 262 ← 直接写后端真源
  }
  settleSystemProxyIntent();                    // 265
}
```

**为什么是问题**：

- 后端在 B/C 类失败时会**回滚 desired 并立即 SyncState**（core/appcore/control_coordinator.go:187-194：SetAndSaveIf(desired, previous) → revision.Add(1) → SyncState()）。这次 sync 的 revision **高于**前端当前 controlRevision，会被正常消费（store.ts:370-380），于是 desiredSystemProxy 又被事件写成 true（真实意图）。
- 但前端 finally 在**之后**执行，把 desiredSystemProxy 再改回 false，且**不校验 revision**（只有事件路径有 revision 门控，UI 直写没有），于是后端真源被前端「就地改掉」，而 settleSystemProxyIntent() 读的就是这个被改过的值 → 卡片显示「关闭」。
- 真实状态：系统代理仍开着（关闭失败），后端 desired=true。**UI 与真实状态相反**，直到下一次 app-state-sync（可能几十秒后，或永不到来——supervisor 的 watchdog reconcile 只在状态变化时 emit）才自愈。
- 门控的位置本身也是缺陷：revision 只保护「事件覆盖事件」，没保护「UI 覆盖事件」。

**证据等级说明（C）**：机制是 A 级（两处写同一字段、其中一处无保护）；用户可见结果取决于「SyncState 事件」与「RPC 拒绝」的到达顺序——Wails 在同一桥接通道按发出顺序投递，Go 侧 SyncState() 在 return err 之前，因此**极可能**是事件先到、finally 后到（即坏顺序）。需实机确认（关闭系统代理的失败场景可造：把 Windows 代理设置锁住 / 拦截注册表写入）。

**修复建议**：

```ts
} finally {
  globalState.systemProxyPending = false;
  settleSystemProxyIntent();                       // 先清 overlay
  // 失败时不要写 desired*，用后端快照校准（一次轻量 RPC，失败也无所谓）
  if (!ok) {
    const latest = await App.GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
  }
}
```

> 更彻底：把 desiredSystemProxy/desiredTun/desiredPortProxy 移出 reactive、改成只读 getter，编译期禁止 UI 直写（与历史 F-A4-02 对 modal 的做法一致）。

---

### S-04（P2）NOTIFICATION_UI 表与后端实际产出的 code 集合脱节

**位置**：frontend/src/store.ts:936-955（表）、961-973（resolver）

**现状**（表，节选）：

```ts
const NOTIFICATION_UI: Record<string, NotificationUi> = {
  // A：静默
  ASSET_WINTUN_BUSY: 'silent',
  ASSET_CORE_PROBE_TIMEOUT: 'silent',
  HELPER_WAKE_TIMEOUT: 'silent',
  TUN_ADAPTER_PENDING: 'silent',
  // B：toast
  ASSET_SEED_UNAVAILABLE: 'toast',
  DEFERRED_RESTART_QUEUED: 'toast',   // + RESUMED / ABANDONED
  SYSTEM_PROXY_STATE_UNKNOWN: 'toast',
  STARTUP_TASK_CHECK_WARNING: 'toast',
  STARTUP_TASK_UNHEALTHY: 'toast',
  // C：模态
  STARTUP_RESTORE_HELPER_REQUIRED: 'modal',   // + NO_CONFIG / WINTUN_MISSING / FAILED
};
```

**为什么是问题**（两个方向的偏差，均已脚本化比对）：

1. **表里有、后端永不产出（7 项）**：ASSET_WINTUN_BUSY、ASSET_CORE_PROBE_TIMEOUT、HELPER_WAKE_TIMEOUT、TUN_ADAPTER_PENDING、ASSET_SEED_UNAVAILABLE、STARTUP_TASK_CHECK_WARNING、STARTUP_TASK_UNHEALTHY。
   全仓 Go 源码中这些字符串 0 命中；后两者实际是**事件名**（startup-task-check-warning / startup-task-unhealthy，controller.go:503 等），store.ts:1044/1050 已用 Events.On 单独处理，放在 code 表里属于误置。
   后果：'silent' 这一整档**在生产中不可达**——设计意图（A 类不打扰）从未生效。
2. **后端有、表里没有（13 项）**，全部落到 level 回落分支。后端实际产出而表里缺失的 code 全集（脚本 grep 全部 c.notify / setLastErrorCode 调用）：ROUTE_SWITCHING、TUN_NOT_READY、TUN_ADAPTER_NOT_READY、MODE_SAVE_FAILED、RUNTIME_OPERATION_FAILED、SYSTEM_PROXY_SET_FAILED、SYSTEM_PROXY_RECONCILE_FAILED、CORE_START_FAILED、ASSET_WINTUN_MISSING、TUN_RESTORE_FAILED、CONFIG_ROLLBACK_INCOMPLETE、STARTUP_TASK_CREATE_FAILED、STARTUP_TASK_DELETE_FAILED。即表只覆盖了 8 个真实 code（STARTUP_RESTORE_* 4 个 + DEFERRED_* 3 个 + SYSTEM_PROXY_STATE_UNKNOWN），其余 13 个都在靠 level 猜。其中 4 个有明确的语义错配（下面详述）：
   - ROUTE_SWITCHING（warn，outbound_ip.go:233，节流 5s）→ **toast**，文案是「路由切换中: expected=direct current=proxy」这种半机翻串，路由切换期间每 5s 可能再来一条。
   - TUN_NOT_READY（warn，controller.go:1416）→ toast。表里最近的 TUN_ADAPTER_PENDING 名字对不上（真实代码是 TUN_ADAPTER_NOT_READY，且只在该错误**不可自愈**时才发，见 controller.go:904-914），所以「可自愈的适配器等待」既没 code 也没静默。
   - MODE_SAVE_FAILED（error，controller.go:2234）→ modal。持久化失败弹阻塞框，是否合规取决于设计意图。
   - RUNTIME_OPERATION_FAILED（error，controller.go:1603 经 setLastError）→ modal，是所有 setLastError 的兜底 code。

**修复建议**：

```ts
const NOTIFICATION_UI: Record<string, NotificationUi> = {
  ROUTE_SWITCHING: 'silent',            // 路由切换是常态，IP 卡自己会显示「切换中」
  TUN_NOT_READY: 'silent',              // 有后台静默阶梯，用户不需要被打断
  MODE_SAVE_FAILED: 'toast',
  // 删掉 7 个无生产者的条目
};
```

并加一条 CI 校验：把 Go 侧 code 集合 dump 成 json，与前端表做差集，任何一侧新增/删除都 fail。

---

### S-05（P2）localStorage 访问无保护，异常会中断整次状态同步（模块级会白屏）

**位置**：frontend/src/store.ts:69-76（模块级读）、343-347 / 400-406（同步中写）

```ts
// 模块顶层，任一 getItem 抛错 = 整个 store.ts 求值失败 = App 白屏
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';        // 69
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';              // 70
const cachedActiveConfigId = localStorage.getItem('goclashz_activeConfigId') || ''; // 71
const cachedOutboundIP = localStorage.getItem('goclashz_outboundIP');               // 72
const savedProxyCardMode = localStorage.getItem('goclashz_proxy_card_mode');        // 75

// updateStateFromBackend 内，未包 try
localStorage.setItem('goclashz_hideLogs', String(newHideLogs));            // 346
localStorage.setItem('goclashz_activeConfigId', rawData.activeConfig);     // 402 / 405
```

**为什么是问题**：同文件与 utils/theme.ts 对同一个 API 采取了不同策略——store.ts:117-120 与 209-212 有 try/catch，utils/theme.ts:421-437 也全包了，唯独上面 8 处没有。一旦 setItem 抛 QuotaExceededError（隐私模式/配额），updateStateFromBackend 会**从中途抛出**：346 行之后的所有字段（logLevel、actual*、controlRevision、desired*、activeConfig*、language…）本次全部不更新，异常还逃进 Wails 事件派发器。模块级 getItem 抛错更直接：App 无法挂载。
WebView2 + http://wails.localhost 场景下 localStorage 通常可用，故实际触发概率低（证据 C），但代码一致性缺陷是确凿的（证据 A）。

**修复建议**：

```ts
function safeGet(key: string): string | null { try { return localStorage.getItem(key); } catch { return null; } }
function safeSet(key: string, v: string): void { try { localStorage.setItem(key, v); } catch { /* 忽略 */ } }
```

---

### S-06（P2）initStore 的订阅不可清理、被两处调用；storeInited 的置位时机

**位置**：frontend/src/store.ts:975-994、991-1174；frontend/src/main.ts:11-12；frontend/src/App.vue:756-757

```ts
let storeInited = false;                              // 975
export async function initStore() {
  if (storeInited) return;
  storeInited = true;                                 // 979 ← 在首个 await 之前就置位
  startPendingWatchdog();
  try { const initialState = await App.GetAppState(); updateStateFromBackend(initialState); }  // 984
  catch (err) { reportError(err, 'boot.initialState'); }
  Events.On("app-state-sync", ...);                   // 991 起 21 个订阅，返回值全部丢弃
  ...
}
```

```ts
// main.ts
bootstrapTheme();          // 11
initStore();               // 12  启动全局监听
createApp(App).mount('#app');

// App.vue
onMounted(async () => { initStore(); ... });   // 757  ← 已被 storeInited 短路，死调用
```

**为什么是问题**（均已自查「别处有没有兜住」）：

1. **双入口调用**：main.ts:12 已保证在挂载前开始初始化，App.vue:757 的调用恒为空操作 → 冗余（P2），并且传达错误信息（读者会以为初始化在这里）。
2. **订阅不可清理**：21 个 Events.On 的 unsub 全部丢弃，store 也没有 disposeStore()。生产环境是单页常驻，**无实际泄漏**；但它使「重启 store 状态」不可测试，也让任何未来的 HMR 边界变成泄漏源。
3. **HMR 结论（任务明确要求）**：全仓 import.meta.hot 命中 0 处（无 accept 边界）→ 改 store.ts 时 Vite 无法热替换，只能整页刷新 → storeInited 与监听器随页面一起重建，**当前不存在重复订阅**。也就是说 storeInited 在 HMR 下「仍然有效」不是因为自身健壮，而是因为项目没有 HMR 边界。一旦有人给 store.ts 加 import.meta.hot.accept，会立刻出现：旧模块 21 个监听继续跑（更新的是已废弃的旧 globalState 对象）+ 新模块 storeInited=false 但没人再调用 initStore（main.ts 不会重跑）→ 界面停在默认值、不再响应任何事件。
4. 附带：storeInited 在首个 await 前置位，若第一次调用时后端未就绪（GetAppState reject），错误被 catch、订阅仍会注册（这点是对的）；只是 storeInited 的语义从「初始化完成」变成了「初始化开始过」，命名与语义不符。

**修复建议**：

```ts
const unsubs: Array<() => void> = [];
export function disposeStore() { unsubs.splice(0).forEach((u) => u()); storeInited = false; }
// 每个 Events.On 改为 unsubs.push(Events.On(...))
// 若将来要 HMR：
if (import.meta.hot) import.meta.hot.dispose(() => disposeStore());
```

并删除 App.vue:757 的重复调用（或反过来只保留 App.vue 一处）。

---

### S-07（P2）modal 的 legacy 兼容分支已不可达；settleModal 不清空 onConfirm/onCancel

**位置**：frontend/src/store.ts:601-621、638-642、653-677（相关 545-587）

```ts
const legacyOccupied = globalState.modal.show && globalState.modal.onConfirm !== managedModalConfirm; // 638
if (activeModal || legacyOccupied) { modalQueue.push(entry); return; }
...
function settleModal(ok: boolean) {          // 569
  const entry = activeModal; activeModal = null;
  globalState.modal.show = false;
  globalState.modal.confirmText = ''; globalState.modal.cancelText = '';
  // ← 没有把 onConfirm/onCancel 置 null
  ...
}
export function confirmModal(): void {       // 653
  reconcileModalDisplacement();
  if (activeModal) { settleModal(true); return; }
  const legacy = globalState.modal.onConfirm;   // 660 ← 拿到的是上一条遗留的 managedModalConfirm
  globalState.modal.show = false;
  if (legacy) runModalAction(() => legacy());
  drainModalQueue();
}
```

**为什么是问题**（先自查是否有害，结论：无害但属死代码 + 一个语义坑）：

- 全仓 grep globalState.modal 的赋值点只有 store.ts 自己（presentModal/settleModal/confirmModal/cancelModal），组件侧 **0 处直写**（历史 F-A4-02 已修完）。因此 legacyOccupied 恒为 false、「被外部覆盖」分支、confirmModal/cancelModal 的 legacy 分支**全部不可达**。
- settleModal 不清 onConfirm/onCancel 使得 legacy 分支拿到的是 managedModalConfirm：若在无模态时误调 confirmModal()，会再进一次 settleModal(true)（entry 为 null，只把 show 置 false 并排空队列），无数据损坏，但语义上是「用残留 handler 伪装成外部模态」。
- 保留这些分支的成本：读者无法判断「现在的契约到底是队列还是单槽」，正是历史 bug 反复出现的原因。

**修复建议**：删除 638-642 的 legacyOccupied、653-677 的 legacy 分支、601-621 的位移分支；settleModal 里补 globalState.modal.onConfirm = null / onCancel = null；modal 的写权限收进 store（导出只读类型）。

---

### S-08（P2）波形采样：与后端推流同频异相；refCount 可漂负；scaleFallAlpha 不可达

**位置**：frontend/src/trafficWaveState.ts:9-12、111-151、67-72；触发源 frontend/src/App.vue:808-826、后端 core/appcore/controller.go:631 / core/appcore/streams.go:150

```ts
const WAVE = { sampleIntervalMs: 900, maxPoints: 96, ... };   // 10
...
export function startWaveSampling() {                          // 137
  refCount++;
  if (sampleTimer === null) sampleTimer = window.setInterval(pushVisualSamples, WAVE.sampleIntervalMs);
}
export function stopWaveSampling() {                           // 144
  refCount--;
  if (refCount <= 0 && sampleTimer !== null) { clearInterval(sampleTimer); sampleTimer = null; refCount = 0; }
}
```

**为什么是问题**：

1. **拍频**：后端流量流固定 900ms（clash.APIURL("/traffic?interval=900")，controller.go:631；推送 ticker 900ms，streams.go:150），前端采样也是 900ms 且**独立计时**。两个等周期不同相位的定时器必然出现「同一份 latestUpload 被采样两次」和「某两次推流被跳过一次」，肉眼就是波形里的平台/突跳点。修复成本极低。
2. **refCount 漂负**：负数只有在 sampleTimer 非空时才被归零，因此多调用一次 stopWaveSampling 会把它压到 -1 并永久保留；之后一次 start 只加回 0，此时定时器已建但 refCount<=0 → 下一次 stop 会立刻停表，即使还有消费者。当前只有 App.vue 一处 start/stop 配对（onMounted/onUnmounted），故为**潜在**问题而非现网 bug。
3. scaleFallAlpha 不可达：updateScale 里 target = Math.max(instant, prevScale) 恒不小于 prevScale，所以 fall 分支必然 delta=0（无效果）→ fall alpha 是死参数，「缩放会回落」的意图没有实现（若本意是回落，需要改成 target = instant）。

**修复建议**：

```ts
// 1) 采样改为跟随事件推进，而不是独立定时器：
export function pushTrafficSample(up: number, down: number) { /* 由 traffic-data 事件直接调用 */ }

// 2) refCount 钳位
export function stopWaveSampling() {
  refCount = Math.max(0, refCount - 1);
  if (refCount === 0 && sampleTimer !== null) { clearInterval(sampleTimer); sampleTimer = null; }
}
```

---

### S-09（P2）日志级别规整有两份不一致实现，'silent' 在日志面板被当作 'info'

**位置**：frontend/src/store.ts:317-323；对照 frontend/src/App.vue:396-405

```ts
// store.ts
const normalizeLogLevel = (level?: string) => {
  const v = String(level || '').toLowerCase().trim();
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (['debug', 'info', 'warn', 'warning', 'error', 'silent'].includes(v)) return v;  // 321 保留 silent
  return 'info';
};
```

```ts
// App.vue（日志面板滤镜）
const logLevels = { debug: 0, info: 1, warn: 2, warning: 2, error: 3 };   // 396 — 没有 silent
const normalizeLogType = (t?: string) => {
  const v = String(t || 'info').toLowerCase().trim();
  if (v === 'warning') return 'warn';
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (['debug', 'info', 'warn', 'error'].includes(v)) return v;
  return 'info';                                                          // 404 — silent 落到 info
};
```

**为什么是问题**：内核日志级别 silent 是 Mihomo 的合法值（core/clash/config.go:395 注释：debug, info, warning, error, silent），SettingsBehavior.vue 的 logLevelOptions 也提供了 silent。用户选择 silent 后，后端确实不再产出新日志；但日志面板的「设置级别地板」用 rankOf(globalState.logLevel) 计算，normalizeLogType('silent') 返回 'info' → 地板变成 info → **已经存在的历史日志继续显示 info/warn/error**，用户会认为「设了静默还是刷屏」。同一概念在前端存在两份归一化函数且取值域不一致（store 接受 silent，App 不接受）；同一概念还散落在 Settings.vue:174-176 的补丁式转换（warn → warning）与 SettingsBehavior.vue:259/267（core 用 warning、app 用 warn）——共 4 处。

**修复建议**：抽到 utils/normalize.ts：

```ts
export const LOG_RANKS = { debug: 0, info: 1, warn: 2, error: 3, silent: 4 } as const;
export function normalizeLogLevel(v?: string): keyof typeof LOG_RANKS { /* 唯一实现，silent 保留 */ }
```

App.vue / store.ts / Settings.vue / SettingsBehavior.vue 全部改为调用它（settingsFloor = LOG_RANKS[level]，silent=4 时任何条目 rank<4 全部过滤掉）。

> 注：可见症状的修复点在 App.vue:396-405（队友范围），我按跨文件移交处理，见 §7-1。

---

### S-10（P2）api.ts 三门面合并：当前无冲突，但没有不变量保护

**位置**：frontend/src/api.ts:13-17（全文 17 行）

```ts
import * as AppService from '../bindings/goclashz/appservice';
import * as CoreService from '../bindings/goclashz/coreservice';
import * as UpdateService from '../bindings/goclashz/updateservice';

export const App = {
  ...AppService,     // 31 个导出
  ...CoreService,    // 62 个导出
  ...UpdateService   // 15 个导出
};
```

**验证结果（A 级，脚本化，非猜测）**：

| 检查 | 结果 |
|---|---|
| 三个 binding 文件的导出形态 | 全部是 export function：31 / 62 / 15，**无** const/class/type/default 导出 |
| 合并后重名（后展开静默覆盖） | **0 个**。108 个导出名，108 distinct |
| 前端全部 App.Method( 调用点 | 82 个不同方法（第 83 个命中是 api.ts 自己的注释），**全部能在导出集合里解析**，无未知方法 |
| 与 Go 服务方法对应 | 抽查 9 个「只在一侧出现」的方法名，均在 services_v3_generated.go 中由对应 service 实现（如 AppService:123 GetHelperServiceStatus、CoreService:351 ExplainDNS）→ 三个 namespace 与 Go 三个 service 一一对应 |
| 未被前端调用的导出 | 26 个（多为 ServiceStartup / ShowMainWindow 等由 Go/框架侧调用），**不构成死代码结论**（C） |

**为什么仍记为问题**：TS 对 {...a, ...b} 的**同名不同签名不会报错**（后者静默胜出，类型也变成后者），所以这条不变量全靠人肉维护。三个文件都是自动生成的，未来任何一侧新增同名方法（例如 AppService 与 CoreService 都想加 GetStatus）就会静默串味，且错误现场极难发现。

**修复建议**（任选，成本都很低）：

```ts
// 1) 编译期：显式断言多余/缺失键
type Dup = { [K in keyof typeof CoreService & keyof typeof AppService]: never };
const _noOverlap: Dup = {};   // 任一侧出现同名键，这里就会报错

// 2) 更省事：CI 脚本比对三份 binding 的导出名集合（10 行 node 脚本）
```

---

### S-11（P2）IP 刷新：手动刷新不取消已调度的 route settle 定时器 → 一次点击两次请求

**位置**：frontend/src/store.ts:767-816

```ts
function scheduleRouteAwareIPRefresh(reason: string) {          // 767
  ...
  if (routeSettleTimer) clearTimeout(routeSettleTimer);         // 782
  routeSettleTimer = window.setTimeout(() => { routeSettleTimer = null; ... refreshOutboundIP({...expectedRoute: route}); }, delay);
}
export function refreshOutboundIPRouteAware(reason = 'manual') { // 801
  const route = getEffectiveOutboundRoute();
  if (route === 'switching') { globalState.outboundIPStale = true; return; }
  return refreshOutboundIP({ force: true, ..., expectedRoute: route });  // ← 不清 routeSettleTimer
}
```

**为什么是问题**：路由刚变化（proxy 开关后 600-1500ms 的延迟窗口）时用户点 TrafficCard 的刷新按钮（TrafficCard.vue:11），手动请求立即发出；随后 settle 定时器照旧触发 → 第二次请求。再叠加 refreshOutboundIP 的 pending 重放（store.ts:919-923），一次用户点击最多产生 3 次后端往返（有界，不会堆积——pendingIPRefresh 是单槽 latest-wins，ipRefreshTimers 命中后即删除）。属于「多余请求」而非「旧结果覆盖新结果」：seq 门控（839/856）确实能挡住乱序。

**修复建议**：refreshOutboundIPRouteAware 开头加 if (routeSettleTimer) { clearTimeout(routeSettleTimer); routeSettleTimer = null; }；settle 回调里加 if (globalState.ipDetecting) return;（避免叠在一次进行中的请求后面）。

---

## 4 冗余对照表

| # | 重复内容 | 位置 | 建议抽到哪 |
|---|---|---|---|
| 1 | 空 OutboundIP 结果对象**逐字段手写两遍**（'检测失败' / '网络请求失败'） | store.ts:876-888 vs 898-910 | makeEmptyOutboundIP(message)，与已有的 normalizeOutboundIP（52-66）放一起 |
| 2 | 日志级别归一化两份、取值域不同 | store.ts:317-323 / App.vue:396-405 / Settings.vue:174-176 / SettingsBehavior.vue:259,267 | utils/normalize.ts（当前该文件只有 1 个死导出，正好复用） |
| 3 | 主题持久化三条并存路径：applyThemeToApp 写 globalState.theme + localStorage 键 'goclashz_theme'（114-119）、App.vue watchTheme（744-749）调 App.SaveThemePreference、后端 syncThemeSettingFile | store.ts:106-125、App.vue:744-749 | 保留一条：applyThemeToApp 只管应用，持久化统一由 App.vue 的 watch 触发；并在 store 注释里写死「谁写盘」 |
| 4 | modal 结算/清理逻辑三份（settleModal、confirmModal legacy、cancelModal legacy 各写一遍 show=false + runAction + drain） | store.ts:569-587、653-664、667-677 | 抽 closeAndDrain(runFn) |
| 5 | 硬编码中文兜底串绕过 i18n | store.ts:628（'操作确认'/'提示'）、884（'检测失败'）、906（'网络请求失败'） | 用 t('common.confirm') 等已有 key；缺的补 8 语言（与历史 F-A4-16 同类） |
| 6 | delayTimers 与 ipRefreshTimers 两套「按 key 存 timer + 触发前 delete」样板 | store.ts:170/459-478 与 708/724-740 | 抽 keyedTimeout 小工具（收益中、风险低） |

---

## 5 已核查且**无问题**的点（覆盖度证明）

> 以下都是逐行读过 + 必要时脚本化验证后确认**没有问题**的结论，不是「没看」。

### 5.1 api.ts（关键线索）

1. **三个 service 无同名导出**：108 个导出全部为 export function，名字集合 108 distinct，**不存在静默互相覆盖**（脚本比对）。详见 S-10。
2. 82 个 App.Xxx( 调用点全部可解析，无拼写漂移、无遗留旧 service 名。
3. 三个 namespace 与 Go 的 AppService/CoreService/UpdateService 一一对应（抽查 9 个边界方法确认）。
4. 26 个未被调用的导出不属于死代码结论（由 Go/Wails 生命周期调用），未计入问题。

### 5.2 模态队列（关键线索：给出确定结论）

**结论：队列本身健全，历史 U4 / F-A4-01 的「Promise 永不 settle」已修复，未发现新的悬挂路径。**

逐项核对的不变量（全部成立）：

| 不变量 | 依据 |
|---|---|
| showAlert **不能**顶掉 pending 的 showConfirm | showAlert → enqueueModal（682-691 → 623-645），已存在 activeModal 时只 modalQueue.push（639-642）；重名 alert 直接 resolve（687-690） |
| 同一时刻最多一个受管模态 | presentModal 只由 enqueueModal（643）与 showNextModal（565）调用，两处前置都检查了 activeModal / modal.show（562 / 639） |
| 不会 double-resolve | settleModal 先取走 activeModal 再 resolve（570-581），第二次进入时 entry 为 null |
| 被外部关闭/覆盖必定结算 | reconcileModalDisplacement（601-621）按取消结算，被 confirmModal / cancelModal / 看门狗三处调用 |
| 队列不会饿死 | settleModal 与 drainModalQueue 都会 setTimeout(showNextModal, 0)，且 showNextModal 有幂等守卫 |
| modalSeq 回绕 | 不存在——JS number 到 2^53 才失精度，单进程实际不可达（非问题） |
| 快速连点「确定」 | 第二击时 activeModal 已是 null，只会重复 drain（无副作用）；残留 handler 说明见 S-07 |
| 遮罩点击/ESC | App.vue:179 @click.self="handleModalCancel" → cancelModal() → 按取消结算，语义正确 |

历史复现路径（F-A4-01 的原始剧本）逐条走查后**已不会发生**：TUN 弹确认框期间后台 app-update-available 到达 → openModal 入队（App.vue:885）而不是覆盖 → 用户的 confirm 仍在 activeModal → 点「确定」结算 TUN 流程 → 下一帧再出队更新弹窗。

**唯一残余的「两个模态同时驻留」不在队列里**，而在 App.vue 的自建模态（历史 F-A4-07 残留），见 §6-1。

### 5.3 出站 IP 刷新链路

1. **stale response 防护有效**：const seq = ++ipDetectSeq（839）→ 结果返回后 if (seq !== ipDetectSeq) return（856、892），旧结果不会覆盖新结果。
2. **in-flight 互斥有效**：if (globalState.ipDetecting) → 单槽 pendingIPRefresh 收纳（828-837），finally 里以 setTimeout(0) 重放（919-923）→ latest-wins，不丢新请求、不堆积。
3. **路由判定与后端一致**：getActualOutboundRoute（751-755）与后端 currentOutboundRoute（core/appcore/outbound_ip.go:187-196）逐条等价（tun-route → proxy ≙ actualSystemProxy && isRunning → direct）。
4. **expectedRoute 只在路由已稳定时下发**：getEffectiveOutboundRoute（757-762）在 desired≠actual 时返回 'switching'，两个入口都提前 return（771-773、804-807），避免自己制造路由竞态。
5. **定时器不堆积**：ipRefreshTimers 同 key 覆盖（724-726）、触发前 delete（733）；routeSettleTimer 单槽且变化时清理（782-784）。
6. 路由切换中的结果按语义保留旧值（859-864，isRouteSwitchingMessage），不会被写成「检测失败」。
7. **类型契约 1:1**：store.ts:38-50 的 OutboundIPResult 与 Go OutboundIPResult（core/appcore/outbound_ip.go:60-71）11 个字段逐一对应（含 complete）；normalizeOutboundIP 用 ?? 处理 false、用 '' 处理空串，null/undefined/部分字段均安全（52-66）。

### 5.4 updateStateFromBackend 的字段覆盖（关键线索）

- **AppState 共 28 个字段，前端消费 27 个（theme 故意跳过），无遗漏、无多余**：isRunning / isAdmin / mode / theme（故意跳过，理由见 337-341）/ hideLogs / logLevel / appLogLevel / desiredSystemProxy / desiredTun / desiredPortProxy / actualSystemProxy / actualTun / controlRevision / systemProxy（legacy 兜底）/ tun（legacy 兜底）/ version / appVersion / activeConfig / activeConfigName / activeConfigType / delayRetention / delayRetentionTime / updateReady / newAppVersion / updateDownloaded / downloadedPath / portProxyMode / language。
- **「缺字段把已有值写成 undefined/false」不成立**：Go 的 AppState 全部字段无 omitempty（core/appcore/controller.go:519-559），JSON 里恒存在；且前端每个分支都有 !== undefined 守卫（330-436），属**部分更新**语义，缺字段保持原值。
- **null 数组 / 空对象**：AppState 无数组字段；事件侧 component-update-tasks-sync 有 if (tasksList) 守卫（1058），空数组会正确把 tasks 清空。
- **revision 门控有效**：revision >= globalState.controlRevision（371）挡住乱序旧事件覆盖 desired*；actual* 不受门控（366-368）——语义正确（actual 是即时事实）。
- **类型变化**：hideLogs / delayRetention 等布尔用 ??（不会把 false 当缺失）；actualSystemProxy / portProxyMode 用 !! 归一；language 仅在真值时 setLocale（435-436）。
- **UpdateTaskState 契约 1:1**：store.ts:23-36 vs Go component_update_state.go:7-19，12 个字段名/类型一致。

### 5.5 主题

- themeState **只有一个写点**（store.ts:111-112），SettingsTheme / App.vue 全部只读派生 → 无「多处写同一状态」。
- bindSystemTheme 返回 unsub，App.vue:771 保存、:979 清理 → 系统主题监听**不会重复注册**（onSystemThemeChange 每次 new matchMedia listener，见 utils/theme.ts:334-341）。
- applyThemeToApp 是唯一入口，顺序正确（applyTheme → cacheTheme → 回写 behavior → themeState → globalState.theme → refreshGlassTint，106-125）；localStorage.setItem 有 try（116-120）。
- readCachedTheme / cacheTheme 对 JSON 与异常全兜（utils/theme.ts:421-438），坏缓存不会崩。
- initialThemeConfig 用 resolveIsDark 解析（95-98），注释解释了「不能写死 false」的原因 → 无首帧回写 bug。

### 5.6 Toast

- id 单调递增（++toastSeq，1231），**不存在同 id 复用**；v-for 用 :key="item.id"（Toast.vue:6）而非 index。
- 淘汰（while length > 3 shift，1240-1242）安全：被淘汰项的定时器稍后触发 dismissToast(id) → findIndex 返回 -1 → 无操作，**无异常、无重复删除**。
- 组件卸载后 dismiss 只改全局数组，不触碰已卸载组件状态。
- delayTimers（proxy 延迟保留）在 updateProxyDelay（460-463）与 delay-cache-clear（1136-1140）两处都清理了，无泄漏。

### 5.7 其他

- controlOverlay 的三态（visible / desired / actual）职责清晰，systemProxyPending 只挡重复点击不影响亮灭（226-243 注释）——设计本身正确，问题只在 S-01 / S-03 的边界。
- resolveNotificationUi 对未知 level 回落 toast（972），不会因为后端新增 level 抛错；level 取值域 success/info/warn/error 与 Go notify() 实参一致（逐条核对 21 处 c.notify( 调用）。
- **事件契约**：store 的 21 个 Events.On 中 **20 个有真实发射端**。最容易误判的 core-update-start/success/error/cancelled 与 core-update-check-* **不是**字面量 Emit，而是任务框架按 name + "-start" 统一发出（core/tasks/manager.go:91/99/103/109），任务名 core-update / core-update-check 来自 updater_orchestrator.go:108/159 —— 逐条对上，**无死监听**（唯一例外 driver-install-success 见 §6-3）。
- scheduleOutboundIPRefresh 的 clearBeforeStart 只清显示值不清定时器，行为符合注释；同 key 重复调度会正确覆盖。
- proxy-delay-update 的 busy 过滤（1151）、非 manual 失败保留旧值（1161-1165）与注释意图一致。
- settings/searchState.ts：模块级 ref 用于跨组件重建保留搜索词，符合注释意图；无并发写、无生命周期问题。
- buildMonotoneAreaPath：空数组/单点有兜底路径（187-189），clamp01 挡住 NaN / 负值 / 超界输入，数值稳定。
- themeState 与 globalState.theme 的语义分工（config=偏好，theme=实际生效）与 App.vue watchTheme 的回写条件（746-748）一致，未发现自激循环。

---

## 6 已知问题（历史已记录，不计入本轮新问题数）

| 旧 ID | 旧严重度 | 标题 | 本轮复核结论 | 位置 |
|---|---|---|---|---|
| **F-A4-07** | P1 | 自建模态与全局模态并存（双层遮罩） | **仍未修复（台账标记 fixed(B2) 但残留）**。App.vue 同时存在两个独立 overlay：全局队列模态（App.vue:179）与「种子内核替换」自建模态 showCoreSeedModal（App.vue:211-239），两者都 class="modal-overlay"、无互斥。该卡片可见时若发生 showAlert（如启动期 STARTUP_RESTORE_* 通知、或 driver/helper 错误），alert 会被压在下面不可见，其 Promise 直到用户关掉种子卡片才结算（不会永久悬挂，故未升级为新问题）。**这是「队列与 activeModal 是否可能同时驻留两个」的唯一肯定答案：队列内部不会，App.vue 的旁路模态会。** | App.vue:179、211 |
| **F-A4-14** | P1 | 前端用中文字面量正则判断后端语义 | **部分残留（A 级）**。isRouteSwitchingMessage（utils/errors.ts:127-129）仍靠 includes('路由切换中')；根因是 OutboundIPResult **没有 Code 字段**（Go struct 见 core/appcore/outbound_ip.go:58-71，bindings/goclashz/core/appcore/models.ts:338 同样无 Code），而后端已在 notify 通道给出 ROUTE_SWITCHING code（outbound_ip.go:233）。长连接那条已由 code 表覆盖（DEFERRED_RESTART_*），仅剩这一处。 | store.ts:859、utils/errors.ts:127 |
| **F-A5-23** | P2 | 事件契约死链 / 同名双发 | **残留 1 条死链（A 级）**：driver-install-success 在 Go 侧只有 application.RegisterEvent 注册（v3_events.go:168），**全仓 0 处 Emit**；任务框架的 6 个 task 名里也没有 driver-install（updater_orchestrator.go:159/427/434/507、component_update_service.go:55、subscriptions.go:75）。因此 store.ts:1133（驱动装完刷 IP）、SettingsTun.vue:217、SettingsUpdate.vue:103（刷新资产）都不会触发。实际完成信号是 wintun-version-updated（updater_orchestrator.go:224，SettingsUpdate.vue:102 已监听），且 SettingsTun 有 onAfterInstall 兜底（SettingsTun.vue:169-172）→ **用户可见影响小，按历史项登记**。 | store.ts:1133 |
| F-A4-01 | P0 | 单一模态槽导致 confirm Promise 悬挂 | **已修复，本轮复核通过**：activeModal + modalQueue + managed handler + 5s 看门狗四重保障，见 §5.2。 | store.ts:484-703 |
| F-A4-02 | P0 | 10 处直写 globalState.modal | **已修复**：全仓 grep 仅 store.ts 自己写 modal，组件侧 0 处。 | — |
| F-A4-18 | P2 | app-notification 过滤规则过窄 | **已修复**：source 白名单已删除（store.ts:997-999 注释），改为 code 表 + level 回落。新问题只剩「表内容与后端脱节」（S-04）。 | store.ts:999 |

---

## 7 需要跨文件 / 后端确认的疑点（移交主控）

按优先级排列，每条都带可直接定位的证据：

1. **【高，App.vue 队友】normalizeLogType 缺 'silent' → 日志面板 silent 档失效**（详见 S-09）。修复点：App.vue:396-405 + logLevels 加 silent: 4；同一概念在 store.ts:317-323 已正确，可互相引用。
2. **【高，App.vue 队友】自建模态与全局模态并存**（F-A4-07 残留）。建议直接复用 openModal / showConfirm 替换 showCoreSeedModal，或至少在 showCoreSeedModal=true 期间禁止全局模态呈现（store 侧加一个 externalModalOpen 闸门）。
3. **【中，后端】OutboundIPResult 加 Code 字段**（F-A4-14 收尾）。否则 isRouteSwitchingMessage 永远只能文案匹配，后端改文案即静默降级。改动点：core/appcore/outbound_ip.go 结构体 + GetOutboundIPForRoute 的切换分支返回值 + 重新生成 bindings。
4. **【中，后端/设置页】SaveAppBehavior 整对象写是丢失更新的根因**（S-02）。若不便加 SaveThemeMode，至少在 Go 侧给 behavior 加版本号或改 PATCH 语义；否则「侧栏改主题 + 设置页保存」这条竞态永远存在。
5. **【中，后端】失败回滚后是否有周期性 SyncState？**（S-03 的最终自愈依赖）。已确认：supervisor 的 5s watchdog（core/appcore/supervisor.go:78/130）只在 reconcile 内部状态变化时 SyncState，**没有无条件周期广播**。若希望前端在失败场景自愈，需明确一个「状态心跳」（或按 S-03 建议由前端失败后主动 GetAppState）。
6. **【低】getRecentErrors 无消费者**：utils/report.ts:38 导出、全仓 0 引用，而 43 处 reportError 写进这个环。意味着 store.ts:1008-1010 的「A 类静默」实际上只到 console，既不进 UI 也不进后端日志——与「silent 也要留痕」的设计目标不符。建议二选一：接到日志面板/诊断包，或直接转发后端日志 API。
7. **【低】utils/normalize.ts 是死文件**：唯一导出 normalizeStartupTaskInfo（18 行）全仓 0 引用。正好可作为 §4-2「日志级别归一化」的新家。
8. **【低】SettingsAbout.vue 仍有 3 处硬编码中文**（:25「新版本已就绪，点击安装」、:29「剩余」、:36「下载完成」），与历史 F-A4-16（标记 fixed(B3)）同类残留，需 locale 8 语言补 key。
9. **【备案】notify-error 仍是死事件**：v3_events.go:47 注释已承认（「前端只订阅 app-notification」），后端也不再发；无需处理，仅登记避免下一轮重复发现。

---

## 附：本轮使用的验证手段（可复现）

1. **初始化**：team_task_get / team_task_update claim task-1（共享任务板）。
2. **导出面比对**：提取三份 binding 的 export function 名（31/62/15），并集 108 → Sort-Object -Unique 计数比对重名（0 个）。
3. **调用点解析**：正则扫 frontend/src 全部 .ts/.vue 中的 App.方法名( 调用 → 82 个不同方法，逐一比对导出集合（1 个 Method 命中来自 api.ts 注释）。
4. **事件发射端反查**：Go 侧正则提取 Emit("名字") 得 43 个名字，与 store.ts 的 21 个 Events.On 名做差集，再对差额逐个 Select-String 定位；最终确认任务框架 name + "-start" 约定（core/tasks/manager.go）覆盖了 6 个「看似缺失」的事件。
5. **消息 code 双向比对**：前端 NOTIFICATION_UI 的 15 个 key 逐个在 Go 全仓搜索命中数（7 个为 0）；反向列出 c.notify( 全部 21 处调用的 level+code 组合，与表做差集。
6. **modal 死代码判定**：grep globalState.modal 全仓，赋值点仅 store.ts。
7. **HMR 边界**：grep import.meta.hot / accept( → 0 处。
8. **时间**：2026-11-18 20:00-20:1x（+08:00）。
