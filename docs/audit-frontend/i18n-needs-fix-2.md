# i18n-needs（fix-2-shell / App.vue）

> 代码里已直接使用 `t('新key')`，Wave 2 的 fix-i18n 补齐即可。
> **在补齐前 `node scripts/check-i18n.mjs` 会报 3 条 `代码使用但 zh-CN 未定义`**（此外不会报「某语言缺 key」，因为基准包也没有这几个键）。
> 另有 1 处刻意复用既有键，不需要新增：App.vue 的「更新任务进行中」提示用了 `settings.update.appUpdateBusy`（8 语言都已存在）。

## 1. 新增 key（3 个 × 8 语言）

### logs.std
用途：日志页来源筛选的第 4 个 tab（后端 LogSource = std，标准库 log 收编的内核输出）。App.vue:127

| 语言 | 文案 |
|---|---|
| zh-CN | 标准库 |
| zh-TW | 標準庫 |
| zh-HK | 標準庫 |
| en-US | Standard |
| ja-JP | 標準ライブラリ |
| ru-RU | Стандартный лог |
| fr-FR | Bibliothèque standard |
| de-DE | Standardbibliothek |

### logs.levelFilter
用途：日志级别下拉的 aria-label（原误用 logs.all）。App.vue:137

| 语言 | 文案 |
|---|---|
| zh-CN | 日志级别 |
| zh-TW | 日誌級別 |
| zh-HK | 日誌級別 |
| en-US | Log level |
| ja-JP | ログレベル |
| ru-RU | Уровень журнала |
| fr-FR | Niveau de journal |
| de-DE | Protokollstufe |

### yamlEditor.leaveConfirmMsg
用途：YAML 编辑器有未保存改动时，离开（侧栏切页 / 更新 chip 跳转）的确认框正文。标题复用 `yamlEditor.discardConfirmTitle`、按钮复用 `yamlEditor.discardBtn` 与 `common.cancel`。App.vue:747-770

| 语言 | 文案 |
|---|---|
| zh-CN | 当前有未保存的修改，离开将丢失这些修改。确定要离开吗？ |
| zh-TW | 目前有未儲存的修改，離開將遺失這些修改。確定要離開嗎？ |
| zh-HK | 目前有未儲存的修改，離開將遺失這些修改。確定要離開嗎？ |
| en-US | There are unsaved changes. Leaving will discard them. Leave anyway? |
| ja-JP | 保存されていない変更があります。離れると破棄されます。離れますか？ |
| ru-RU | Есть несохранённые изменения. При уходе они будут потеряны. Всё равно уйти? |
| fr-FR | Des modifications ne sont pas enregistrées. Quitter les perdra. Quitter quand même ? |
| de-DE | Es gibt ungespeicherte Änderungen. Beim Verlassen gehen sie verloren. Trotzdem verlassen? |

## 2. 说明

- 3 个 key 都只插在对应语言包的既有同名分组里（`logs:` 与 `yamlEditor:`），不需要新分组。
- 未改动任何 locales 文件；Wave 2 补齐后这 3 条 check-i18n 错误即消失。
