# 后端需求 · fix-5-settings（设置模块 / 更新任务）

> 本 Wave 不改 Go。下面每一条都是「前端已经把可控的那一半做完」之后剩下的根治项，
> 按优先级排序；每条都给了现状证据（file:line）与前端侧的现状，便于后端判断是否本轮做。

## 1 【P1】SaveAppBehavior 是整份结构覆盖，多写者仍会互相覆盖（G-04 / P1-A）

**现状证据**
- `app.go:539` `func (a *App) SaveAppBehavior(config AppBehavior) error` → `core/appcore/controller.go:2405` → `BehaviorStore.SetAndSave(b)`（`core/appcore/behavior.go:121`）**整份替换**，不是字段合并。
- 同一份 `user_behavior.json` 的写者至少 4 个：设置页（`Settings.vue`）、侧栏明暗 `store.ts:134-145 setThemeMode`（GET→改 themeMode→SAVE 整份）、备份还原后端的 `Behavior.Load()`（`core/appcore/backup_service.go:81-90`）、启动项同步 `syncStartupTaskStateSafe`（`controller.go:458-511`）。
- Go 侧**已经有**原子读改接口：`BehaviorStore.Update(mutate func(b *AppBehavior)) error`（`core/appcore/behavior.go:143`），只是没暴露给前端。

**前端本轮已完成（Settings.vue:177-277）**
保存改为「GET 最新 → 只合并本地脏字段 → SAVE」，并加保存串行链；`setThemeMode` 的单字段 RMW 由 fix-1 负责（见 handoff）。两个前端写者并发时仍有理论窗口（GET 与 SAVE 之间）。

**建议后端做（二选一）**
1. 新增 `PatchAppBehavior(patch map[string]any) error`：只写传入字段（内部 `Behavior.Update`，逐字段赋值）；
2. 或把 `SaveAppBehavior` 改成补丁语义：先解到 `map[string]json.RawMessage`，再应用到 `Behavior.Get()`，只覆盖出现的键。

⚠️ 注意 `Controller.SaveAppBehavior`（`controller.go:2405-2453`）依赖 `old` vs `next` 的逐字段 diff 来触发副作用（自动测速、自动更新、日志流、系统代理口径、开机自启回滚、端口代理清零）。补丁语义必须**先把 patch 应用到 current 再算 next**，保留这套 diff 语义，否则开机自启的回滚（:2447-2453）会失效。

## 2 【P1】RestoreBackup 成功后补发 `behavior-changed` 事件（G-05）

**现状证据**：`core/appcore/backup_service.go:81-90` 还原后重新 `Behavior.Load()`；`core/appcore/events.go:13` 已经有常量 `EventBehaviorChanged = "behavior-changed"`，但**全仓没有任何发射点**。

**前端本轮已完成**：`SettingsAbout` 还原成功后 emit `reload-behavior` → `Settings.loadBehavior()`（含 `setLocale` + `syncThemeFromBehavior`）。这只能覆盖"从设置页内触发还原"这一条路径。

**建议后端做**：`RestoreBackup` 成功（且还原模式包含 settings/all）后 `events.Emit(EventBehaviorChanged, core.Behavior.Get())`。前端随即可以统一监听（并顺带覆盖托盘/命令行等其它入口），前端那半只需要加一个 `Events.On('behavior-changed', () => loadBehavior())`。同时建议 `SaveAppBehavior` 成功后也发一次（便于多窗口/多入口对齐，但要注意别让本页保存再触发重载，否则会打断正在编辑的输入框）。

## 3 【P2】ResetComponentSettings 没有 theme 分支（G-15）

**现状证据**：`app.go:560-586` 的 switch 只有 `behavior / network / dns / tun`，其它段落返回 `unknown settings section: %s`（:584-585）。前端 `SettingsTheme` 的重置只在前端完成（改 themeState + behavior 的玻璃/colorDelay 后整份保存）。

**前端本轮已完成**：删掉 theme 重置的死链路（`emit('reset','theme')`、`resetVersions.theme`、`modules.theme`），不存在"看起来能重置但 500"的中间态。

**建议后端做（可选）**：若要统一路径，加
```go
case "theme":
    cur := a.core.Behavior.Get()
    def := a.core.Behavior.Default()
    cur.ThemeMode, cur.ThemePreset, cur.ThemeAccent, cur.ThemeRadius, cur.ThemeAura = def.ThemeMode, def.ThemePreset, def.ThemeAccent, def.ThemeRadius, def.ThemeAura
    cur.GlassBlurEnabled, cur.GlassTintStrength, cur.GlassDepth = def.GlassBlurEnabled, def.GlassTintStrength, def.GlassDepth
    cur.ColorDelay = def.ColorDelay
    return a.SaveAppBehavior(cur)
```
（`ColorDelay` 是否属"主题"要产品定：前端本轮的"恢复默认"已经把它一起重置，见 G-14。）

