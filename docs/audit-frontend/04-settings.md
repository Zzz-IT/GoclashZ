# 04 设置模块前端只读审计（Settings* / manifest / ModernSelect / ModernNumberInput / UpdateTask*）

- **审计角色**：settings-ui（前端只读审计代理，共享任务 task-4）
- **审计时间**：2026-11-18 20:05 (+08:00)
- **判据**：① 代码冗余 ② 逻辑错误 ③ 操作冲突（重点）④ bug（泄漏/卸载后改状态/key/空值）⑤ 前后端对接
- **结论预告**：本轮**未发现 P0**；P1 共 **5** 条、P2 共 **19** 条。最值得先看的是 G-02（校验形同虚设）、G-03（"反选"其实是全不选）、G-05（还原备份后被首次保存回滚）。
- **重要前提**：本轮以**工作区当前内容**为准（`git status` 显示 SettingsTheme.vue / manifest.ts / locales/* 有未提交修改）。历史审计 `docs/audit/A4-frontend.md`（主题＝提示面与错误处理）的发现多为**已修复**，残留项单独列在 §6，不计入新问题数。

## 1. 审计范围与覆盖度

### 1.1 实际读过的文件（前端，共 ~5,900 行）

| 文件 | 总行数 | 覆盖情况 |
|---|---|---|
| components/Settings.vue | 257 | 全文 |
| components/SettingsHome.vue | 233 | 全文 |
| components/SettingsTun.vue | 236 | 全文 |
| components/SettingsDns.vue | 329 | 全文 |
| components/SettingsNetwork.vue | 484 | 全文 |
| components/SettingsLan.vue | 423 | 全文 |
| components/SettingsBehavior.vue | 376 | 全文 |
| components/SettingsTheme.vue | 996 | 模板 1-267 + 脚本 269-667 + CSS 669-784（余下 784-996 为纯样式） |
| components/SettingsAbout.vue | 520 | 1-469（模板 + 全部脚本 + CSS 主体） |
| components/SettingsDiagnostics.vue | 357 | 全文 |
| components/SettingsUwp.vue | 256 | 全文 |
| components/SettingsUpdate.vue | 116 | 全文 |
| components/ModernSelect.vue | 319 | 全文 |
| components/ModernNumberInput.vue | 162 | 全文 |
| components/UpdateTaskPanel.vue | 119 | 全文 |
| components/UpdateTaskRow.vue | 210 | 全文 |
| settings/manifest.ts | 1012 | 全文（1-260 / 260-560 / 560-810 / 810-1009 分段读完）+ 脚本化解析 103 条条目 |

> 任务书里写"13 个 Settings*.vue"，实际 glob 只命中 **12 个**（没有第 13 个）；`SettingsHome` 已含在内。

### 1.2 为判定"前后端对接"而额外读的后端/binding 文件

| 文件 | 用途 |
|---|---|
| app.go（380-630、1296-1545） | SaveAppBehavior / ResetComponentSettings / CheckTunEnv / InstallTunDriverAsync / ExportBackup / ExecuteRestore / GetDataDirInfo |
| core/appcore/behavior.go（95-404、474-620） | BehaviorStore.Get/SetAndSave/Update、normalizeBehavior、Default() |
| core/clash/config.go（150-400、600-712） | TunConfig/DNSConfig/NetworkConfig 默认值、GetDefaultXxx、hosts 注入、LAN IP 注入 |
| core/appcore/controller.go（452-512、2380-2470） | syncStartupTaskStateSafe 写 behavior、SaveAppBehavior 副作用与回滚 |
| core/appcore/component_update_service.go / component_update_state.go / tasks/manager.go / geo_update_manager.go | 任务状态机、事件名、并发闸门 |
| core/appcore/backup_service.go（1-110） | RestoreBackup 会重载 behavior |
| core/utils/settings.go（全文） | LoadSetting 的错误契约（**永不返回 error**） |
| v3_events.go（全文） | 事件名/类型注册表 |
| frontend/bindings/goclashz/{app,core,update}service.ts + core/*/models.ts | 方法签名、返回类型、字段名 |
| docs/audit/A4-frontend.md、ISSUE_LEDGER 相关行 | 历史去重 |

### 1.3 未覆盖 / 未验证

- 未运行任何构建、`vue-tsc`、应用本体（按规则由主控统一跑）；所有运行时结论均为静态推断，证据等级已标注。
- SettingsTheme.vue 784-996 行（约 210 行纯 CSS）未逐行读；SettingsAbout.vue 469-520 行（CSS 尾部）未读。
- 未审计 store.ts 的 toast/watchdog 全貌、utils/theme.ts、composables/useTunToggle 的内部实现（只读了接口与 pending 语义），这些属于其他代理范围；涉及跨文件的结论已移交 §7。

### 1.4 证据等级

- **A**：代码可直接确证（读到唯一决定路径的源码）。
- **B**：代码 + 运行时推断（依赖时序/异步/浏览器行为，但路径清晰）。
- **C**：假设待实测（文中显式标注）。

---

## 2. 问题总表

| ID | 严重度 | 类别 | 位置(file:line) | 一句话问题 | 证据 |
|---|---|---|---|---|---|
| G-01 | P1 | 操作冲突 | SettingsLan.vue:11,181 / Settings.vue:10,157,233-235 | 局域网页"重置"发的是 'network' 段名 → 重置后 LAN 页永不重载，且随后任一编辑把重置前的 LAN 值写回 | A |
| G-02 | P1 | 逻辑错误 + 对接 | SettingsLan.vue:280-287,223-228 / core/clash/config.go:621-625,703-712 | CIDR 校验结果被丢弃，非法 IP/CIDR 照样落盘并原样注入运行时 YAML（manifest 声称"校验通过才保存"） | A/B |
| G-03 | P1 | 逻辑错误 | SettingsUwp.vue:29,143-146 + locales/zh-CN.ts:745 | 按钮文案"反选"，实际动作是"全部取消豁免" | A |
| G-04 | P1 | 操作冲突 | Settings.vue:202-216 / store.ts:134-145 | AppBehavior 全量写回且无串行化，侧栏主题开关是"读-改-写全量"，两个入口互相覆盖 | B |
| G-05 | P1 | 操作冲突 | SettingsAbout.vue:355-367 / core/appcore/backup_service.go:82 | 还原备份后后端 behavior 已换，前端快照不刷新 → 之后第一次保存把还原结果覆盖回旧值 | B |
| G-06 | P2 | 逻辑错误 | Settings.vue:166 / SettingsNetwork.vue:270-285 | 兜底默认对象与 Go Default() 有 3 处值不一致，且 Network 兜底缺 5 个 LAN 字段（全量保存会清零） | A |
| G-07 | P2 | bug | ModernNumberInput.vue:48,57-81 | 输入中间态不 clamp、NaN 回落 min ?? 0（可能与后端下限冲突）、step 非响应式；无 Enter/Esc | A |
| G-08 | P2 | 操作冲突 | ModernSelect.vue:55-61,140-144 | 挂载期"自动纠错"会 emit change → 直接触发父级保存；只监听 modelValue 不监听 options | B |
| G-09 | P2 | 冗余（死代码） | ModernSelect.vue:208-214,276-280 | Teleport 到 body 后 .w-full .select-dropdown / .drop-up / z-index:100 三条规则永不生效 | A |
| G-10 | P2 | bug/泄漏 | UpdateTaskPanel.vue:39-46 | deep watch 每次任务同步都排一个 3s 定时器，卸载不清理；清理会把 error 任务一并删掉 | A/B |
| G-11 | P2 | bug | UpdateTaskRow.vue:68-96 | 4 个更新 RPC 全部无 catch；pause/resume 无本地 in-flight（仅靠后端 beginOrJoin 兜住） | A |
| G-12 | P2 | 逻辑错误 | SettingsBehavior.vue:283-294 | watch 监听 prop 字段并回写 + emit('save')，父级换 behavior 对象时也会命中（内置弹窗） | A |
| G-13 | P2 | 逻辑错误 | SettingsTheme.vue:2 / SettingsBehavior.vue:2 / SettingsAbout.vue:2 | 三页 loading 期间不禁用控件（其余页禁用）→ 可在数据未到前编辑并整份保存 | A |
| G-14 | P2 | 逻辑错误 | SettingsTheme.vue:630-653 | "恢复默认"漏掉 colorDelay；showConfirm 未传 isDanger（其余重置均传 true） | A |
| G-15 | P2 | 冗余 + 潜在 bug | SettingsTheme.vue:306,310,609 / Settings.vue:11,157,163 / app.go:559-586 | theme 重置链路三处死代码：emit('reset') 未使用、resetVersions.theme 永不递增、后端无 theme 分支 | A |
| G-16 | P2 | 冗余/文档漂移 | manifest.ts:486,1008 | manifest 两处与实现不符（LAN "校验通过才保存"、UWP "写 SaveUwpExemptions"） | A |
| G-17 | P2 | 逻辑错误 | SettingsNetwork.vue:290-317,336-348 + core/clash/config.go:493-497 | hosts 校验返回值未使用，非法 hosts 照样保存；后端 YAML 解析失败静默忽略 | A/B |
| G-18 | P2 | 操作冲突 | SettingsUwp.vue:135-146 | "全选/反选"作用于未过滤全集，搜索时看到的与被改的不一致 | A |
| G-19 | P2 | 逻辑错误 | SettingsAbout.vue:48-52,228-233 | 语言选择只接 update:modelValue（不接 change），setLocale 先于保存结果生效，失败后界面语言与后端不一致 | B |
| G-20 | P2 | bug | SettingsAbout.vue:191,264,302-312,355-360 | 备份/还原只认 "SUCCESS" 字面量（其余静默）；Browser.OpenURL 无 catch；t(...) || '兜底' 死兜底 | A |
| G-21 | P2 | bug | SettingsUpdate.vue:103,108,110 | 事件回调里 void refreshRuntimeAssets() 无 catch → 未处理的 Promise 拒绝；pendingCoreUpdate 的 watch 与 store 全局取消互相顶 | B |
| G-22 | P2 | 冗余（本地化） | utils/format.ts:37-50（调用点 SettingsUpdate.vue:32） | formatRelativeTime 硬编码中文，8 语言下漏译 | A |
| G-23 | P2 | 冗余（死配置） | Settings.vue:166 / core/appcore/behavior.go:606-612 / utils/glass.ts:24-28 | 4 个玻璃强度字段（含 legacy glassBlurStrength）全链路无消费者 | A |
| G-24 | P2 | 冗余 | 12 个设置页 | 页面骨架（头部/开关/分隔线/加载-保存）与 CSS 大段复制，见 §4 | A |

