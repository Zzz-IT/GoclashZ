# i18n-needs-4 · fix-4-pages 需要的新文案

> 提出方：fix-4-pages　处理方：fix-i18n（Wave 2）
> 说明：Wave 1 代码里已直接使用下列 key（`t()` 的形参是 `path: string`，不做类型校验；缺 key 时运行时会原样返回 key 本身，所以**必须**在 Wave 2 补齐，否则用户会看到 `rules.ruleChanged` 这种原文）。
> 插入位置：`zh-CN.ts` 等 8 个语言包的对应顶层段内（`rules` / `subs` / `yamlEditor`），键顺序建议与下表一致，保持 8 语言同构。

## 1. 新增 key（6 个 × 8 语言）

### `rules.configSwitched`（toast，info）
| 语言 | 文案 |
|---|---|
| zh-CN | 激活配置已切换，本次操作已取消 |
| zh-TW | 使用中的設定已切換，本次操作已取消 |
| zh-HK | 使用中的設定已切換，今次操作已取消 |
| en-US | The active configuration changed, this action was cancelled |
| ja-JP | 使用中の設定が切り替わったため、この操作は中止しました |
| ru-RU | Активная конфигурация изменилась, действие отменено |
| fr-FR | La configuration active a changé, l'action a été annulée |
| de-DE | Die aktive Konfiguration wurde gewechselt, die Aktion wurde abgebrochen |

### `rules.ruleChanged`（toast，info）
| 语言 | 文案 |
|---|---|
| zh-CN | 规则列表已变化，本次操作已取消，请重试 |
| zh-TW | 規則清單已變更，本次操作已取消，請重試 |
| zh-HK | 規則清單已變更，今次操作已取消，請再試一次 |
| en-US | The rule list has changed, this action was cancelled. Please try again |
| ja-JP | ルール一覧が変更されたため、この操作は中止しました。もう一度お試しください |
| ru-RU | Список правил изменился, действие отменено. Повторите попытку |
| fr-FR | La liste des règles a changé, l'action a été annulée. Veuillez réessayer |
| de-DE | Die Regelliste hat sich geändert, die Aktion wurde abgebrochen. Bitte erneut versuchen |

### `subs.urlInvalid`（alert 正文，含 no {error} 插值）
| 语言 | 文案 |
|---|---|
| zh-CN | 订阅链接格式不正确，需要以 http:// 或 https:// 开头 |
| zh-TW | 訂閱連結格式不正確，需以 http:// 或 https:// 開頭 |
| zh-HK | 訂閱連結格式不正確，需以 http:// 或 https:// 開頭 |
| en-US | Invalid subscription URL, it must start with http:// or https:// |
| ja-JP | 購読リンクの形式が正しくありません。http:// または https:// で始まる必要があります |
| ru-RU | Неверный формат ссылки подписки, адрес должен начинаться с http:// или https:// |
| fr-FR | Lien d'abonnement invalide, il doit commencer par http:// ou https:// |
| de-DE | Ungültige Abo-URL, sie muss mit http:// oder https:// beginnen |

### `subs.dupSubWarn`（命名弹窗内的警示行，前接 `common.warning` + ": "）
| 语言 | 文案 |
|---|---|
| zh-CN | 已存在同名或相同链接的配置；继续导入会新增第二条订阅（不会覆盖原配置）。 |
| zh-TW | 已存在同名或相同連結的設定；繼續匯入會新增第二筆訂閱（不會覆蓋原設定）。 |
| zh-HK | 已存在同名或相同連結的設定；繼續匯入會新增第二筆訂閱（不會覆蓋原設定）。 |
| en-US | A configuration with the same name or link already exists. Continuing adds a second subscription (the existing one is not overwritten). |
| ja-JP | 同じ名前または同じリンクの設定が既に存在します。続行すると 2 件目の購読が追加されます（既存の設定は上書きされません）。 |
| ru-RU | Конфигурация с таким же именем или ссылкой уже существует. Продолжение добавит вторую подписку (существующая не перезаписывается). |
| fr-FR | Une configuration du même nom ou avec le même lien existe déjà. Continuer ajoutera un second abonnement (l'existant n'est pas écrasé). |
| de-DE | Eine Konfiguration mit gleichem Namen oder Link existiert bereits. Fortfahren fügt ein zweites Abo hinzu (das bestehende wird nicht überschrieben). |

### `yamlEditor.backConfirmMsg`（confirm 正文，title 复用 `common.notice`）
| 语言 | 文案 |
|---|---|
| zh-CN | 当前有未保存的修改，确定要离开编辑器吗？离开后这些修改将丢失。 |
| zh-TW | 目前有未儲存的修改，確定要離開編輯器嗎？離開後這些修改將會遺失。 |
| zh-HK | 目前有未儲存的修改，確定要離開編輯器嗎？離開後這些修改將會遺失。 |
| en-US | You have unsaved changes. Leave the editor anyway? Your changes will be lost. |
| ja-JP | 未保存の変更があります。エディタを離れますか？変更は失われます。 |
| ru-RU | Есть несохранённые изменения. Выйти из редактора? Изменения будут потеряны. |
| fr-FR | Des modifications ne sont pas enregistrées. Quitter l'éditeur ? Elles seront perdues. |
| de-DE | Es gibt ungespeicherte Änderungen. Editor trotzdem verlassen? Die Änderungen gehen verloren. |

### `yamlEditor.saveReloadFailed`（alert 正文，含 {error} 插值）
| 语言 | 文案 |
|---|---|
| zh-CN | 配置文件已保存，但重新加载运行配置失败: {error} |
| zh-TW | 設定檔已儲存，但重新載入執行中設定失敗: {error} |
| zh-HK | 設定檔已儲存，但重新載入執行中設定失敗: {error} |
| en-US | Configuration file saved, but reloading the running configuration failed: {error} |
| ja-JP | 設定ファイルは保存されましたが、実行中の設定の再読み込みに失敗しました: {error} |
| ru-RU | Файл конфигурации сохранён, но перезагрузить работающую конфигурацию не удалось: {error} |
| fr-FR | Fichier de configuration enregistré, mais le rechargement de la configuration active a échoué : {error} |
| de-DE | Konfigurationsdatei gespeichert, aber das Neuladen der laufenden Konfiguration ist fehlgeschlagen: {error} |

## 2. 因本次修复而变成"死键"的旧文案（Wave 2 清理时注意）

| key | 原用途 | 现状 |
|---|---|---|
| `yamlEditor.loadFailed` | 只写入被删掉的 `statusText` | 已无引用，可计入死键清理 |
| `yamlEditor.syntaxInvalid` | 只写入被删掉的 `statusText` | 已无引用，可计入死键清理 |

> 其余 YamlEditor key（`saveError` / `formatError` / `saved` / `unsaved` / `syntaxValid` / `reloadConfirmMsg` / `readError` / `saveFileError`）仍在使用，勿删。
> Subscriptions 的 `sortBtn` / `sortModeHint` / `doneBtn` 仍被模板引用（只是排序入口被 `v-if` 隐藏），**Wave 1 不要删**，等后端 SaveSubOrder 落地恢复功能后再评估。
