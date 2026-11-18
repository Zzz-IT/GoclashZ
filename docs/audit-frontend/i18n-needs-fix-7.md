# i18n 需求 · fix-7（工具层 utils/*）

> 来源条目：**U-03**（相对时间 / 错误兜底后缀硬编码中文）、**U-18**（剪贴板失败文案）
> 归属：Wave 2（`frontend/src/locales/*` 由 fix-i18n 统一补齐；本代理不动 locale 文件）
> 代码现状：已按「优先 t(key)、缺失回退内置中文」双兼容实现 —— `t()` 在缺键时原样返回 key
> （`locales/index.ts:64-67`），`utils/format.ts`、`utils/errors.ts`、`utils/clipboard.ts` 里的
> `tr()` 助手会据此回退旧中文。**因此这批 key 补齐前 UI 不会出现裸 key，补齐后自动切换语言。**

## 1 落点

8 个语言包的 `common` 段（`zh-CN.ts:2-28`，放在 `longConnectionDeferred` 之后即可）。
共 **7 个新 key**，插入位置一致、顺序建议如下。

## 2 key 与 8 语言文案

| key | zh-CN | zh-TW | zh-HK |
|---|---|---|---|
| `common.never` | 从未 | 從未 | 從未 |
| `common.justNow` | 刚刚 | 剛剛 | 剛剛 |
| `common.minutesAgo` | {n} 分钟前 | {n} 分鐘前 | {n} 分鐘前 |
| `common.hoursAgo` | {n} 小时前 | {n} 小時前 | {n} 小時前 |
| `common.daysAgo` | {n} 天前 | {n} 天前 | {n} 天前 |
| `common.errorLogged` | （已记录日志） | （已記錄日誌） | （已記錄日誌） |
| `common.copyFailed` | 复制失败，请手动复制 | 複製失敗，請手動複製 | 複製失敗，請手動複製 |

| key | en-US | ja-JP |
|---|---|---|
| `common.never` | Never | 未実施 |
| `common.justNow` | Just now | たった今 |
| `common.minutesAgo` | {n} min ago | {n} 分前 |
| `common.hoursAgo` | {n} h ago | {n} 時間前 |
| `common.daysAgo` | {n} d ago | {n} 日前 |
| `common.errorLogged` | (logged) | （ログに記録済み） |
| `common.copyFailed` | Copy failed, please copy manually | コピーに失敗しました。手動でコピーしてください |

| key | ru-RU | fr-FR | de-DE |
|---|---|---|---|
| `common.never` | Никогда | Jamais | Nie |
| `common.justNow` | только что | à l'instant | gerade eben |
| `common.minutesAgo` | {n} мин назад | il y a {n} min | vor {n} Min. |
| `common.hoursAgo` | {n} ч назад | il y a {n} h | vor {n} Std. |
| `common.daysAgo` | {n} дн назад | il y a {n} j | vor {n} Tagen |
| `common.errorLogged` | (записано в журнал) | (consigné dans le journal) | (protokolliert) |
| `common.copyFailed` | Не удалось скопировать, сделайте это вручную | Échec de la copie, veuillez copier manuellement | Kopieren fehlgeschlagen, bitte manuell kopieren |

> `common.copyFailed` 可直接复用现成的 `subs.copyFailed` 译文（8 个语言包 `:210` 已有），保持措辞一致。
> `{n}` 是 `t()` 的插值占位符（`locales/index.ts:70-74`），不要改成别的名字。

## 3 消费点（补完 key 后无需改代码）

| key | 消费位置 |
|---|---|
| `common.never` / `justNow` / `minutesAgo` / `hoursAgo` / `daysAgo` | `frontend/src/utils/format.ts:41-64`（`formatRelativeTime`，唯一调用方 `SettingsUpdate.vue` 的「上次检查」） |
| `common.errorLogged` | `frontend/src/utils/errors.ts:123-130`（`displayErrorMessage` 的兜底后缀，38 处调用点） |
| `common.copyFailed` | `frontend/src/utils/clipboard.ts`（`copyText` 失败 toast；调用方改造见 handoff-fix-7.md） |

## 4 附带建议（不算本代理需求）

`scripts/check-i18n.mjs` 的死键判定对 `format.ts` / `errors.ts` / `clipboard.ts` 的 `tr('common.xxx')`
是静态可识别的字面量，不会误报；这 7 个 key 补齐后即可通过校验。