---

## 3. 逐条详情

### G-01 [P1] 局域网页"重置"发错段名：对话框文案错、重置后页面不重载、编辑会把旧值写回

**位置**：frontend/src/components/SettingsLan.vue:11、:181；frontend/src/components/Settings.vue:10、:157、:233-235

**现状**

```vue
<!-- SettingsLan.vue:11 -->
<button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'network')">
<!-- SettingsLan.vue:181 -->
defineEmits<{ navigate: [view: 'main']; reset: [module: 'network'] }>();
```

```ts
// Settings.vue:10（LAN 页的 resetKey 绑的是 resetVersions.lan）
<SettingsLan v-else-if="view === 'lan'" :reset-key="resetVersions.lan" ... @reset="confirmReset" />
// Settings.vue:233-235
const apiModule = resetModule.value === 'lan' ? 'network' : resetModule.value;
await App.ResetComponentSettings(apiModule);
resetVersions.value[resetModule.value as keyof typeof resetVersions.value] += 1;   // ← 这里被写成 'network'
```

```ts
// SettingsLan.vue:290 —— 只监听自己的 resetKey
watch(() => props.resetKey, () => { void loadData(); });
```

**为什么是问题（触发路径）**

1. 用户在"局域网"页点重置 → resetModule = 'network' → resetVersions.network += 1，**resetVersions.lan 永远是 0**。grep 确认全仓只有 Settings.vue:235 一处自增，且没有任何组件 emit 'lan' —— LAN 页的重载通道整条是死的。
2. 后端 ResetNetworkConfig 已把 user_network.json 删掉（LAN 字段与端口/外部控制器同存 NetworkConfig），但页面仍显示重置前的值（lanConfig 未重载）→ 用户看到"点了没反应"。
3. 更糟的是下一步：SettingsLan.save() 是"重新 Get + 只覆盖 LAN 字段"（SettingsLan.vue:259-268），本地 lanConfig 还是重置前的旧值 → 用户随便改一个字段，就把刚被重置掉的 lanAuthUser/lanAuthPass/lanAllowedIPs/... **原样写回**，重置实际被回滚。
4. 确认框文案也错：modules[resetModule] 取到 settings.home.network（"网络"），在局域网页弹"确定要将 **网络** 恢复为默认设置吗"，却把 LAN 认证一起清了。

**修复建议**（最小改动两行）

```ts
// SettingsLan.vue:11
@click="$emit('reset', 'lan')"
// SettingsLan.vue:181
defineEmits<{ navigate: [view: 'main']; reset: [module: 'lan'] }>();
```

Settings.vue:233 的 'lan' → 'network' 映射本来就在，改完即：对话框显示"局域网"、resetVersions.lan 正常自增、页面重载。另外建议 save() 在 loadData 未完成时直接 return（见 G-06 的 loaded 闸门）。

---

### G-02 [P1] CIDR 校验只用于画警告，非法值照样落盘并注入运行时 YAML

**位置**：frontend/src/components/SettingsLan.vue:205-228、:280-287；core/clash/config.go:621-635、:703-712；frontend/src/settings/manifest.ts:486

**现状**

```ts
// SettingsLan.vue:280-287
const saveAndValidate = async (key: string) => {
  validateCIDR(                                   // ← 返回值被丢弃
    key === 'allowed' ? lanConfig.lanAllowedIPs :
    key === 'disallowed' ? lanConfig.lanDisallowedIPs : lanConfig.lanSkipAuthIPs,
    key
  );
  await save();                                   // ← 无条件保存
};
```

```ts
// manifest.ts:486（自称的契约）
note: '... @input 逐行正则校验、@blur 校验通过才保存，非法行会阻止保存并报行号',
```

```go
// core/clash/config.go:621-625 + 703-712：不做任何校验，原样写进运行配置
if list := splitIPLines(userNet.LanAllowedIPs); len(list) > 0 {
    root["lan-allowed-ips"] = list
}
func splitIPLines(s string) []string { /* 只跳过空行和 # 注释 */ }
```

**为什么是问题**

- validateCIDR 返回 boolean，validateAll()（:223-228）甚至没有任何调用点 —— 整套校验只负责渲染 .validation-error 红字，用户失焦即保存后脏数据进磁盘。
- 运行时链路：user_network.json → BuildRuntimeConfig → root["lan-allowed-ips"] → mihomo 解析 netip.Prefix。非法条目（例如 192.168.1.0/33、aaa）会让**内核加载配置失败**，表现为"改了个 IP 白名单，内核起不来了"。用户端唯一线索是一行和操作无关的核心错误。
- 正则本身也不严：/^((\d{1,3}\.){3}\d{1,3}(\/...)?|[0-9a-fA-F:]+\/\d{1,3})$/ 接受 999.999.999.999，且**拒绝无前缀的 IPv6**（::1 判非法）—— 误放 + 误杀同时存在。
- 证据等级：前端 A；"内核起不来"是 B（依赖 mihomo 的 lan-allowed-ips 解析语义，见 §7-7 待实测）。

**修复建议**

```ts
// SettingsLan.vue:280
const saveAndValidate = async (key: string) => {
  const ok = validateCIDR(..., key);
  if (!ok) return;              // 校验不过不落盘（红字已经在 @input 时显示）
  await save();
};
// 同时删掉未使用的 validateAll()（:223-228），或让 save() 入口统一走它
```

并把正则换成 net.isIP/net.isIPv4/net.isIPv6 逐段校验 + 前缀位数范围检查（不需要完美，只要不与后端冲突）。

---

### G-03 [P1] "反选"按钮的实际行为是"全部取消豁免"

**位置**：frontend/src/components/SettingsUwp.vue:29、:143-146；frontend/src/locales/zh-CN.ts:745（en-US.ts 同）

**现状**

```vue
<!-- SettingsUwp.vue:28-30 -->
<button class="batch-btn" @click="toggleAllUwp(true)">{{ t('settings.uwp.selectAll') }}</button>
<button class="batch-btn" @click="toggleAllUwp(false)">{{ t('settings.uwp.invertSelect') }}</button>
```

```ts
// SettingsUwp.vue:143-146
const toggleAllUwp = (val: boolean) => {
  // 「全不选」= 全部关闭，不是逐项取反（F-A4-20：原实现会把已关闭的反向打开）
  uwpApps.value.forEach(app => { app.isEnabled = val; });
};
```

