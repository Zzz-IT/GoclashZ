# handoff-4 · fix-4-pages 移交给主控的跨文件事项

> 提出方：fix-4-pages　接收方：主控 lead（分发到 fix-2 / fix-3 / 指派者）
> 原则：只做本文件范围内能做的那一半，另一半写在这里，不碰别人的独占文件。

## 1. P-07 的另一半：App.vue 的离开守卫（归 fix-2）

**现状**（我未改动 App.vue）：
- `App.vue:646-648` `closeYamlEditor = () => { currentTab.value = 'subs' }` —— 无任何未保存检查。
- `App.vue:650-659` `watch(currentTab, ...)` 只在离开 yaml-editor 时**清状态**，不询问。

**我已经做完的**：`YamlEditor.vue:5-6, 405-413` 的返回箭头改走 `handleBack`，isModified 时 `showConfirm(t('yamlEditor.backConfirmMsg'), t('common.notice'))`，确认后 `emit('back')`。`isModified` 通过 `status-change` 上报到 `yamlEditorModified`，值可靠（改/存/重载/保存失败四条路径都会及时 emit）。

**fix-2 需要注意的三个约束（否则会出真 bug）**：

1. **不要在 `closeYamlEditor` 里再弹一次确认** —— 返回箭头已经弹过了，会连弹两次。`closeYamlEditor` 只负责把它标记为"已确认离开"再切 tab。
2. **侧边栏/托盘切页的拦截不能在 `watch(currentTab)` 里直接 return** —— watch 触发时 tab 已经切走了。必须先回退 tab、记住用户想去的那个 tab，确认后再切过去。示例骨架：

```ts
let yamlLeaveConfirmed = false;          // 编辑器自己已经确认过（handleBack 路径）
let pendingTab: TabId | null = null;     // 被拦下的目标页

const closeYamlEditor = () => { yamlLeaveConfirmed = true; currentTab.value = 'subs'; };

watch(currentTab, (newVal, oldVal) => {
  if (oldVal === 'yaml-editor' && newVal !== 'yaml-editor' && !yamlLeaveConfirmed && yamlEditorModified.value) {
    pendingTab = newVal;
    currentTab.value = 'yaml-editor';    // 先回退
    void showConfirm(t('yamlEditor.backConfirmMsg'), t('common.notice')).then(ok => {
      if (!ok) { pendingTab = null; return; }
      yamlLeaveConfirmed = true;
      currentTab.value = pendingTab!;
      pendingTab = null;
    });
    return;
  }
  yamlLeaveConfirmed = false;
  // ……原有的清理逻辑（editingConfigId / yamlEditorStatus / modified / error / cursor 复位）
});
```

3. **回退时不要清 `editingConfigId`** —— 现有 watch 里 `if (newVal !== 'yaml-editor')` 会清空 editing* 与 yaml 状态；被拦下回退的那一次也会先触发清理。建议把"清理"挪到"确认真正离开之后"执行，或清理前先判断 `pendingTab`。

> 如果 fix-2 决定改用统一拦截（H-* 条目里可能有同名守卫），把我这份约束当作"编辑器侧已完成、勿重复确认"的输入即可。

## 2. P-08 的另一半：Proxies.vue 的死监听（归 fix-3）

`Proxies.vue:765-767` 的 `config-changed` 监听与本页同样不可达（唯一 emit 点在 `config_editor_controller.go:35`，触发它的 YamlEditor 与 Proxies 页面互斥挂载）。我在 Subscriptions.vue 里删了同款监听（`Subscriptions.vue:665-668` / `693-704`），Proxies.vue 归 fix-3 处理：要么删，要么等后端新增"配置内容已变更"事件后改造（见 backend-needs-4.md 第 3 条）。

## 3. P-13 的漏网项：Rules.vue 的 `onActivated`（需指派）

`Rules.vue:568-572` 有一个 `onActivated(() => { if (isRemote.value) resetRuleSlider() })`。本应用**没有 KeepAlive**（App.vue 全是 v-if），这个钩子永不执行（P-13 已列为死钩子）。

- P-13 归 fix-3，但 fix-3 的独占文件只有 Proxies.vue / Connections.vue，**不包含 Rules.vue**；
- P-13 也不在我的条目表里（我的条目是 P-01/05/06/07/08/22/23/24/25/26/27/28/29/30/31/32/33/34）；
- 按"每处改动都要能对应到一条 ID"的规矩，我**没有动它**。

请主控指派：由我下一轮删（一行 + 去掉 import），或直接并入 fix-3 的 P-13 批次（需要临时授权改 Rules.vue）。

## 4. 与我这次改动相关、但不是我的条目的观察（未处理）

| 位置 | 现象 | 建议 |
|---|---|---|
| `Rules.vue:331-341` / `343-354` | `searchTimer` 与 `loadRules` 无序号：连续切段/搜索时旧的 GetRulePageData 仍可能后到并覆盖（与 P-26 同型） | 可作为 P-26 的扩展条目，Wave 2 统一做 |
| `Subscriptions.vue:532-554` | `confirmDelete` 成功后仍然 `fetchConfigs()`（P-28 修完后这一次是"校正"而非"救命"） | 保留，无需改 |
| `YamlEditor.vue:274-290` | P-32 的 180ms 常量可以考虑抽到常量区，Wave 3 重构时统一 | 现在留在原地更易读 |
