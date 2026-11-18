# 修复报告 · fix-5-settings（设置模块 / 通用控件 / 更新任务）

> 修复代理：fix-5-settings（8 人修复团队成员，主控 = lead）
> 日期：2026-11-18
> 独占文件：`frontend/src/components/Settings*.vue`（12 个，任务书说 13 个，实际 glob 只有 12）、`frontend/src/settings/manifest.ts`、`ModernSelect.vue`、`ModernNumberInput.vue`、`UpdateTaskPanel.vue`、`UpdateTaskRow.vue`
> 证据来源：`04-settings.md`（G-01~G-24）、`00-SUMMARY.md` §3.4（U-20）
> 约束遵守：只改独占文件；未运行 pnpm build / vite build / vue-tsc / 应用本体；未改后端 Go；未改 `frontend/src/locales/*`；未新增 i18n key（见 §3-1）。

## 1 逐条状态

| ID | 状态 | 改动位置(file:line) | 一句话说明 |
|---|---|---|---|
| G-01 | fixed | SettingsLan.vue:11、SettingsLan.vue:185 | 子页 emit 的段名从 `'network'` 改成 `'lan'`（含 defineEmits 类型）。Settings.vue:297-298 的 `'lan'→'network'` 映射本来就在，改完即：确认框显示「局域网」、`resetVersions.lan` 正常自增、LAN 页重载。 |
| G-02 | fixed | SettingsLan.vue:251-268（save 内整体校验）、SettingsLan.vue:97/127/154（`@blur="save"`） | 校验不再被丢弃：`save()` 落盘前把三条 CIDR 列表整体复校，不通就 return（红字 + toast），**不落盘**。`saveAndValidate` 与未使用的 `validateAll` 一并删除——校验收进 `save()` 才能覆盖「填了非法行再点任意开关」这条绕过路径。 |
| G-03 | partial | SettingsUwp.vue:26-33、SettingsUwp.vue:146-151 | 语义按 audit 推荐定为「全部不选」（不是逐项取反）。**文案键沿用 `settings.uwp.invertSelect`**，8 语言包文案改动写入 `i18n-needs-fix-5-settings`（Wave 2 落 locales）——不新增 key 是为了不让验收闸门 #2（check-i18n）先变红，见 §3-1。 |
| G-04 | fixed | Settings.vue:177-200（`loadedSnapshot` + `sameFieldValue` + `pickDirtyFields`）、Settings.vue:236-277（`saveBehavior` 串行链 + `runSaveBehavior`）、Settings.vue:217（load 后建基线） | 改为 **GET 最新 behavior → 只合并本地脏字段 → SAVE**；成功后基线前进到「本次提交的那份」。另加保存串行链，连点两个开关时后一次必须在前一次落盘后再读基线。失败路径不变（reportError + showAlert + loadBehavior）。与 fix-1 的 setThemeMode 单字段 RMW 配合，见 handoff。 |
| G-05 | fixed | SettingsAbout.vue:220-226（emit 类型）、SettingsAbout.vue:372-384（成功分支）；Settings.vue:13（`@reload-behavior="loadBehavior"`） | 还原成功后 emit `reload-behavior` → 父级 `loadBehavior()`（内含 setLocale + syncThemeFromBehavior），前端快照与后端一致，下一次保存不会把还原结果覆盖回旧值。提示文案改「已还原并重载」→ i18n-needs（沿用 `settings.about.restoreSuccess`）。 |
| G-06 | fixed | Settings.vue:168-173（兜底 behavior）；SettingsNetwork.vue:270-291（兜底 netConfig） | behavior 兜底与 Go `Default()` 对齐：`logLevel:'error'`、`delayRetention:true`、`updateInterval:1`，补齐 `portProxyMode`/`lastUpdateCheck`/`autoDelayTest`/`autoDelayTestInterval`/`activeMode:'rule'`/`subUA:'clash-verge'`/四条下载链接（原为空串）。Network 兜底补 5 个 LAN 字段、`tcpKeepAliveInterval` 15→30。 |
| G-07 | fixed | ModernNumberInput.vue:52-124 | `step` 改 computed；失焦提交统一走 `commit()`：clamp 到 [min,max]，NaN 先回落 min、无 min 再回落上一次合法值（不再回落 0）；补 Enter（=失焦提交）与 Esc（还原且不发 change）。提交后把输入框显示规范回合法值。 |
| G-08 | fixed | ModernSelect.vue:54-64 | 挂载期自动纠错只 `emit('update:modelValue')`，**不再 emit change**：父级普遍把 @change 当「用户改了→立刻落盘」，否则后端一个不在选项表里的值会让页面一打开就静默写盘。change 只在 `selectOption()`（用户点选）发出。 |
| G-09 | fixed | ModernSelect.vue:210-212、ModernSelect.vue:267-269、ModernSelect.vue:275-278（删除） | 删掉 Teleport 之后永不生效的三条规则：`.modern-custom-select.w-full .select-dropdown`、`.select-dropdown.drop-up`、`.select-dropdown` 里的 `z-index:100`（都被 `updatePosition()` 的内联样式覆盖）。用注释标明位置/尺寸由 updatePosition 接管。 |
| G-10 | fixed | UpdateTaskPanel.vue:31-42（只删 success）、UpdateTaskPanel.vue:44-68（单 timer + 卸载清理） | ① 自动清理改为逐个 `RemoveUpdateTask`，只动 success，不再用后台的 `ClearFinishedUpdateTasks`（它会把 error/cancelled 一起删）；② 定时器先 `clearTimeout` 再重建，全页恒为一个，`onUnmounted` 清理。 |
| G-11 | fixed | UpdateTaskRow.vue:82-108 | 4 个更新 RPC 全部收进 `runTaskAction`：catch + `reportError` + `showToast`。失败文案复用 `updateTask.statusFailed`（不新增 key，见 §3-1）。 |
| G-12 | fixed | SettingsBehavior.vue:285-300 | 新增「同步中」闸门：watch `props.behavior` 身份变化（父级 loadBehavior 换对象）时置位，`nextTick` 复位；`updateInterval` 的 watch 命中闸门直接 return，不再在打开页面时弹窗 + 写盘。 |
| G-13 | fixed | Settings.vue:248（saveBehavior 闸门）；SettingsBehavior.vue 16 处 `:disabled="loading"`；SettingsTheme.vue 12 处；SettingsAbout.vue 4 处（语言/自动更新/方式/间隔） | 三页控件在 loading 期间禁用（与 DNS/Network/Tun 一致），并在 `runSaveBehavior` 入口兜 `if (behaviorLoading.value) return`，杜绝「兜底值被整份写盘」。 |
| G-14 | fixed | SettingsTheme.vue:644-660 | `showConfirm` 补 `isDanger=true`（与 Settings.confirmReset 一致）；「恢复默认」补 `behavior.colorDelay = false`（Go 默认 false），不再漏项。 |
| G-15 | fixed | SettingsTheme.vue:314-324（props/emit）、SettingsTheme.vue:623（watch）、Settings.vue:11、Settings.vue:157-167 | 删掉 theme 重置死链路：SettingsTheme 的 `reset` emit 与 `resetKey` prop、Settings.vue 的 `resetVersions.theme`/`modules.theme`/`:reset-key`/`@reset`。若希望主题也能走后端统一重置 → backend-needs（Go 侧补 `case "theme"`）。 |
| G-16 | fixed | manifest.ts:486、manifest.ts:998、manifest.ts:1008 | 三处注释按修好后的实现改写：LAN「保存前整体复校、不通不落盘」；UWP「全选/全部不选只作用于当前搜索结果」；applyBtn「差异提交 ApplyUwpExemptionChanges / 空集走 ClearAllUwpExemptions，前端已无 SaveUwpExemptions 调用点」。 |
| G-17 | fixed | SettingsNetwork.vue:343-354 | `saveNet` 落盘前判 `validateHosts()`，不通就 toast 并 return（后端对解析失败的 hosts 是静默忽略，等于「保存成功但不生效」）。 |
| G-18 | fixed | SettingsUwp.vue:146-151 | 批量操作改作用于 `filteredUwpApps`：搜索时看到的项 = 被改的项，不再出现「看到 3 项、改了 200 项」。 |
| G-19 | fixed | SettingsAbout.vue:48-55（`v-model` + `@change`）、SettingsAbout.vue:234-241 | 语言选择不再先 `setLocale` 再保存：只写 behavior + emit('save')，由 Settings.saveBehavior **保存成功后**统一 `setLocale`；失败时它的 loadBehavior 会把界面语言与选项一起退回后端值。 |
| G-20 | fixed | SettingsAbout.vue:311-325（导出）、SettingsAbout.vue:371-385（还原）、SettingsAbout.vue:271-274（openLink）、SettingsAbout.vue:194（`|| '处理中...'`）、SettingsAbout.vue:358-367（两处 `||` 兜底） | ① SUCCESS / CANCELLED / 其它 三分支穷举，其它返回值不再静默；② `Browser.OpenURL(...).catch(reportError)`；③ 删掉 5 处 `t(...) || '中文兜底'` 死代码（t 缺键返回 key 路径，`||` 分支永不执行）。 |
| G-21 | fixed | SettingsUpdate.vue:76-94 | `refreshRuntimeAssets` 包 try/catch（5 个事件回调以 `void` 调它，原来 reject 未处理），与 SettingsTun 同名函数对齐。**附带项（pendingCoreUpdate 语义）本轮未动**，见 §3-4。 |
| G-22 | partial | 调用点未改（SettingsUpdate.vue:32）；需求写入 i18n-needs + handoff | `formatRelativeTime` 在 `utils/format.ts`（fix-7 独占），本代理不改。已给出 5 个 key 的 8 语言文案与调用点改法，交 fix-7 / 主控。 |
| G-23 | fixed | Settings.vue:173（删 4 个字段）；说明见 backend-needs | 前端唯一的引用点（兜底 behavior 对象）删掉 `glassBlurStrength` / `Content` / `Overlay` / `Control` 四个字段；字段本身保留在后端不动。 |
| G-24 | skipped | — | 12 个设置页重复的页面骨架/样式（约 500 行）按 wave 计划留给 Wave 3 的 R-B（设计系统统一），本 Wave 不动。 |
| U-20 | fixed | 同 G-06 | 兜底 behavior 与 Go `Default()` 逐字段对齐（`00-SUMMARY` §3.4「缺 4 个字段 + 3 处值不一致」）。 |