```ts
// locales/zh-CN.ts:745 / en-US.ts:745
invertSelect: '反选',   // English: 'Invert'
```

**为什么是问题**

历史 F-A4-20 修的是"实现是取反"，改成了置 false，但**按钮文案仍是"反选/Invert"**：用户期望 A→B、B→A，实际得到"全部取消"。配合本页的差异提交（added/removed 只提交变化项），一次误点 + 应用 = **把所有 UWP 环回豁免清空**（这些应用之后连不上本地回环代理）。这是可感知、可造成配置损失的错标动作。

**修复建议**：二选一，不要保持现状。
- 想保留"反选"语义：uwpApps.value.forEach(a => { a.isEnabled = !a.isEnabled });
- 想保留"全不选"语义：改文案（invertSelect: '全不选' / 'Select None'，8 个语言包同步），并顺手确认 settings.uwp.clearAllConfirm 的语义仍成立。推荐后者 —— 它和"清空全部豁免走独立确认"的设计一致。

---

### G-04 [P1] AppBehavior 全量写回没有串行化，侧栏主题开关是"读-改-写全量"

**位置**：frontend/src/components/Settings.vue:202-216；frontend/src/store.ts:134-145

**现状**

```ts
// Settings.vue:202-216 —— 每次 change/blur 都会走一遍，无 in-flight 闸门、成功后不重载
const saveBehavior = async () => {
  try {
    await App.SaveAppBehavior(behavior.value);         // 整份（含 theme/glass/log 等全部字段）
    ...
  } catch (error: any) {
    reportError(error, 'settings.saveBehavior');
    ...
    await loadBehavior();                              // 失败才重载
  }
};
```

```ts
// store.ts:134-145 —— 侧栏快捷明暗：Get 全量 → 只改 themeMode → Save 全量
export async function setThemeMode(mode: ThemeMode): Promise<void> {
  applyThemeToApp({ ...themeState.config, mode });
  const current: any = await App.GetAppBehavior();
  current.themeMode = themeState.config.mode;
  await App.SaveAppBehavior(current);                  // 整份写回
}
```

**为什么是问题**

App.SaveAppBehavior → BehaviorStore.SetAndSave(config)（behavior.go:121-137）是**整份结构覆盖**，不是字段合并（后端自己有 Update(mutate) 的原子读改接口，前端这层没用）。

触发路径（B 级，需时序配合）：
1. 设置页点"预设/圆角/玻璃" → saveBehavior 发 SaveAppBehavior(behavior)（含刚改的 themePreset），请求在途；
2. 同一瞬间用户在侧栏点明暗快捷开关 → setThemeMode 先 GetAppBehavior()。后端此时可能还没落盘新预设 → 读到旧预设；
3. setThemeMode 把"旧预设 + 新 themeMode"整份写回 → 设置页刚选的预设被静默回滚，而界面上预设卡片仍是选中态（themeState.config 已被前端改过）。

同源的还有两种：设置页连点两个选项 → 两个全量请求并发，落盘顺序由调度决定（内容单调，通常无害）；loadBehavior 只在"失败"与"重置"时触发，成功保存后不重载 → 前端缓存与后端长期可能不同步（见 G-05）。

**修复建议**
- 前端加模块级串行队列（let behaviorSaveChain = Promise.resolve() 把每次保存串起来），并把 setThemeMode 改成"只提交差异"：等后端补字段级接口，或至少把 current 的读取放进同一条 Promise 链；
- 后端更彻底：App.SaveAppBehavior 改为 Behavior.Update(func(b){...}) 只写前端真正拥有的字段（theme*/glass*/behavior 开关），避免任何调用方用整份结构覆盖他人改动。

---

### G-05 [P1] 还原备份后前端不刷新 behavior 快照，下一次保存会把还原结果覆盖回去

**位置**：frontend/src/components/SettingsAbout.vue:355-367；core/appcore/backup_service.go:82；对照 Settings.vue:170-201

**现状**

```go
// backup_service.go:81-90 —— 后端还原后重新 Load behavior / desired
if err := c.Behavior.Load(); err != nil { ... }
if err := c.Desired.Load(); err != nil { ... }
```

```ts
// SettingsAbout.vue:355-360 —— 前端只弹了成功提示，没有重新拉 behavior
const res = await App.ExecuteRestore(selectedPath.value, restoreMode.value);
if (res === "SUCCESS") {
  showRestoreModal.value = false;
  showToast(t('settings.about.restoreSuccess'), ...);   // 文案：设置及配置已即时生效
}
```

**为什么是问题**

- 备份文件里 Settings/user_behavior.json 属于 settings 类（core/backup/archive.go:652、:656-661），还原 settings/all 会替换它；后端已 Load 进内存，**但前端 behavior ref 仍是还原前的对象**，themeState 也没重算（store.ts:337-339 明确"刻意不消费后端 theme"）。
- 于是：还原后界面上一切照旧（和"已即时生效"的提示相矛盾）；用户改任意一个开关 → saveBehavior(behavior.value) 整份写回 → **被还原的 behavior 又被覆盖成还原前的值**。标题、玻璃、语言、日志级别、长连接保护等一起回滚。
- 相同机制还有"侧栏主题开关"：还原后点一下侧栏明暗，setThemeMode 会把 themeMode 写成前端的旧偏好（store.ts:139）—— 还原带来的主题偏好被一次点击回滚。
- 证据 B：还原不重启进程是读代码确认的（RestoreBackup 返回 nil 后前端只弹 toast）。

**修复建议**

```ts
// SettingsAbout.vue:357-361
if (res === "SUCCESS") {
  showRestoreModal.value = false;
  showToast(...);
  emit('reload-behavior');   // 新增事件，由 Settings.vue 调 loadBehavior()
}
```

推荐：SettingsAbout 新增 emit('reload') → Settings.vue loadBehavior()（内含 syncThemeFromBehavior）；或后端在 RestoreBackup 成功后补发 behavior-changed 事件（core/appcore/events.go:13 本来就有这个常量，目前无发射点），前端统一监听。

---

### G-06 [P2] 兜底默认对象与 Go 默认值不一致；Network 兜底缺 LAN 字段

**位置**：frontend/src/components/Settings.vue:166；SettingsNetwork.vue:270-285；core/appcore/behavior.go:566-601；core/clash/config.go:354-371

**现状（节选）**

```ts
// Settings.vue:166（节选）
{ ... delayRetention: false, delayRetentionTime: 'long', logLevel: 'info', appLogLevel: 'info',
  autoUpdate: true, updateMethod: 'startup', updateInterval: 3, ... }
```

| 字段 | 前端兜底 | Go Default() | 影响 |
|---|---|---|---|
| logLevel | 'info' | 'error' | 兜底值一旦被保存，内核日志从 error 变成 info（噪音/写盘量增加） |
| delayRetention | false | true | 延迟缓存被静默关闭 |
| updateInterval | 3 | 1 | 自动检查更新周期从 1 天变 3 天 |
| tcpKeepAliveInterval（Network 页:279） | 15 | 30 | 保活间隔被改成 15s |

SettingsNetwork 的兜底对象还**缺少 5 个 LAN 字段**（lanAuthUser/lanAuthPass/lanAllowedIPs/lanDisallowedIPs/lanSkipAuthIPs），而 saveNet() 是整份 SaveNetworkConfig(netConfig.value) → 若在兜底状态下保存，Go 侧反序列化缺失键为零值 → **LAN 认证与白名单被清空**（含密码）。

**为什么只判 P2（自评降级说明）**

utils.LoadSetting 的错误契约是"**永不返回 error**"（core/utils/settings.go:20-58，坏文件改名留档后回落默认值），所以 GetAppBehavior/GetNetworkConfig/GetTunConfig 实际上不会抛错，这些兜底对象在正常环境是**死代码**。保留 P2 是因为：① 它们是"看起来像真配置"的假数据，谁把 LoadSetting 改成返回错误就会立刻变成 P1；② 早于 loadBehavior 完成的那 1-2 帧里用户仍可操作（见 G-13），此时写盘的就是这些兜底值。

**修复建议**：让兜底对象等于 Go Default() 的镜像（或直接 null + loaded 闸门，见 SettingsUwp.vue:154 的 if (!uwpLoaded.value) return; 范式 —— 全项目只有 UWP 页做了这件事）。

---

### G-07 [P2] ModernNumberInput 的数值中间态 / NaN / step

**位置**：frontend/src/components/ModernNumberInput.vue:48、:57-81

**现状**

