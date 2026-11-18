# fix-2-shell 修复报告（frontend/src/App.vue）

> 代理：fix-2-shell（只改 frontend/src/App.vue，未改其它文件）
> 依据：docs/audit-frontend/02-app-shell.md（H-*）、06-contract.md（C-02/C-13）、01-state-core.md（S-09）
> 约束遵守：未改 locales / 未改后端 Go / 未跑 build·vue-tsc（只做了 SFC 解析+script 语法检查）
> 校验：SFC parse 0 错、template compile 0 错、script 语法 0 诊断；Events.On 14→15、unsub 15→16、addEventListener/removeEventListener 4/4 全部配对

| ID | 状态 | 改动位置(file:line) | 说明 |
|---|---|---|---|
| H-01 | fixed | App.vue:784-800 | watch(currentTab) 里补「离开 settings 就把 targetSettingsView 复位 main」，chip 那一次仍进 about（先写 about 再切页） |
| H-03（chip 部分） | fixed | App.vue:361-416 | 新增 appUpdateActionBusy，闸门提到 openModal 之前；onConfirm finally + onCancel 复位；重复点击给提示 |
| H-04 | fixed | App.vue:156, 499-508, 616-633, 1246-1257 | 新增 logFollowTail（距底 <24px 才跟随）+ @scroll.passive 维护；落批前判定，不再无条件滚到底；切回日志页复位为跟随 |
| H-05 | fixed | App.vue:157-159, 588-595, 554-558 | v-for 改用自增 seq 作稳定 key（入列时打 tag）；批窗口 16ms→50ms，满速时全量 filter+整表 diff 从 60Hz 降到约 20Hz |
| C-13 | fixed | App.vue:159, 588-595 | 同上：去掉 index key，头部 splice 裁剪不再让整表 key 平移 |
| H-06 | fixed | App.vue:159, 435-448, 1429-1431 | 模板 class 改用 normalizeLogType(log.type)；CSS .log-line.warning→.log-line.warn；删 logLevels.warning 死键（warning 仍归一成 warn 以兼容旧数据） |
| H-07 | fixed | App.vue:126-128, 460-461 | 新增「标准库(std)」来源 tab；缺 source 回落 'unknown'（不再冒认 core） |
| H-08 | fixed | App.vue:468-480 | 全局级别地板只在下拉选定「具体级别」时生效；选「全部」不再被压制 |
| H-09 | fixed | App.vue:994-1007, 597-614 | 订阅提到 StartStreamingLogs 之前；GetRecentLogs 改 mergeHistoryLogs（历史在前+活日志在后，按 ts/type/source/payload 指纹去重，含未落批的缓冲） |
| H-10 | fixed | App.vue:1164-1186 | App.ClearLogs 包 try/catch：失败 toast（displayErrorMessage 同类文案）+ 回拉 GetRecentLogs 对齐前后端 |
| H-11 | fixed | App.vue:361-370, 1068-1088, 1105-1129, 1155-1161, 1238-1239 | 新增 app-update-busy 监听（非设置页才提示，避免与 SettingsAbout/SettingsUpdate 重复）；下载/安装两入口加在途闸门，重复确认给提示而非静默 |
| H-12 | fixed | App.vue:372-380, 396-400, 1111-1115 | 空 path 统一走 openInstallPathEmptyAlert（chip 路径不再静默 return） |
| H-13 | fixed | App.vue:1015-1024 | 订阅 runtime-assets-status 后补一次 App.GetRuntimeAssetStatus() 快照兜底写入 globalState.assetStatus |
| H-19 | fixed | App.vue:811-852, 1208-1217 | resize 用 rAF 合并；IsMaximised 加查询序列号丢弃乱序结果；最大化复查定时器与 rAF 句柄在 onUnmounted 清理 |
| H-23 | fixed | App.vue:136-137 | 级别下拉 aria-label 从 logs.all 换成新键 logs.levelFilter |
| C-02（App 侧守卫） | partial | App.vue:66, 414, 741-782 | 侧栏切页与 chip 跳转统一走 requestTabChange（切页前判定未保存并 showConfirm，取消则不切页，确认在途禁止再入队）。返回箭头已由 YamlEditor 自己的 P-07 守卫（同批次 fix-4），App 侧 closeYamlEditor 直接放行避免双确认。**关窗（X/Alt+F4）路径无法在 App.vue 内完成**，见 handoff-2 与 backend-needs-2 |
| S-09（App 侧） | fixed | App.vue:435-448 | normalizeLogType 保留 'silent'（不再归成 info），logLevels 加 silent:4（「全部静音」，作为地板时任何条目都被过滤），与 store.normalizeLogLevel 口径一致 |

## 回归风险

1. **H-08 改变了日志面板默认观感**：默认「全部」不再套全局地板，环形缓冲区里已有的 info/warn 历史行会显示出来（原来只剩 error 及以上）。若产品认为这是回归，回退点只有一处：把 `if (logLevelFilter.value !== 'all')` 恢复成无条件计算地板。
2. **H-05 把批窗口从 16ms(1 帧) 提到 50ms(3 帧)**：单行显示延迟最多 +34ms，肉眼无感；但若评审要求「日志逐条即时出现」，把 `LOG_BATCH_MS` 改回 16 即可（key/seq 改动与之独立，可保留）。
3. **C-02 的守卫是异步切页且跨代理协作**：返回箭头依赖 fix-4 的 YamlEditor.handleBack(P-07)。若那条改动未落地，返回路径会重新变成静默丢弃（App.vue:777-782 的注释已写明改法）；另外 `currentTab` 现在可能在 await 之后才变，任何「切页后立刻读 DOM」的新代码都需要 nextTick。
