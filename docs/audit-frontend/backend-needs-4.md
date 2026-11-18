# backend-needs-4 · fix-4-pages 需要后端配合的事项

> 提出方：fix-4-pages（Wave 1，未改任何 Go 代码）
> 处理方：fix-backend（Wave 2）
> 前端现状：所有需要后端配合的点都已按**双兼容**落地（新字段优先、旧形态回退），后端不动也不会崩，只是能力受限。

## 1. P-30 · `SaveConfigText` 需要结构化返回（优先级：高）

**现状**：`core/appcore/config_editor_controller.go:24-33`

```go
if err := clash.SaveConfigText(id, content); err != nil { return err }   // 文件已写盘
...
if err := c.controls.ReloadConfig(ctx, "config-save"); err != nil {
    return fmt.Errorf("保存成功，但重载运行配置失败: %w", err)          // ← 也是 error
}
```

前端只能拿到字符串 error，无法区分"没保存"和"存了但内核没换"。

**前端已做的双兼容**（`YamlEditor.vue:340-348`）：
```ts
const isSavedButReloadFailed = (e: any): boolean => {
  if (e && (e.saved === true || e.reloadFailed === true)) return true;   // 优先读结构化字段
  const msg = String((e && (e.message || e.error)) || e || '');
  return msg.includes('保存成功') && msg.includes('重载');                // 回退兼容现有文案
};
```

**期望后端改成**（任选其一，按 Wails v3 的可行性定）：
1. `SaveConfigText` 返回结构体 `{saved bool, reloadError string}`，前端把 `e.saved === true && e.reloadError !== ''` 当"已保存但重载失败"；
2. 继续返回 error，但挂上可序列化的附加信息（Wails 错误对象带 `meta`/`data` 字段），字段名沿用 `saved` / `reloadFailed`（前端已按这两个名字读）；
3. 最次：**保留现有中文文案**中的「保存成功」与「重载」两个词（前端靠它兜底，改文案会让该分支失效，退化成旧的"保存失败"提示——方向安全但误导用户）。

## 2. P-05 · 需要 `SaveSubOrder(ids []string)`（优先级：中）

**现状**：订阅排序前端只有内存数组，后端没有任何写顺序的接口（`coreservice` 里无 `*Order*`/`*Move*`）。

**前端已做**：`Subscriptions.vue:246-253` 用 `const SORT_MODE_AVAILABLE = false` 把排序入口 `v-if` 隐藏，`moveUp/moveDown/toggleSortMode` 与样式原样保留（一行未删）。

**期望后端**：
```go
// CoreService.SaveSubOrder 按传入 id 顺序重写 SubIndex 并 SaveIndex
func (a *App) SaveSubOrder(ids []string) error
```
- 语义：`ids` 必须是当前 SubIndex 的**完整 id 列表**（前端排序模式里就是全量列表）；长度或元素不匹配时返回可读错误，不要静默丢弃。
- 完成后前端需要把 `SORT_MODE_AVAILABLE` 改回 `true`，并在"完成"按钮里调用 `App.SaveSubOrder(localConfigs.map(c => c.id))`，同时把 `fetchConfigs` 在排序模式下暂停。

## 3. P-08 · 配置内容变更通知（优先级：低，可选）

**现状**：`config-changed` 全仓唯一 emit 点是保存"激活配置"的文本（`config_editor_controller.go:35`）。订阅更新（`clash.DownloadSub` 重写 `Subscriptions/<id>.yaml`）**不发任何事件**；规则写操作也不发。于是 Rules/Proxies 页没有"页内刷新"通道，只能靠重新挂载。

**前端已做**：Wave 1 按任务要求**不新造事件**，Rules 的陈旧性用"删除前重读当前段"（P-01）兜底；并删掉了 Subscriptions 里那条永不触发的 `config-changed` 监听。

**期望后端（若做）**：在 `SaveConfigText` / 订阅更新完成 / `AddRuleFromForm` / `DeleteRule` 之后统一 emit `config-content-changed`，payload `{ "id": "<configId>" }`（不区分页面）。前端消费侧改动很小：Rules → `if (payload.id === globalState.activeConfigId) loadRules()`；Proxies → `loadData()`。**不建议**直接复用 `config-changed`（那个事件还承载"激活配置已换"的语义，混用会让 Subscriptions 的同步逻辑重新变成必需）。

## 4. 已核对、无需后端改动的事实（供 fix-backend 免重复调研）

| 事项 | 结论 |
|---|---|
| `subs-update-cancelled` 事件 | 已存在：`core/tasks/manager.go:99` `m.emit(name + "-cancelled")`，前端 Wave 1 已监听（P-27），**无需后端改动** |
| `subs-update-*` 是否一定成对 | `UpdateAllSubsAsync` 走 `Tasks.Run`（非 RunIfIdle）⇒ 一定发 `-start`；结束发 success/error/cancelled 三者之一。前端因此不会"置位后永远收不到事件" |
| `UpdateSub` 语义 | `core/clash/sub.go` 在 id 为空时用 `time.Now().UnixMilli()` 生成 ⇒ **恒新建**、不覆盖同名。前端只做"判重提示"，未改语义（P-25） |
| `DeleteRule` 越界索引 | `app.go:1125/1152/1166` 越界/不匹配**静默返回 nil**，前端读不出"没删到"。P-01 已改为按文本定位规避；若后端愿意返回"未找到"错误，前端可进一步提示（非必需） |