```ts
const step = props.step ?? 1;                 // ← 只在 setup 求值一次，props.step 变化无效（目前无人传 step）
const onInput = (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value);
  if (!isNaN(val)) emit('update:modelValue', val);     // ← 不 clamp
};
const onBlur = (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value);
  const clamped = isNaN(val) ? (props.min ?? 0) : clamp(val);   // ← 清空输入 = 回到 min 或 0
  emit('update:modelValue', clamped);
  emit('change', clamped);                     // ← 只有 blur / ± 按钮会触发 change
};
```

**为什么是问题**

- 输入 99999（max=1500 的 MTU、max=65535 的端口）时 modelValue 立刻变成 99999，只有失焦才 clamp；若父级把 update:modelValue 接到瞬时保存（本仓目前没有，都是 @change，所以只是潜在风险），越界值会进后端。
- 清空输入框后失焦 → min ?? 0：min 未定义的实例会写 0，而 Go 侧很多字段用 <=0 判定"未设置"并回落到别的默认值（如 AutoDelayTestInterval<=0 → 60，behavior.go:501-503），用户输入 0 会得到 60，看起来"没生效"。
- 无 Enter/Esc 处理：Enter 不 blur（@keyup.enter 只出现在 SettingsUpdate 的 DB 弹窗文本输入），Esc 不还原；与 SettingsTheme 的 hex 输入（有 @keydown.enter.prevent）不一致。
- step prop 无人传（grep：6 个调用点全部只传 min/max）→ 属"声明了但没人用"的冗余，加上非响应式写法，一旦有人传 step=5 再动态改就会静默失效。

**修复建议**：onInput 里也 clamp（并允许中间态空串用本地 draft 字符串承载）；onBlur 的 NaN 兜底改成"还原上一次合法值"而不是 min ?? 0；const step = computed(() => props.step ?? 1)；补 @keydown.enter / @keydown.esc。

---

### G-08 [P2] ModernSelect 挂载期自动纠错会触发父级保存；且只监听 modelValue

**位置**：frontend/src/components/ModernSelect.vue:55-61、:140-144

**现状**

```ts
// 自动修正非法值
watch(() => props.modelValue, (newVal) => {
  const match = props.options.find(o => o.value === newVal);
  if (!match && props.options.length > 0) {
    emit('update:modelValue', props.options[0].value);
    emit('change', props.options[0].value);      // ← 直接把"纠错"当成"用户选择"通知父级
  }
}, { immediate: true });
```

**为什么是问题**

- 父级普遍把 @change 当"用户改了 → 立刻保存"：SettingsBehavior.vue:155/170/183、SettingsTun.vue:27/36、SettingsAbout.vue:92。所以后端只要返回一个不在选项表里的值，页面一打开就会**把第一个选项写盘**（静默改配置）。
- 可达性评估（因此判 P2 而非 P1）：
  - logLevel 安全 —— normalizeBehavior 会过 clash.NormalizeLogLevel（behavior.go:475），输出必在选项表内；
  - appLogLevel **不安全** —— Go 只把空串补成 info（behavior.go:477-479），而 logger.NormalizeLevel 接受 warn/warning/trace/fatal/panic（core/logger/level.go:5-18）。手改过 user_behavior.json 或从旧版本带来的 "warning" 会被 ModernSelect 静默改成 "debug" 并保存；
  - enhancedMode/stack/delayRetentionTime 同理（Go 只在空串时兜底）。
- 另外 watch **只监听 modelValue 不监听 options**：选项是 computed（如 stackOptions 依赖 tunStatus.supportsMips），值不变而选项集变化时不会重新校验（当前靠 SettingsTun:144-147 手工兜 mips，属补丁而非机制）。
- DNS 页之所以没出事，是因为 saveDns 有 if (loading.value) return; 把这次误触发挡掉了（SettingsDns.vue:237）—— **这是巧合兜底，不是设计**；而 saveBehavior 完全没有 loading 闸门（G-13）。

**修复建议**：自动纠错只 emit('update:modelValue')，不发 change；change 只在 selectOption()（用户点击）里发。若必须纠正，加一个 suppress-change 标记。

---

### G-09 [P2] ModernSelect 的 Teleport 让三条 CSS 规则变成死代码

**位置**：frontend/src/components/ModernSelect.vue:10-18（Teleport）、:208-214、:269、:276-280

**现状**

```vue
<Teleport to="body"><transition name="fade"><div ref="dropRef" class="select-dropdown" ...>
```

```css
.modern-custom-select.w-full .select-dropdown { width:100%; max-width:100%; left:0; right:0; }  /* 208-214 */
.select-dropdown.drop-up { top:auto; bottom:calc(100% + 6px); }                                  /* 276-280 */
.select-dropdown { ... z-index:100; ... }                                                        /* 269 */
```

**为什么是问题**：下拉层已被 Teleport 到 body，不再是 .modern-custom-select 的后代，后代选择器（208-214）永远匹配不到；drop-up/z-index 又被 updatePosition() 写的内联样式（:80-104，zIndex:'10001'、显式 top/bottom）覆盖。功能上已被内联样式补偿（w-full 分支在 :90-96 显式写 width），所以只是**冗余与误导**：改这三条 CSS 不会有任何效果。建议删掉，或加注释标明"位置/尺寸由 updatePosition() 接管"。

---

### G-10 [P2] UpdateTaskPanel：定时器不清理、清理会把错误任务一起删

**位置**：frontend/src/components/UpdateTaskPanel.vue:39-46；core/appcore/component_update_state.go:67-85

**现状**

```ts
watch(tasks, (newTasks) => {
  const hasFinished = newTasks.some(t => t.status === 'success');
  if (hasFinished) {
    setTimeout(() => { clearFinishedTasks(); }, 3000);   // ← 每次触发都新开一个，且没有 clearTimeout
  }
}, { deep: true });
```

```go
// ClearFinished 把 success / error / cancelled 全都删掉
if v.Status == "success" || v.Status == "error" || v.Status == "cancelled" { delete(s.tasks, k) }
```

**为什么是问题**

- tasks 是 Object.values(globalState.componentUpdate.tasks).sort(...)，而 store 在每次 component-update-tasks-sync 都会**整体替换** tasks 对象（store.ts:1056-1062）；下载进度期间后端每个进度回调都会 Set 并广播一次（component_update_service.go:71-80）→ deep watch 每次都命中。只要列表里有一个 success 任务，就会不断累积"3 秒后清理"的定时器。
- 定时器**没有在 onUnmounted 清理**：离开设置页后仍在跑，继续打 ClearFinishedUpdateTasks。
- 语义层面：一次成功更新会顺手把用户还没读完的 **error / cancelled 任务行一并删掉**（后端 ClearFinished 的定义如此），3 秒后错误信息就消失了。
- 影响有限（后端 ClearFinished 幂等、无变更不发事件），故 P2。

**修复建议**：改成"仅在有 success 且列表非空时排**一个**定时器 + 卸载时清理"，或用 debounce；并把清理范围收窄为 success（前端自己过滤 key 调 RemoveUpdateTask）以保留错误信息。

---

### G-11 [P2] UpdateTaskRow 四个更新 RPC 全无错误处理

**位置**：frontend/src/components/UpdateTaskRow.vue:68-96

**现状**

```ts
const handleTogglePause = async () => {
  if (props.task.status === 'running') { await App.CancelUpdateTask(props.task.key); }
  else { await startTask(); }
};
const handleRemove = async () => { await App.RemoveUpdateTask(props.task.key); };
const handleForceRetry = async () => { await App.ClearUpdateCache(props.task.key); await startTask(); };
```

**为什么是问题**：CancelUpdateTask/RemoveUpdateTask/ClearUpdateCache/UpdateGeoDatabaseAsync 在 Go 侧都是无返回值的包装（app.go:1325-1352），正常情况下不会 reject；但 RPC 通道异常时会抛 → 无 catch → 未处理的 Promise 拒绝（控制台红字、按钮无反馈）。与同目录其他组件的 reportError(...) 约定不一致。
另外 pause/resume 没有本地 in-flight 标记，连点两次会发两次请求 —— 后端 GeoUpdateManager.beginOrJoin（geo_update_manager.go:227-244）与 runComponentUpdateTransaction（component_update_service.go:39-43）都有幂等闸门，**这条不构成实际竞态**，记录为"缺前端闸门但后端兜住"。
v-for 用的是 task.key（UpdateTaskPanel.vue:8），**不是 index**，列表重排安全 ✓。

**修复建议**：包一层 try/catch + reportError + showToast(失败)。