状态统计：**fixed 22 / partial 2（G-03 文案待 locales、G-22 跨文件）/ skipped 1（G-24）/ 驳回 0**，共 25 条（G-01~G-24 + U-20）。

## 2 关键取舍（给复核用）

1. **不新增任何 i18n key**：G-03 / G-05 / G-11 三处需要新文案的地方，一律**复用现有 key + 把文案改动写进 `i18n-needs-fix-5-settings`**（G-03 的 `invertSelect` 文案改动就是 audit G-03 修复建议里的原方案）。理由：`scripts/check-i18n.mjs` 对「代码用了但 zh-CN 未定义」是 error + exit 1（check-i18n.mjs:90、:101-106），在 Wave 2 补语言包之前新增 key 会让验收闸门 #2 直接失败。当前本代理的 13 个文件对 check-i18n 零贡献（错误全部来自其它代理新增的 key）。
2. **G-02 的校验放在 `save()` 而不是 `saveAndValidate()`**：原路径外还有 `allowLan` / 用户名 / 密码的 `@change`/`@blur` 也调 `save()`，只在文本框失焦时校验等于留了后门（先填非法行、再点开关）。代价是「有非法行时整页保存都会被挡」，见回归风险 2。
3. **G-04 的基线取「本次提交前的本地快照」而不是「保存后的 behavior.value」**：两者只在「Get/SAVE 往返期间用户又改了字段」时有差别——取前者，那些改动仍算脏、由链上的下一次保存补上；取后者会被基线前进吞掉（静默丢改动）。
4. **G-21 只修 catch，不动 pendingCoreUpdate**：`cancelCoreUpdateConfirm` 直接写 `globalState.componentUpdate.pendingCoreUpdate = false` 属跨文件语义问题（store 侧还有 `core-update-*` 事件的回收路径），改动会牵到 fix-1 的文件，且现有事件链下未观察到卡死。留给主控决定是否本轮修（见 handoff §4）。
5. **G-13 的禁用只加在三个"behavior 编辑页"**：SettingsDiagnostics / SettingsUpdate / SettingsUwp 本来就有各自的 in-flight 闸门（audit §5-9、§5-11），不重复加。

