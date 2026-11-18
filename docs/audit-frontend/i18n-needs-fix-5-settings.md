# i18n 需求 · fix-5-settings（Settings 模块 / 更新任务）

> **本代理没有新增任何 key**：三处需要的文案改动全部落在**已有 key** 上，
> 所以 `scripts/check-i18n.mjs` 不会因本代理变红（当前 12 项缺键全部来自其它代理新增的 key）。
> Wave 2（fix-i18n）只需按下面第 1、2 节替换 8 个语言包里两行的文案，代码侧**零改动**。
>
> 关联：`fix-5-settings.md` G-03、G-05、G-11、G-22；`handoff-fix-5-settings.md` §2。

## 1 G-03：`settings.uwp.invertSelect` 文案要改成「全部不选」

**为什么**：按钮实际动作是「全部置为不豁免」，不是逐项取反（F-A4-20 修的是实现，文案没跟上）。
一次误点 + 应用 = 清空所有 UWP 环回豁免。audit G-03 的推荐方案就是改这行文案。
**位置**：`frontend/src/locales/<lang>.ts:745`（每个语言包都是第 745 行，`uwp` 段内）。

| 语言 | 现值 | 应改为 |
|---|---|---|
| zh-CN | 反选 | **全部不选** |
| zh-TW | 反選 | **全部不選** |
| zh-HK | 反選 | **全部唔選** |
| en-US | Invert | **Select None** |
| ja-JP | 選択を反転 | **すべて選択解除** |
| ru-RU | Инвертировать | **Снять все** |
| fr-FR | Inverser la sélection | **Tout désélectionner** |
| de-DE | Auswahl umkehren | **Alle abwählen** |

```ts
// zh-CN.ts:745
invertSelect: '全部不选',
```

> 若 Wave 2 想顺手把键名改成语义正确的 `selectNone`：**必须同时改**
> `SettingsUwp.vue:32`（`t('settings.uwp.invertSelect')` → `t('settings.uwp.selectNone')`）
> 和 8 个语言包的键名，否则 check-i18n 会报「代码使用但 zh-CN 未定义」。

## 2 G-05：`settings.about.restoreSuccess` 文案改成「已还原并重载」

**为什么**：旧文案「设置及配置已即时生效」在修复前是假话（前端快照没刷新，下一次保存还把还原结果覆盖回去）。
修复后真的会重载 behavior（Settings.vue 的 `@reload-behavior="loadBehavior"`），文案据实改写。
**位置**：`frontend/src/locales/<lang>.ts:492`（`settings.about` 段内）。

| 语言 | 现值 | 应改为 |
|---|---|---|
| zh-CN | 数据还原成功！设置及配置已即时生效。 | **数据还原成功，设置已重新载入。** |
| zh-TW | 資料還原成功！設定及配置已即時生效。 | **資料還原成功，設定已重新載入。** |
| zh-HK | 資料還原成功！設定及配置已即時生效。 | **資料還原成功，設定已重新載入。** |
| en-US | Data restored successfully! Settings applied immediately. | **Data restored. Settings have been reloaded.** |
| ja-JP | データの復元が完了しました！設定が即時反映されました。 | **データの復元が完了し、設定を再読み込みしました。** |
| ru-RU | Данные успешно восстановлены! Настройки применены. | **Данные восстановлены, настройки перезагружены.** |
| fr-FR | Données restaurées avec succès ! Paramètres appliqués immédiatement. | **Données restaurées. Les paramètres ont été rechargés.** |
| de-DE | Daten erfolgreich wiederhergestellt! Einstellungen sofort übernommen. | **Daten wiederhergestellt. Einstellungen neu geladen.** |

```ts
// zh-CN.ts:492
restoreSuccess: '数据还原成功，设置已重新载入。',
```

> 确认框里的 `restoreNotice`（「还原完成后，部分设置将即时生效。」）是"将要发生"的表述，重载后依然成立，**不用改**。

## 3 G-11（可选）：更新任务操作失败文案

现状：`UpdateTaskRow.vue:92` 用 `t('updateTask.statusFailed') + ': ' + 错误原文`（复用了"失败"这个 key，避免新增 key）。
如果 Wave 2 希望有独立文案，加下面这个 key 后**只需把那一行换成注释里写的调用**：

```ts
// 位置：updateTask 段，建议放在 statusWaiting 之后
updateTask: {
  ...
  statusWaiting: '等待中',
  actionFailed: '操作失败: {error}',   // ← 新增
},
```

| 语言 | 文案 |
|---|---|
| zh-CN | 操作失败: {error} |
| zh-TW | 操作失敗: {error} |
| zh-HK | 操作失敗: {error} |
| en-US | Action failed: {error} |
| ja-JP | 操作に失敗しました: {error} |
| ru-RU | Не удалось выполнить действие: {error} |
| fr-FR | Échec de l'opération : {error} |
| de-DE | Aktion fehlgeschlagen: {error} |

代码侧对应改法（1 行）：`UpdateTaskRow.vue:92`
```ts
showToast(t('updateTask.actionFailed', { error: displayErrorMessage(e) }), 'error');
```
**不急**：不加这个 key 也不会露 key 字面量，当前实现已经是本地化前缀 + 原始错误。

## 4 G-22（交 fix-7，`utils/format.ts` 不归本代理）：`formatRelativeTime` 硬编码中文

现状 `frontend/src/utils/format.ts:37-50` 直接返回 `'从未' / '刚刚' / 'N 分钟前' / 'N 小时前' / 'N 天前'`，
调用点 `SettingsUpdate.vue:32`（设置 → 组件与库 → GeoIP/GeoSite/MMDB/ASN 行的"文件大小 · 时间"）。
建议改成返回 key + 参数，或在 util 内 import `t`。需要的 key（建议放在 `time` 新段）：

```ts
time: {
  never: '从未',
  justNow: '刚刚',
  minutesAgo: '{n} 分钟前',
  hoursAgo: '{n} 小时前',
  daysAgo: '{n} 天前',
},
```

| 语言 | never | justNow | minutesAgo | hoursAgo | daysAgo |
|---|---|---|---|---|---|
| zh-CN | 从未 | 刚刚 | {n} 分钟前 | {n} 小时前 | {n} 天前 |
| zh-TW | 從未 | 剛剛 | {n} 分鐘前 | {n} 小時前 | {n} 天前 |
| zh-HK | 從未 | 啱啱 | {n} 分鐘前 | {n} 小時前 | {n} 天前 |
| en-US | Never | Just now | {n} min ago | {n} hr ago | {n} days ago |
| ja-JP | なし | たった今 | {n} 分前 | {n} 時間前 | {n} 日前 |
| ru-RU | Никогда | Только что | {n} мин назад | {n} ч назад | {n} дн назад |
| fr-FR | Jamais | À l'instant | il y a {n} min | il y a {n} h | il y a {n} j |
| de-DE | Nie | Gerade eben | vor {n} Min. | vor {n} Std. | vor {n} Tagen |

> 超长区间（>30 天）现在的实现是 `toLocaleDateString()`，自带本地化，不需要 key。