---

### G-12 [P2] SettingsBehavior 的 updateInterval watch 会回写 prop 并弹窗

**位置**：frontend/src/components/SettingsBehavior.vue:283-294

**现状**

```ts
watch(() => props.behavior.updateInterval, async (newVal) => {
  if (newVal !== undefined && newVal <= 0) {
    props.behavior.updateInterval = 1;                     // ← 改父级对象
    if (props.behavior.autoUpdate && props.behavior.updateMethod === 'scheduled') {
      await showAlert(t('settings.behavior.intervalWarning'), ...);   // ← 用户没做任何操作就弹窗
    }
    emit('save');                                          // ← 顺手落盘
  }
});
```

**为什么是问题**：这个 watch 不区分"用户输入"与"父级替换对象"—— loadBehavior() 把 behavior.value 换成新对象时也会触发。若后端值 <=0 就会在**打开页面时**弹警告并写盘一次。可达性：Go normalizeBehavior 会把 UpdateInterval<=0 夹成 1（behavior.go:497-499），所以今天**不会触发**；属"防御代码写法错误 + 死路径"，一旦有人放宽后端 clamp 就直接变成 P1（进页误弹窗 + 误保存）。另外 ModernNumberInput 的 min=1 已经让用户拖不到 0，这个 watch 重复兜底。

**修复建议**：删除该 watch（组件已有 min=1 + 后端 clamp）；若保留，改为在 ModernNumberInput 的 @change 里判断，或仅在用户交互标记为真时处理。

---

### G-13 [P2] behavior 系三页在 loading 期间不禁用控件

**位置**：SettingsTheme.vue:2、SettingsBehavior.vue:2、SettingsAbout.vue:2（对照 SettingsDns.vue:19/27/34、SettingsNetwork.vue:27、SettingsTun.vue:27、SettingsLan.vue:27）

**现状**

```vue
<!-- 三页都只有 no-anim，控件没有 :disabled="loading" -->
<div class="settings-page ui-page-shell" :class="{ 'no-anim': loading }">
<!-- 反例：DNS 页每个控件都带 :disabled="... || loading" -->
<input type="checkbox" v-model="dnsConfig.enable" @change="saveDns" :disabled="loading">
```

**为什么是问题**：Settings.vue 的 behaviorLoading 一开始是 true，loadBehavior 完成前 behavior 是**兜底对象**（G-06）。这三页此时控件可点，一点就 emit('save') → 把兜底值 + 这一处改动整份写盘，等于清掉真实配置里的主题/玻璃/日志等字段。窗口只有 1-2 帧（rAF×2 解除），但慢机器/首次进设置页时确实存在。三页里只有 SettingsTheme 的动画抑制用到了 loading，语义不统一。

**修复建议**：给三页的交互控件补 :disabled="loading"（与 DNS/Network/Tun 一致），并在 saveBehavior 里加 if (behaviorLoading.value) return; 兜底。

---

### G-14 [P2] 主题"恢复默认"漏项，且危险确认未标 isDanger

**位置**：frontend/src/components/SettingsTheme.vue:630-653

**现状**

```ts
const handleReset = async () => {
  const confirmed = await showConfirm(
    t('settings.resetConfirmMsg', { name: t('settings.theme.title') }),
    t('settings.resetConfirmTitle')            // ← 少传 isDanger=true（其余重置：Settings.vue:222-226 传 true）
  );
  ...
  config.value = normalizeThemeConfig(DEFAULT_THEME);
  behavior.glassBlurEnabled = false;
  behavior.glassTintStrength = GLASS_TINT_DEFAULT;
  behavior.glassDepth = GLASS_DEPTH_DEFAULT;
  // ← 没有重置 behavior.colorDelay（本页"数据显示"分组的唯一开关）
  emit('save');
};
```

**为什么是问题**：① 同一语义的动作在 Settings.vue（:222-226，isDanger=true）与本页（无）走两种危险等级样式，属一致性缺陷；② 页面上"恢复默认"看起来覆盖整页，但"彩色延迟"保持原值，用户无法预期（要么把它一起重置，要么把它挪到行为页）。

**修复建议**：showConfirm(msg, title, true)；在 handleReset 里补 behavior.colorDelay = false（Go 默认 false），或明确注释"colorDelay 属行为设置，不在主题重置范围"。

---

### G-15 [P2] theme 重置链路的三处死代码（含一个潜在 500）

**位置**：SettingsTheme.vue:306、:310、:609；Settings.vue:11、:157、:163；app.go:559-586

**现状**

```ts
// SettingsTheme.vue:310 —— 声明了 reset 但从没 emit
const emit = defineEmits(['navigate', 'reset', 'save']);
// SettingsTheme.vue:609 —— resetKey 只有父级自增才变化，而父级不会为 theme 自增
watch(() => [props.resetKey, props.behavior], syncFromBehavior);
```

```ts
// Settings.vue:157 / 163 —— 两个入口都没人用
const resetVersions = ref({ tun: 0, dns: 0, network: 0, lan: 0, theme: 0, behavior: 0 });
const modules = computed<Record<string, string>>(() => ({ ... theme: t('settings.theme.title') ... }));
```

**为什么是问题**：resetVersions.theme 与 modules.theme 永不被使用（grep resetVersions 只有 Settings.vue:235 一处自增，且只会取到 tun/dns/network/behavior）；emit('reset') 是无人消费的声明。潜伏风险：App.ResetComponentSettings 的 switch 只有 behavior/network/dns/tun（app.go:559-586），**没有 theme 分支**，默认分支返回 unknown settings section: theme。如果谁把 SettingsTheme 的按钮改回 @click="$emit('reset','theme')"（看起来是最自然的写法），用户会看到"重置失败：unknown settings section: theme"。附带：Settings.vue:163 的 modules.theme 说明作者原计划走统一重置，现状是"半迁移"。

**修复建议**：删掉 SettingsTheme 的 'reset' 声明与 resetKey prop（保留 watch(props.behavior) 做玻璃同步）；或二选一补齐 —— 后端加 case "theme": 重置 theme* 字段，前端改回统一路径。不要保持"声明存在但走不通"的中间态。

---

### G-16 [P2] manifest 有两处与实现不符（文档漂移）

**位置**：frontend/src/settings/manifest.ts:486、:1008

**现状**

```ts
// manifest.ts:486（set-lan-allowedIPs）
note: '... @blur 校验通过才保存，非法行会阻止保存并报行号'
// manifest.ts:1008（set-uwp-applyBtn）
note: '提交时只把 isEnabled=true 的 SID 列表写入 SaveUwpExemptions'
```

**为什么是问题**：manifest 自称"单一数据源（用于搜索索引 + 后续 schema 化）"，但 ① LAN 的校验实际不阻止保存（见 G-02，同一 bug 的文档面）；② UWP 已改为差异提交 ApplyUwpExemptionChanges(added, removed) / 空集走 ClearAllUwpExemptions（SettingsUwp.vue:169-200），SaveUwpExemptions 在整个前端**已无调用点**（grep 只命中这行注释），binding 仍在（appservice.ts:141）。错误的文档在 schema 化时会被当真。

**修复建议**：两条 note 按实现改写；SaveUwpExemptions 若无其他消费者，交回后端决定是否删除（见 §7-3）。

---

### G-17 [P2] hosts 校验只警告不拦截，后端解析失败静默忽略

**位置**：frontend/src/components/SettingsNetwork.vue:290-317、:336-348；core/clash/config.go:493-497

**现状**

```ts
watch(() => netConfig.value.hosts, (newVal) => { validateHosts(newVal || ''); });   // 只写 hostsError
const saveNet = async () => {
  if (loading.value) return;
  await App.SaveNetworkConfig(netConfig.value as any);      // 不看 hostsError
};
```

```go
if userNet.Hosts != "" {
    var hostsMap map[string]interface{}
    if err := yaml.Unmarshal([]byte(userNet.Hosts), &hostsMap); err == nil {   // 失败即忽略
        root["hosts"] = hostsMap
    }
}
```

**为什么是问题**：validateHosts 的返回值（boolean）无人使用，非法行仍会落盘；后端把解析失败**静默吞掉** —— 用户看到红色警告、继续编辑、保存成功、hosts 映射却完全不生效，且没有任何"未生效"提示（对比：network/tun/lan 有 DeviationStrip 偏差条，hosts 不在其覆盖范围）。风险低于 G-02（不会让内核起不来），故 P2。

