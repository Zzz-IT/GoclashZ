# 备份恢复兼容审计：v1.4.0 ↔ v2.0.0（2026-11-19）

> 触发：改版本号为 v2.0.0 时用户要求检查备份恢复兼容性。
> 方法：对比两版 core/backup/archive.go 与 core/appcore/backup_service.go 的实际源码。

## 结论摘要

**v1.4.0 的备份可以安全恢复到 v2.0.0**。备份包结构（manifest.json + Settings/Subscriptions/profiles/config.yaml/theme_setting.txt/offline_nodes.json）两版一致，恢复流程有事务化保护，新字段走默认值修补。

**v2.0.0 的备份恢复到 v1.4.0（降级）**：备份包里有 v2 新增的字段（themeMode/glass*/revision 等），v1.4.0 的 BehaviorStore 不认识这些字段，但 Go 的 json 反序列化会忽略未知字段，所以**降级恢复也不会坏**，只是主题相关设置会丢。

---

## 备份包结构（两版一致）

```
manifest.json          # { app, backupVersion, appVersion, createdAt }
Settings/              # user_behavior.json, user_desired_state.json, user_dns.json, ...
Subscriptions/         # *.yaml 订阅文件
profiles/              # index.json（订阅索引）
config.yaml            # 内核运行配置
theme_setting.txt      # 明暗主题缓存
offline_nodes.json     # 离线节点选择记忆
```

备份版本：两版都是 CurrentBackupVersion=2，互相兼容。

---

## 恢复流程（v2.0.0）

1. 停内核 + 关系统代理（backup_service.go:46-60）；
2. 事务化还原文件（archive.go RestoreTransactional）：先备份当前到 rollbackDir，再按模式覆盖，失败回滚；
3. 重载：clash.LoadIndex() → MigrateRuleStorageV2() → Behavior.Load() → Desired.Load() → Offline.Load()（backup_service.go:74-95）；
4. 活跃配置自愈：如果还原后的订阅里找不到当前 activeConfig，自动切到首个可用或清空（backup_service.go:98-116）；
5. Supervisor.Reconcile 调和运行态（backup_service.go:122）；
6. 前端收到成功后 emit reload-behavior，让父组件重读 behavior（SettingsAbout.vue:378）。

---

## 跨版本恢复逐项判断

| 项 | v1.4.0 备份 → v2.0.0 | v2.0.0 备份 → v1.4.0（降级） |
|---|---|---|
| manifest.json | ✅ backupVersion=2 兼容 | ✅ 同 |
| user_behavior.json 缺新字段 | ✅ LoadSetting 自动修补默认值 + migrateGlassStrengths | N/A |
| user_behavior.json 多新字段 | N/A | ✅ Go json 忽略未知字段 |
| revision 乐观锁 | ✅ 旧文件无 revision=0，首次保存正常 | N/A |
| user_desired_state.json | ✅ 字段两版一致 | ✅ 同 |
| 订阅索引 index.json | ✅ SubIndexItem 结构两版一致 | ✅ 同 |
| 内核二进制（runtime/） | ✅ 不在备份包里 | ✅ 同 |
| 前端主题缓存 | ✅ localStorage 键独立，恢复后后端 behavior 同步 | ⚠️ 主题设置丢（v1.4.0 不认识） |

---

## 风险清单

| # | 风险 | 严重度 | 证据 | 触发条件 | 建议 |
|---|---|---|---|---|---|
| B-1 | 恢复后 Behavior.Load() 换掉内存 behavior，但前端快照还是旧的 | ✅ 已修 | backup_service.go:82 + SettingsAbout.vue:378 emit reload-behavior | 恢复后立即改设置 | 无需处理 |
| B-2 | 恢复后 revision 被重置为备份里的值 | ✅ 兼容 | behavior.go:201 SetActiveConfig 用 cache.Revision+1 | 恢复后首次保存 | 无需处理 |
| B-3 | 恢复后活跃配置不存在 | ✅ 已处理 | backup_service.go:98-116 自愈逻辑 | 备份里的订阅与当前不同 | 无需处理 |
| B-4 | 降级恢复主题设置丢失 | ⚠️ P2 | v1.4.0 不认识 themeMode 等字段 | v2.0.0 备份 → v1.4.0 | 可接受（降级场景） |

---

## 手动验收步骤

1. v1.4.0 导出备份；
2. 升级到 v2.0.0；
3. 改几个设置（主题、语言、订阅顺序）；
4. 还原 v1.4.0 的备份；
5. 检查：设置是否恢复到备份时的状态；切换任意开关保存，确认无冲突；
6. 检查订阅列表、激活配置、节点选择是否与备份一致。