# i18n 需求（来自 fix-6-cards）

> 说明：Wave 1 不允许改 `frontend/src/locales/*`，代码里已直接使用下列 key。Wave 2 的 fix-i18n 补齐后即可正常显示；
> 在补齐之前：`traffic.*` 两键会把 key 名当文案显示（H-20 徽标），`deviation.severity.*` 会回落成后端原始 code（ignored / forced，可读）。
> 静态可被 `scripts/check-i18n.mjs` 扫到：`traffic.staleShort`、`traffic.staleHint`；`deviation.severity.*` 是动态拼接（`'deviation.severity.' + code`），脚本可能有盲区，需人工确认。

## 1 traffic（frontend/src/components/TrafficCard.vue）

```ts
traffic.staleShort  // 出站 IP 已过期徽标（极短，紧贴 IP）
traffic.staleHint   // 徽标 title：说明这个 IP 来自上一次检测
```

| locale | traffic.staleShort | traffic.staleHint |
|---|---|---|
| zh-CN | 已过期 | 这个 IP 来自上一次检测：切换代理或 TUN 后可能已不是当前出口 IP，点击可重新检测 |
| zh-TW | 已過期 | 這個 IP 來自上一次檢測：切換代理或 TUN 後可能已不是目前出口 IP，點擊可重新檢測 |
| zh-HK | 已過期 | 這個 IP 來自上一次檢測：切換代理或 TUN 後可能已不是目前出口 IP，點擊可重新檢測 |
| en-US | Stale | This IP came from the previous check and may no longer be the current outbound IP after switching proxy/TUN. Click to check again. |
| ja-JP | 期限切れ | この IP は前回の検出結果です。プロキシ／TUN を切り替えた後は現在の出口 IP ではない可能性があります。クリックで再検出します。 |
| ru-RU | Устарел | Этот IP получен при предыдущей проверке и после переключения прокси/TUN может уже не быть текущим исходящим IP. Нажмите, чтобы проверить снова. |
| fr-FR | Périmée | Cette IP provient de la vérification précédente et peut ne plus être l'adresse de sortie actuelle après un changement de proxy/TUN. Cliquez pour vérifier à nouveau. |
| de-DE | Veraltet | Diese IP stammt aus der letzten Prüfung und ist nach einem Proxy-/TUN-Wechsel möglicherweise nicht mehr die aktuelle Ausgangs-IP. Klicken zum erneuten Prüfen. |

## 2 deviation.severity（frontend/src/components/DeviationStrip.vue）

现有 `deviation` 块只有 `field` 与 `reason`，请在同级追加 `severity` 子对象：

```ts
deviation: {
  field: { ... },
  reason: { ... },
  severity: {
    ignored: '…',   // 设置写进了配置，但没被应用到内核
    forced: '…',    // 设置被程序强制改写成了别的值
  },
}
```

| locale | deviation.severity.ignored | deviation.severity.forced |
|---|---|---|
| zh-CN | 未生效 | 被改写 |
| zh-TW | 未生效 | 被改寫 |
| zh-HK | 未生效 | 被改寫 |
| en-US | Not applied | Overridden |
| ja-JP | 未適用 | 上書き |
| ru-RU | Не применено | Переопределено |
| fr-FR | Non appliqué | Remplacé |
| de-DE | Nicht angewendet | Überschrieben |

> 徽标空间很窄（`.deviation-item__severity` 是单行 pill），请保持 ≤ 12 个全角字符 / ≤ 16 个半角字符。