**修复建议**：saveNet 前判 if (!validateHosts(netConfig.value.hosts)) return;；或把后端"hosts 解析失败"作为一条 deviation 暴露给 UI。

---

### G-18 [P2] UWP 全选/反选忽略搜索过滤

**位置**：frontend/src/components/SettingsUwp.vue:135-146

**现状**

```ts
const filteredUwpApps = computed(() => uwpApps.value.filter(app => ...q...));
const toggleAllUwp = (val: boolean) => { uwpApps.value.forEach(app => { app.isEnabled = val; }); };
```

**为什么是问题**：搜索状态下列表只显示 3 个应用，用户点"全选"却改了全部 200 个（含不可见的）。之后点"应用更改"会把不可见的 197 个也提交。用户无法在界面上核对将发生什么（列表只显示过滤结果）。属"看到的 ≠ 改到的"操作冲突。

**修复建议**：批量操作作用于 filteredUwpApps（用户看到的就是被选的），或按「有搜索词时改过滤集、无搜索词时改全集」并在按钮上提示影响的条数。

---

### G-19 [P2] 语言选择：不接 change、先切语言后等保存

**位置**：frontend/src/components/SettingsAbout.vue:48-52、:228-233

**现状**

```vue
<ModernSelect :modelValue="behavior.language || currentLocale" :options="SUPPORTED_LOCALES"
              @update:modelValue="handleLanguageChange" />    <!-- 只有 update，没有 change -->
```

```ts
const handleLanguageChange = (val: any) => {
  const lang = String(val) as SupportedLocale;
  props.behavior.language = lang;
  setLocale(lang);        // 立即切界面语言
  emit('save');           // 不等结果
};
```

**为什么是问题**：setLocale 先改 localStorage + 全局 currentLocale，保存失败时父级 saveBehavior 的 catch 会 loadBehavior() → 里面又会 setLocale(value.language)（Settings.vue:187-189）→ 界面语言**在用户眼前跳回去**，且此前没有任何"正在保存"的提示。写法上只有这一处用 @update:modelValue 而不用 @change，与全项目约定不一致（虽然对语言这种"即时生效"控件是合理的，但需要显式处理失败）。
SUPPORTED_LOCALES 的 8 个 value 与 Go normalizeBehavior 的白名单完全一致（behavior.go:522-527），所以**不会**触发 G-08 的自动纠错 —— 这点已核查无问题。

**修复建议**：改成 @change="handleLanguageChange" 并在 saveBehavior 失败时用 setLocale(behavior.value.language) 回滚（或让 saveBehavior 支持返回值/回调）。

---

### G-20 [P2] 备份/还原与外部链接的返回值/异常处理

**位置**：frontend/src/components/SettingsAbout.vue:191、:264、:302-312、:355-367

**现状**

```ts
const handleExportBackup = async () => {
  const res = await App.ExportBackup();
  if (res === "SUCCESS") { showToast(...); }        // CANCELLED / 其它一律静默；无 else
};
const confirmRestore = async () => {
  const res = await App.ExecuteRestore(...);
  if (res === "SUCCESS") { ... }                    // 同上
};
const openLink = (url) => { Browser.OpenURL(url); }; // 无 await / catch
```
```vue
<button ... @click="confirmRestore">{{ isRestoring ? (t('common.loading') || '处理中...') : ... }}</button>
```

**为什么是问题**：① 后端确实会返回 "CANCELLED"（app.go:1450-1457），静默取消是合理的，但**除 SUCCESS/CANCELLED 外的任何字符串**（未来新增状态）也会静默 → 建议显式 switch；② Browser.OpenURL 是 Promise，失败时未处理；③ t() 在键缺失时返回**键路径本身**（locales/index.ts:64-67），所以 t('common.loading') || '处理中...' 的 || 分支是死代码 —— 同理 :306/:345/:348/:359 共 5 处（历史 F-A4-17 遗留写法）。

**修复建议**：删 5 处 || '兜底'；openLink 加 .catch；ExecuteRestore 返回值用穷举处理。

---

### G-21 [P2] SettingsUpdate 事件回调里的 void refreshRuntimeAssets() 无 catch

**位置**：frontend/src/components/SettingsUpdate.vue:103、:108、:110

**现状**

```ts
unsubs.push(Events.On('driver-install-success', () => { void refreshRuntimeAssets(); }));      // :103
['geoip','geosite','mmdb','asn'].forEach(key => unsubs.push(
  Events.On("geo-update-" + key + "-success", () => { void refreshRuntimeAssets(); })          // :108
));
```

**为什么是问题**：refreshRuntimeAssets 内部 await App.GetRuntimeAssetStatus() 没有 try/catch（:76-84），而事件回调里的 void 会吞掉拒绝 → 未处理的 Promise 拒绝。同一文件里 refresh() 有 try/catch（:86），SettingsTun.refreshRuntimeAssets 也有（:123-129）—— 三处同功能函数，两种写法。
附带一处状态互相顶的隐患：watch(() => globalState.componentUpdate.pendingCoreUpdate, v => { if (v) showCoreUpdateConfirm.value = true }, {immediate:true})（:110）与 cancelCoreUpdateConfirm() 里直接写 globalState.componentUpdate.pendingCoreUpdate = false（:88）—— 后者是**全局** store 字段，取消本页的确认框等于替全应用取消了"待更新"状态；同时 executeCoreUpdate 又不清该字段（:89）。目前 core-update-* 事件会回收（store.ts:1087-1098），未观察到卡死，但语义混乱（P2）。

**修复建议**：给 refreshRuntimeAssets 包 try/catch（与 Tun 页对齐）；cancelCoreUpdateConfirm 只关本地 showCoreUpdateConfirm，不要改 store 的 pendingCoreUpdate（或反过来统一由 store 提供 dismissCoreUpdate()）。

---

### G-22 [P2] formatRelativeTime 硬编码中文

**位置**：frontend/src/utils/format.ts:37-50（调用点 SettingsUpdate.vue:32）

**现状**

```ts
if (!timestamp) return '从未';
if (diff < 60000) return '刚刚';
if (diff < 3600000) return Math.floor(diff / 60000) + ' 分钟前';
```

**为什么是问题**：设置 → 组件与库 的 GeoIP/GeoSite/MMDB/ASN 行会显示"3 小时前"，在 en-US/ja-JP/zh-TW/de-DE 下仍是中文。这是我在本范围内读到的**唯一**漏译点（其余 i18n key 全通过，见 §5-2）。函数在 utils/format.ts（非我范围），所以只登记调用点，修复需改该文件 + 8 语言包新增 4 个 key。

**修复建议**：改为 formatRelativeTime(ts) 返回 {key, params} 或接受 t 函数注入；或在调用点用 Intl.RelativeTimeFormat。

---

### G-23 [P2] 4 个玻璃强度字段全链路无消费者（前后端死配置）

**位置**：Settings.vue:166；core/appcore/behavior.go:606-612；frontend/src/utils/glass.ts:24-28、:267-283

**现状**

```ts
// glass.ts:24-28 —— 前端只认两组
export interface GlassStrengthGroups { tint?: number; depth?: number; }
// applyGlassEffect 只读 groups.tint / groups.depth（:273/:278）
```

```go
// behavior.go:606-612
GlassBlurStrength:        defaultGlassBlurContent,   // 旧字段，"仅保留做迁移"
GlassBlurContentStrength: defaultGlassBlurContent,
GlassBlurOverlayStrength: defaultGlassBlurOverlay,
GlassBlurControlStrength: defaultGlassBlurControl,
```

**为什么是问题**：前端**唯一**引用这 4 个字段的地方是 Settings.vue:166 的兜底对象（grep 确认），既无 UI 也无 CSS 消费者；UI 早已收敛成"厚度总控 glassDepth + 染色 glassTint"（SettingsTheme.vue:184-186 注释说明了为什么不要分级滑条）。但 Go 侧注释仍写"玻璃层按 3 组独立半径…任一组独立生效"（appcore/models.ts:108-113 同步了这段注释）。属**死配置 + 文档矛盾**，会误导后续维护者以为存在 4 条滑条。

**修复建议**：后端确认是否可删（或保留 legacy 只做迁移、从 API 结构里摘掉）；前端把 4 个字段从兜底对象删掉，避免"看起来能配"。

---

### G-24 [P2] 设置页骨架与样式的复制粘贴（详见 §4）

**位置**：12 个 Settings*.vue