## 3 自查（未跑构建，用只读手段代替）

| 项 | 方法 | 结果 |
|---|---|---|
| 语法 | typescript `createSourceFile` 解析 12 个 .vue 的 script 块 + manifest.ts | 0 语法错误 |
| manifest 锚点 | `node scripts/validate-settings-manifest.mjs` | 103 项全部就位（EXIT 0） |
| i18n | `node scripts/check-i18n.mjs` | 本代理文件 0 缺键（现存 12 项缺失全部来自其它代理新增 key，非本代理引入） |
| 死符号 | grep `saveAndValidate`/`validateAll`/`resetVersions.theme`/`selectNone`/`actionFailed`/`restoreSuccessReloaded` | 仅剩解释性注释，无代码引用 |
| 临时文件 | 复查 git status | 只改了独占的 13 个文件，无临时/备份文件 |

## 4 回归风险（3 条）

1. **G-04 改了 SaveAppBehavior 的 payload 形状**：现在发的是「后端最新值 + 本地脏字段」。只要有一条路径依赖「整份写回把别的字段纠正回去」，行为就变了。验收请走：进设置页改任意一个开关 → 侧栏点明暗 → 回设置页再改一个开关 → 重启应用，确认三个改动都在且主题没有回退。若发现某字段改不动，先看 `loadedSnapshot` 是否因 load 失败为 null（此时退回整份写，等同旧行为）。
2. **G-02 / G-17 会让「保存被拒绝」**：LAN 三条 CIDR 或 hosts 里有非法行时，该页任何保存动作（包括不相关的开关）都会被挡住并 toast 一次。用户可能觉得「开关点不动」。验收时故意填一个非法 IP/CIDR 与一行坏 hosts，确认提示出现且不落盘（改回合法值后立刻恢复正常）。
3. **G-08 去掉自动纠错时的 `emit('change')` 后，选项表外的历史值不再被自动落盘纠正**：界面会显示第 1 个选项的文案，但 `behavior` 里的真实值仍是旧值（例如手改过 `user_behavior.json` 把 `appLogLevel` 写成 `warning`），直到用户手动选一次才会改。这正是本次要的行为（不再静默改配置），但验收时若看到"下拉显示 info、后端是 warning"不要当成新 bug；`logLevel` 不受影响（loadBehavior 已把 `warn` 归一成 `warning`，在选项表内）。

## 5 补充：本代理未做但相关

- SettingsDns 的「保存失败每次失焦都弹模态、无 5s 去重」（audit §6 残留 F-A4-06）不在 G 编号内，未动。
- SettingsHome / SettingsDiagnostics 无 G 条目，未动。
