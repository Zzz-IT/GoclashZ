# i18n-needs-3 · fix-3-proxy 新增文案

> 我只改 `Proxies.vue` / `Connections.vue`，没有动 `frontend/src/locales/*`。
> Wave 2 的 fix-i18n 请把下列 key 补进 8 个语言包（`proxies` 命名空间）。
> 代码里已经用 `t('proxies.loadFailedHint')`（缺失时 `t()` 返回 key 本身，不会白屏）。

## 新增 key（1 条）

### `proxies.loadFailedHint`（用途：P-16 代理页"读取配置失败"错误态文案，替换原来的"请先添加配置"）

| 语言 | 文案 |
|---|---|
| zh-CN | 读取代理配置失败，请检查配置是否有效后重试。 |
| zh-TW | 讀取代理設定失敗，請檢查設定是否有效後重試。 |
| zh-HK | 讀取代理配置失敗，請檢查配置是否有效後重試。 |
| en-US | Failed to read the proxy configuration. Check that the profile is valid and try again. |
| ja-JP | プロファイルの読み込みに失敗しました。設定が有効か確認して再試行してください。 |
| ru-RU | Не удалось прочитать конфигурацию прокси. Проверьте корректность профиля и повторите попытку. |
| fr-FR | Échec de la lecture de la configuration du proxy. Vérifiez que le profil est valide et réessayez. |
| de-DE | Die Proxy-Konfiguration konnte nicht gelesen werden. Prüfen Sie das Profil und versuchen Sie es erneut. |

插入位置建议：紧跟 `proxies.noGroupsHint`（zh-CN.ts:127）之后。

## 复用的既有 key（不需要新增）

- `proxies.speedTestFailed`（{error} 插值）—— P-02 失败原因 toast
- `proxies.speedTestBusy` —— P-02 `DELAY_TEST_BUSY` 专用提示
- `proxies.testError` —— P-02 失败原因缺失时的兜底文案
- `common.retry` —— 未使用（P-16 错误态未加"重试"按钮，避免新增交互）