**为什么是问题**：每个子页都重复了同一套"头部（返回按钮 + 标题 + 重置按钮）"、"setting-group/setting-item/divider/modern-switch/slider/modern-input/sub-header/mini-btn-reset"样式块（约 35-60 行/页，合计 ~500 行重复 CSS），以及"loading ref + loadData + rAF×2 解除 no-anim + save + watch(resetKey)"结构（5 页逐字相同）。改一处交互（例如加载失败提示）需要改 5 个文件。具体清单见 §4。

---

## 4. 冗余对照表

| 重复内容 | 出现位置 | 建议抽取 |
|---|---|---|
| 子页头部（back-btn + h3 + mini-btn-reset） | SettingsTun:3-11、SettingsDns:3-11、SettingsNetwork:3-11、SettingsLan:3-14、SettingsBehavior:3-11、SettingsTheme:4-13、SettingsDiagnostics:3-12、SettingsUpdate:3-8、SettingsUwp:3-8、SettingsAbout:3-8 | <SettingsPageHeader :title @back @reset>（含"诊断/更新/UWP 无重置按钮"的 showReset 开关） |
| .setting-group / .setting-item / .col-item / .divider / h3 / h4 / p / .info | 12 个页面各一份 | 提到 styles/surfaces.css，或做 components/SettingsSection.vue |
| .modern-switch / .slider / input:checked 开关样式 | Tun:229、Dns:314-320、Network:407-414、Lan:384-390、Behavior:329-336、Theme:421-427、About:421-427 | 抽 components/ModernSwitch.vue（目前每页各维护一套选中/禁用视觉） |
| .modern-input / .modern-textarea | Tun:228、Dns:310-313、Network:393-405、Lan:332-343,365-379、Behavior:318-327 | 抽 components/SettingInput 或全局 class |
| loadData() 的 loading=true → try → finally → rAF×2 | Tun:131-166、Dns:219-234、Network:319-334、Lan:230-252（Settings.vue:194-200 同型） | utils/useAsyncPageLoad.ts：返回 {loading, initializing, run}，统一 no-anim 时序 |
| save() 的"loading 闸门 + reportError + toast/alert + deviationRef.refresh" | Tun:192-211、Dns:236-244、Network:336-348、Lan:254-278、Settings.vue:202-216 | utils/saveWithFeedback.ts（历史 F-A4-06 建议过同名文件，目前仍是 5 份实现） |
| 装驱动的"confirm → InstallTunDriverAsync" | SettingsTun:182-187 与 SettingsUpdate:90 | 抽到 useTunToggle 或 utils/installDriver.ts |
| refreshRuntimeAssets | SettingsTun:123-129（有 catch）与 SettingsUpdate:76-84（无 catch） | 合并 + 统一错误处理（G-21） |
| watch(() => props.resetKey, () => loadData()) | Tun:219、Dns:296、Network:351、Lan:290 | 复用 useAsyncPageLoad；同时修 G-01 |
| "split('\n') → trim → filter(空/注释)" | Network:296-311（YAML 版）、Lan:210-221（CIDR 版）、Dns:249/256/263（数组版） | utils/parseLines.ts 一套解析 + 各页只给校验器 |
| 重复/无效的 i18n 语义 | settings.behavior.sec60 / min5 无任何选项引用（manifest.ts:685 自己注明）；settings.resetConfirmMsg 在 overview(zh-CN.ts:115) 与 settings(zh-CN.ts:337) 各一份且语义不同 | 删死 key；两份 resetConfirmMsg 改名区分（overview.trafficResetConfirm） |
| as any 绕类型 | Network:339、Lan:269、Settings.vue:204、Tun:107/112-121、Dns:194/223/249、Lan:196 | 给 App.Save* 传 bindings 已生成的 $models.XConfig 类型，或本地定义 NonNullable<...> |

---

## 5. 已核查且无问题的点（覆盖度证明）

1. **manifest ↔ 模板锚点一一对应**：脚本化比对，103 条 SETTING_FIELDS 的 id **全部**在对应页面的 id="..." 里存在（含 4 条由 :id="'set-update-dbFile'+..." 动态生成的 GeoIP/GeoSite/MMDB/ASN），无重复 id、无孤儿锚点、无 page 字段为空。
2. **i18n 完整性**：8 个语言包各 **796 键**，我的 17 个文件里静态 t('...') / titleKey / group 引用 **0 缺失**（以 zh-CN 为基准、其余 7 包交叉验证）。历史上"777 键 + ja-JP 缺 14 处"的记录已修复。
3. **事件名全部真实存在**：driver-install-success、core-update-*、geo-update-<key>-success 由 core/tasks.Manager 的 Emit(name+"-success") 动态发出（tasks/manager.go:91-110），任务名 core-update/driver-install 见 updater_orchestrator.go:108/206；geo 事件名见 geo_update_manager.go:139/214。SettingsTun:217、SettingsUpdate:100-108、SettingsAbout:375 的监听名**逐个对得上**，且都在 onUnmounted 注销。
4. **ModernSelect 没有监听器泄漏**：每个实例在 onMounted 注册 document.click + window.scroll(capture) + window.resize，onUnmounted 三个都移除（ModernSelect.vue:160-175），并清掉模块级 activeSelectId。任务书里"每实例都挂 document 监听 = 泄漏"的假设**不成立**（属必要实现）。
5. **托盘不改行为设置**：tray_runtime_windows.go 的菜单只有 显示窗口/系统代理/TUN/出站模式(3)/重启内核/退出（:340-373），**没有任何 behavior 写入**（全文无 Behavior 引用）。任务书"行为设置在 SettingsBehavior 与托盘菜单都能改"的假设不成立 —— 真正的多写者见 G-04/G-05。
6. **TUN 开关已是单一实现**：useTunToggle 统一了首页与设置页，pending 复用 globalState.tunPending（useTunToggle.ts:36），两处入口互斥；失败回滚由 handleTunToggle 写回 input.checked（SettingsTun.vue:174-180）。历史 F-A4-03 已修复，无重复写。
7. **null 返回处理正确**：GetTunConfig 的 dnsHijack: string[] | null → config.dnsHijack ?? []（SettingsTun:143）；GetUwpApps 的 | null → ?? []（SettingsUwp:107）；DNSConfig.fallbackFilter 在 Go 是**值类型**（config.go:313），JSON 永不缺席，所以模板里 dnsConfig.fallbackFilter.geoip 不会抛（任务书怀疑的空指针不成立）；nameserverPolicy: null 由 formatNameserverPolicy（Dns:267-270）与 || [] 各处置好。
8. **数据目录/诊断字段逐个核对无误**：GetDataDirInfo() 返回 sys.DataDirInfo，SettingsDiagnostics 用到的 coreReady/coreExists/coreError/wintun*/seedCoreReady…seedAsnReady/seedManifestOK/canAutoRepair*/legacyDataDir/legacyExists/coreBinDir/seedCoreBinDir **全部存在且拼写一致**（sys/models.ts:4-34）；GetDiagnosticInfo().logDir/appVersion/goos/goarch 一致（appcore/models.ts:297-323）；GetHelperServiceStatus() 的 installed/running/reachable/error 与 SettingsUpdate:25-26 一致（sys/models.ts:39-44）。
9. **SettingsDiagnostics 的重复触发防护到位**：inflight 变量闸门（:164-183）+ 刷新按钮 :disabled="loading" + 导出 exporting 闸门（:253-268）+ 剪贴板 try/catch（:245-250）+ 导出空路径静默（:258-259，与后端 SaveFileDialog 取消语义一致）。本页是我范围内**唯一**做全了 in-flight 互斥的页面。
10. **全局模态有队列，不会互相顶掉**：store.ts:623-703 的 enqueueModal 排队 + settleModal 保证 resolve + reconcileModalDisplacement 兜底结算。所以 SettingsUwp 的"提权确认 → 清空确认"、SettingsTun 的"驱动确认 → 重试"、"重置确认 → 失败 alert" 不会出现 Promise 悬挂/模态互顶（任务书怀疑的 modal 顶掉问题**已由历史修复解决**）。
11. **UWP 的"空列表≠全取消"判断成立**：uwpLoaded 闸门（:91、:114、:154）+ :disabled="savingUwp || !uwpLoaded"，枚举失败时不允许提交；GetUwpApps() 失败也不会把 uwpApps 当"用户清空全部"。
12. **ModernNumberInput 的 ± 按钮边界正确**：min !== undefined && modelValue <= min / max !== undefined && modelValue >= max（:6、:27）+ clamp 双向夹取，无 off-by-one。
13. **SettingsTheme 的定时器全部清理**：themePreviewTimer（120ms 合并）与 glassFrame（rAF）在 flushThemePreview、commitGlass、onBeforeUnmount（:655-666）三处都会 cancel；卸载时还会把最后一次预览应用到界面。
14. **两套日志等级语义各自匹配后端**：logLevel 用 warning（Mihomo 关键词，clash.NormalizeLogLevel 输出也是 warning，config.go:396-411）；appLogLevel 用 warn（logger.NormalizeLevel 输出 warn，level.go:5-18）。Settings.vue:174-176 的 warn→warning 映射只对内核日志生效，正确。
15. **ResetComponentSettings 的实际调用段名合法**：前端只会发出 tun/dns/network/behavior（grep "$emit('reset'"），全部命中 Go 的 4 个 case（app.go:559-586）；'lan'→'network' 的映射在 Settings.vue:233 存在但当前**永不生效**（因为 LAN 发的是 'network'，见 G-01）。
16. **v-for key 全部使用业务 id**（SettingsHome 的 e.id/f.id、SettingsUpdate 的 db.key、UpdateTaskPanel 的 task.key、SettingsUwp 的 app.sid、SettingsTheme 预设 preset.id），无 index key。
17. **SettingsTheme 的"主题真源"设计正确**：config 从 themeState.config 初始化（:350）而非 props.behavior 快照，并有 themeState 的 fingerprint watch（:614-626）回灌外部改动 —— 避免了"侧栏切明暗后被页面旧快照顶回去"（历史 P1 已修）。
18. **applyAnchor 的重试链自限**：最多 6 次 ×120ms，超时回顶部 + toast（Settings.vue:96-140），DOM 访问有 if (!el) 分支，不会无限轮询。
19. **validateHosts / validateCIDR 的行号提示参数正确**：t('settings.network.hostsLineError', {line})、t('settings.lan.cidrError', {line,text}) 的参数名与 8 语言包占位符一致。
20. **SettingsHome 的搜索实现**：labelOf 的 dbFile 参数化标题把 set-update-dbFileGeoip → dbNames['Geoip'] 正确映射；sectionLabel 避免组名与页名重复；结果按固定 order 分组，与 manifest 的 103 条一致。

