# handoff（fix-2-shell → 主控 / 其它代理）

> 我独占 frontend/src/App.vue，以下 5 条必须跨文件或跨代理处理，App 侧我已做到能做的部分。

| # | 事项 | 现状（App 侧已做） | 需要谁做什么 |
|---|---|---|---|
| 1 | **C-02 关窗（X / Alt+F4）时未保存的 YAML 仍会丢** | 无法在 App.vue 内完成：窗口关闭是 Go 侧 WindowClosing hook（main.go:401-404 无条件 Cancel+Hide），WebView 侧拿不到「关闭前询问」的机会 | 需 Wails 层配合（见 backend-needs-fix-2.md #3）。若暂时不做，这条保持已知缺陷，不要假装已修 |
| 2 | **C-02 返回箭头依赖 fix-4 的 P-07** | App.vue:777-782 的 closeYamlEditor 直接切页 + 清 yamlEditorModified，因为 YamlEditor.handleBack 已自行 showConfirm（工作区里 fix-4 的改动已在） | 若 fix-4 的 P-07 最终被回退，App.vue:779-782 必须改成 `void requestTabChange('subs')`（否则返回路径重新变成静默丢改动）；这段注释已写在代码里 |
| 3 | **app-update-busy 的提示归属** | App.vue:1155-1161 只在 `currentTab !== 'settings'` 时 toast（设置页内由 SettingsAbout.vue:375 / SettingsUpdate.vue:104 自己弹 alert，避免一条事件两次提示） | 若 fix-5-settings 改掉这两处监听（比如去掉弹窗或改文案），App 侧条件要同步；另外 02-app-shell.md H-11 说「前端零监听」只覆盖了 App.vue/store.ts，遗漏了这两个设置子页，总报告口径可修正 |
| 4 | **S-09 的 store 侧口径** | App.vue 的 logLevels 现为 {debug:0,info:1,warn:2,error:3,silent:4}，silent 保留 | 若 fix-1-state 把 store.normalizeLogLevel 的 silent 处理改成别的（例如归成 error），App 侧 logLevels 必须同步，否则地板 rank 不一致 |
| 5 | **后续若后端给 LogEntry 加 seq** | App.vue:588-595 目前是前端自增 seq（log.seq）作 v-for key | 后端 seq 到位后可直接改用后端 seq（模板 key 不变），并让 H-09 的合并去重从「内容指纹」升级为「seq 去重」；当前实现与后端 seq 不冲突 |

## 我改过但需要主控知晓的「刻意行为变化」

1. 日志页默认「全部」不再被全局日志级别地板压制（H-08 的修复目标），因此环形缓冲区里已有的 info/warn 行会显示出来。
2. 日志批渲染窗口 16ms → 50ms（H-05/C-13），单行显示最多晚约 50ms。
3. 侧栏切页改为异步守卫（requestTabChange）：有未保存 YAML 时先弹确认，取消则侧栏高亮不动。

## 验收提醒

- 我只验证到「SFC 解析 + template 编译 + script 语法」层面（0 错），未跑 vue-tsc（按要求由主控统一跑）。
- 自检：Events.On 14→15、unsub 调用 15→16、add/removeEventListener 4/4，全部配对。
- `node scripts/check-i18n.mjs` 现在报 `logs.std / logs.levelFilter / yamlEditor.leaveConfirmMsg` 三条「代码使用但 zh-CN 未定义」，与 i18n-needs-fix-2.md 一一对应（另有其它代理的 12 条）。
