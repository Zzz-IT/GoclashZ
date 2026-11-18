# backend-needs（fix-2-shell / App.vue 相关）

> 本 Wave 不改 Go，以下为前端侧已完成双兼容、根治需要后端配合的事项。App.vue 当前实现均可独立工作（不阻塞验收）。

| # | 需求 | 为什么 | App.vue 现状（双兼容） |
|---|---|---|---|
| 1 | **LogEntry 增加单调 seq** | 日志没有唯一 id：H-05 只能前端自增 seq 做渲染 key；H-09 的「历史快照 + 实时流」合并只能按 `ts+type+source+payload` 内容指纹去重，同一毫秒内容相同的两行会被误合并成一行 | 前端自增 seq（App.vue:591）与内容指纹去重（App.vue:597-614）都可独立工作；后端 seq 到位后可直接替换 |
| 2 | **app-update-busy 带 payload（进行中的任务名/类型）** | 现在只发 Void，前端只能提示「已有软件更新任务正在进行」，无法说明是下载还是安装、进展到哪 | 现有监听按 Void 处理（App.vue:1158）；若后端改带 payload，前端读 `data?.task` 即可，旧 Void 路径（undefined）继续兼容 |
| 3 | **窗口关闭前的未保存确认（C-02 第三路径）** | WindowClosing hook（main.go:401-404）无条件 `e.Cancel(); Hide()`，前端拿不到关闭前的询问机会；用户在 YAML 编辑器里有未保存改动时点 X，改动会随窗口隐藏/退出丢失 | App 侧已维护 `yamlEditorModified`（App.vue:341, 741-782）但无入口。需要后端在 hook 里 emit「before-close」事件并等待前端回传，或提供 `RequestClose` 绑定由前端确认后再调 —— 属 Wails 层设计，非本 Wave |
| 4 | （可选）GetRecentLogs 返回 snapshot + lastSeq | 与 #1 配套：前端可用 seq 精确判断「快照末尾」与「实时流起点」是否连续，彻底消除启动期空洞 | 当前 mergeHistoryLogs 已把空洞缩到「去重粒度」，无 seq 也能用 |

> 注意：`app-update-busy` 已在 v3_events.go:124 注册，**无需新增注册**；SettingsAbout.vue:375 与 SettingsUpdate.vue:104 也已有监听（02-app-shell.md 的「前端零监听」结论漏了这两处）。