---

## 6. 已知问题（历史已记录，不计入本轮编号）

| 旧 ID | 问题 | 当前状态 | 位置 |
|---|---|---|---|
| F-A4-06 / 表#15 | 设置页自动保存失败反馈四套不统一：TUN 有 5s 去重、DNS 每次失焦都弹模态、LAN/Network 曾完全静默 | **部分残留**：DNS 仍无去重（每次失败弹一个模态），与 SettingsTun 的 5s 去重不一致 | SettingsDns.vue:236-244 vs SettingsTun.vue:198-210 |
| F-A4-16 / 表#16 | 行为设置保存失败 → showAlert + loadBehavior()，等于**静默撤销用户改动** | 仍存在（属设计取舍：不撤销则界面与磁盘分叉） | Settings.vue:210-215 |
| F-A4-11 | .catch(()=>{}) 吞错 5 处（含 SettingsTun 装驱动、SettingsUpdate 的三个 Async 调用） | **大部分已修**（现为 reportError）；残留形态见本轮 G-21 | SettingsUpdate.vue:103/108 |
| F-A4-20 | UWP"全不选"按钮实现成取反 | 实现已改为置 false；**但文案仍是"反选"** → 以 G-03 重新登记（问题方向相反，不是重复） | SettingsUwp.vue:143-146 |
| F-A4-07 | 8 处自建模态与全局模态并存、重置两套确认 UI | **已修**（Settings.vue:219-229 已统一 showConfirm）；残留：SettingsTheme 的确认漏 isDanger → 并入 G-14 | SettingsTheme.vue:631-634 |
| F-A4-05 | mips→mixed 回退从未落盘、每次进页重复弹窗 | **已修**：saveTunCore(true) 绕过 loading 闸门并落盘 | SettingsTun.vue:160-165、192-193 |
| F-A4-09 | SettingsTun 首开并发 2×全量 GetStatus 且互相覆盖 | **已修**：只调一次 CheckTunEnv + applyTunStatus 归一化 | SettingsTun.vue:139、112-121 |
| F-A4-21 | UWP 提权确认只有"重启/取消"，取消后无法以普通权限查看 | **已修**：cancelText = 普通权限查看 | Settings.vue:73 |
| F-A4-17 | settings.uwp.clearAllConfirm 在 8 个语言包缺失、代码里手写兜底 | **已修**：键已补齐（zh-CN:758 / en-US:758），clearAllConfirmText() 无兜底层 | SettingsUwp.vue:150 |
| 历史 i18n 表 | 8 语言各 777 键、ja-JP 缺 14 处 | **已修**：各 796 键，本轮静态引用 0 缺失 | — |

---

## 7. 需要跨文件 / 后端确认的疑点（移交主控）

1. **geo-update-<key>-{start,success,error,cancelled,busy} 未注册事件**：v3_events.go:23-26 明确"动态事件名不注册（保持现有行为）"。若 Wails v3.1.0 的 Emit 对未注册事件名会 cancel/丢弃，则 SettingsUpdate 的"geo 更新成功后刷新版本/文件大小"（:108）和将来任何 geo 失败提示都会静默失效。需后端同学用 wails_event_sink.go 或实测确认。
2. **4 个玻璃强度字段是否可删**（G-23）：glassBlurStrength/ContentStrength/OverlayStrength/ControlStrength 已无前端消费者，但 Go 注释仍宣称"3 组独立半径"。请后端定夺：删字段 or 改注释。
3. **SaveUwpExemptions binding 是否已成死 API**：前端零调用（仅 manifest 文档字符串提到），实际已用 ApplyUwpExemptionChanges / ClearAllUwpExemptions。
4. **longConnectionMinSeconds 没有 UI 入口**（grep：仅出现在 Settings.vue:166 的兜底对象）—— 是"漏做高级设置"还是"有意固定 60"？后端 normalizeBehavior 会 <=0 → 60（behavior.go:518-520）。
5. **还原备份后是否需要前端全量刷新**（G-05）：建议后端在 RestoreBackup 成功后补发事件（core/appcore/events.go:13 已有 EventBehaviorChanged = "behavior-changed" 常量但无发射点），前端统一监听并 loadBehavior()。
6. **syncStartupTaskStateSafe 会直接落盘 startupWithOS**（controller.go:458-511）且不通知前端：若设置页此刻持有旧快照，下一次保存会把旧值写回并触发 handleStartupWithOSChange 重建计划任务（controller.go:2447-2455 有失败回滚）。需确认这是可接受的自愈行为，还是应补事件。
7. **LAN 非法 CIDR 是否真会让内核起不来**（G-02 的 B 级依据）：lan-allowed-ips 由 splitIPLines 原样注入（config.go:621-625），mihomo 解析 netip.Prefix 失败的表现需实测（配置校验拒绝 or 启动即退出）。
8. **dns-hijack: [] 的行为**：SettingsTun.updateTunDnsHijack（:213）允许清空成 [] 并保存；mihomo 对空 dns-hijack 是"不劫持"还是报错，需确认是否该加"至少一条"校验（当前无下限保护）。
9. **ClearFinishedUpdateTasks 的语义**：后端把 error/cancelled 也清掉（component_update_state.go:67-75），与本轮 G-10 的前端 3s 自动清理叠加后，用户可能来不及读错误。是否改为仅清 success。

---

## 附：本轮自评与降级记录

- 曾怀疑"GetDataDirInfo 字段名对不上" → 读 sys/models.ts 后确认**全部存在**，撤回。
- 曾怀疑"driver-install-success / core-update-* 是注册了但没人发的死事件" → 读 core/tasks/manager.go 后确认由任务框架动态发，撤回。
- 曾怀疑"加载失败 → 兜底值落盘 → 配置被冲掉"为 P1 → 读 core/utils/settings.go 确认 LoadSetting 永不返回 error，降级为 P2（G-06）。
- 曾怀疑"ModernSelect 每实例挂 document 监听 = 泄漏" → 读 onUnmounted 后撤回（§5-4）。
- 曾怀疑"托盘菜单也能改行为设置 = 多写者" → 读 tray_runtime_windows.go 后撤回（§5-5）。
- 曾怀疑"modal 队列会被顶掉导致 await 悬挂" → 读 store.ts:623-703 后撤回（§5-10）。
- 曾怀疑"UWP 全不选"是历史重复 → 历史上修的是**实现**，本轮是**文案与实现再次相反**，按新问题登记并注明旧 ID。