## 4 【P2】4 个玻璃强度字段是否删除（G-23）

**现状**：`AppBehavior` 里 `GlassBlurStrength`（legacy）/ `GlassBlurContentStrength` / `GlassBlurOverlayStrength` / `GlassBlurControlStrength`（`core/appcore/behavior.go:71-75`）后端只做持久化与 normalize（:531-534），前端**零消费者**（已 grep 全前端，唯一引用点是 `Settings.vue` 的兜底对象，本轮已删）。UI 早已收敛成"厚度总控 `glassDepth` + 染色 `glassTintStrength`"（`SettingsTheme.vue:184-186` 注释说明了为什么不分级）。

**矛盾点**：`behavior.go:68-70` 的注释仍宣称「玻璃层按 3 组独立半径…任一组独立生效，互不换算」，与前端实现（单条总控）矛盾；`frontend/bindings/goclashz/core/appcore/models.ts:108-113` 是同一段注释的镜像。

**建议后端做（三选一）**：① 从结构体移除（需迁移/兼容旧 json，且 `migrateGlassStrengths` 要保留一个版本）；② 保留字段但改注释为「历史字段，仅供迁移，运行时不再消费」；③ 保留并把 models.ts 的注释同步改掉。请给结论，Wave 2 可顺手改注释。

## 5 【P2】ClearFinishedUpdateTasks 会把 error/cancelled 一起删（G-10）

**现状证据**：`core/appcore/component_update_state.go:67-85` `ClearFinished()` 对 `success|error|cancelled` 一律删除，然后广播 `component-update-tasks-sync`。用户还没来得及读失败原因，3 秒后错误行就消失。

**前端本轮已完成**：`UpdateTaskPanel` 不再调用它，改成对 success 任务逐个 `RemoveUpdateTask`（`Remove` 同样会广播 sync，见 `component_update_state.go:87-102`）。

**建议后端做**：把 `ClearFinished` 收窄为只清 success，或加参数 `ClearFinished(includeFailed bool)`。同时确认托盘/其它调用方（若有）是否依赖"一次全清"。

## 6 【P2】hosts / lan-* 的非法值在后端被静默处理（G-02 / G-17）

**现状证据**
- hosts：`core/clash/config.go:493-499` `yaml.Unmarshal` 失败就**静默跳过**，运行配置里干脆没有 hosts 段，用户侧表现为"保存成功但映射不生效"。
- LAN 白名单：`core/clash/config.go:701-713` `splitIPLines` 只过滤空行与 `#` 注释，**不做任何合法性校验**，直接注入 `lan-allowed-ips`（:621-625）。非法条目（如 `192.168.1.0/33`）会让 mihomo 解析 `netip.Prefix` 失败。

**前端本轮已完成**：`SettingsNetwork.saveNet` 与 `SettingsLan.save` 落盘前都判校验结果，不通就不保存并 toast（`SettingsNetwork.vue:343-354`、`SettingsLan.vue:251-268`）。但前端正则是宽松的基础校验（例如仍接受 `999.999.999.999`），挡不住所有非法值。

**建议后端做**：`BuildRuntimeConfig` 里对 hosts（YAML 解析失败）与 lan-*/CIDR（`netip.ParsePrefix` 失败）做一次真实校验，失败时 ① 不要静默；② 把原因暴露成 deviation（`DeviationStrip` 已有 module 机制，前端已按 module 轮询）或放进内核启动错误文案，替代"内核起不来但不知道为什么"。

## 7 【P3】SaveUwpExemptions 是否已成死 API（G-16）

**现状**：前端零调用（`grep SaveUwpExemptions frontend/src` 只剩 manifest 的文档字符串，本轮已改）；实际走 `ApplyUwpExemptionChanges(added, removed)`（差异提交）与 `ClearAllUwpExemptions()`（目标为空时）。binding 仍在 `frontend/bindings/goclashz/appservice.ts:141`。请确认删除或标注 `Deprecated`，Wave 2 可同步 regen bindings。

## 8 不需要后端配合的条目（记录，避免误提需求）

- **G-06 / U-20**：前端兜底对象与 `Default()` 对齐即可，本轮已完成；`utils.LoadSetting` 的错误契约（永不返回 error）不变，兜底值仍属死代码。
- **G-22**：`formatRelativeTime` 硬编码中文属纯前端（`utils/format.ts`，fix-7 范围）+ 语言包补 key，不涉及 Go。
