# i18n 需求 · fix-1-state

> 来源：Wave 1 修复引入的新文案 key。代码里已用 `tr('key', 原文兜底)` 形式调用，
> **语言包补齐前 UI 回落后端原文/中文兜底，不会露出 key 字面量**；补齐后自动生效，无需再改代码。
> 两个 key 都不在 `t('字面量')` 位置（走 `tr()`），所以 `scripts/check-i18n.mjs` 当前不会报缺键；
> 补齐后它们会被静态扫描视作「死键（warn，不阻断）」——属预期，因为 key 来自映射表。

## 1 需要的 key

| key | 用途 | 关联 ID | 出现位置 |
|---|---|---|---|
| `settings.update.checkFailed` | 「检查内核更新」失败提示（toast，占位符 `{error}` = 人话化后的后端错误） | C-04 | store.ts:1154 |
| `notify.systemProxyStateUnknown` | 系统代理状态读取失败的通知文案（后端 code `SYSTEM_PROXY_STATE_UNKNOWN`） | C-20 | store.ts:981（经 localizedNotificationText 使用） |

## 2 8 语言文案

### zh-CN
```ts
settings: {
  update: {
    checkFailed: '检查内核更新失败：{error}',
  },
},
notify: {
  systemProxyStateUnknown: '系统代理状态读取失败，已跳过自动恢复（不会盲目开启）',
},
```

### zh-TW
```ts
settings: {
  update: {
    checkFailed: '檢查內核更新失敗：{error}',
  },
},
notify: {
  systemProxyStateUnknown: '系統代理狀態讀取失敗，已跳過自動恢復（不會盲目開啟）',
},
```

### zh-HK
```ts
settings: {
  update: {
    checkFailed: '檢查內核更新失敗：{error}',
  },
},
notify: {
  systemProxyStateUnknown: '系統代理狀態讀取失敗，已跳過自動還原（唔會亂咁開）',
},
```

### en-US
```ts
settings: {
  update: {
    checkFailed: 'Failed to check for core updates: {error}',
  },
},
notify: {
  systemProxyStateUnknown: 'Could not read the system proxy state, so automatic restore was skipped (it will not be enabled blindly).',
},
```

### ja-JP
```ts
settings: {
  update: {
    checkFailed: 'カーネルの更新確認に失敗しました：{error}',
  },
},
notify: {
  systemProxyStateUnknown: 'システムプロキシの状態を取得できませんでした。自動復元をスキップしました（勝手に有効化しません）。',
},
```

### ru-RU
```ts
settings: {
  update: {
    checkFailed: 'Не удалось проверить обновление ядра: {error}',
  },
},
notify: {
  systemProxyStateUnknown: 'Не удалось прочитать состояние системного прокси; автоматическое восстановление пропущено (включение «наугад» не выполняется).',
},
```

### fr-FR
```ts
settings: {
  update: {
    checkFailed: 'Échec de la vérification des mises à jour du noyau : {error}',
  },
},
notify: {
  systemProxyStateUnknown: 'Impossible de lire l’état du proxy système ; la restauration automatique a été ignorée (aucune activation à l’aveugle).',
},
```

### de-DE
```ts
settings: {
  update: {
    checkFailed: 'Prüfung auf Kernel-Updates fehlgeschlagen: {error}',
  },
},
notify: {
  systemProxyStateUnknown: 'Status des System-Proxys konnte nicht gelesen werden; automatische Wiederherstellung wurde übersprungen (keine Aktivierung auf Verdacht).',
},
```

## 3 落地注意

1. `notify` 是**顶层新命名空间**（与 `common`/`logs`/`settings` 同级），8 个语言包都要加，否则 check-i18n 会报「某语言缺少 key」。
2. `{error}` 必须保留在 8 语言里（`t()` 用 `\{(\w+)\}` 做插值，缺占位符则该段不显示）。
3. 补齐后不需要改 store.ts：`tr()` 一旦命中 key 就自动用语言包文案替换后端原文。
