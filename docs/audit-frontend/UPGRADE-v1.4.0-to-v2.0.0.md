# 升级兼容审计：v1.4.0 → v2.0.0（2026-11-19）

> 触发：改版本号为 v2.0.0 时用户要求检查与云端 v1.4.0 的升级兼容与备份恢复问题。
> 方法：对比本机旧版仓库 D:\GOLANG\GoclashZ（v1.4.0-1-g09528ed）与新版 D:\GOLANG\GoclashZv3（v3 分支，版本已改为 v2.0.0）的实际源码，每条结论带 file:line。

## 结论摘要

**可以平滑升级**。v1.4.0 → v2.0.0 在更新通道、数据目录、设置文件、订阅索引、helper 协议这五个关键面都是兼容的，没有发现会让老用户数据丢失或升级后立即坏掉的问题。

需要注意两点：
1. **helper 服务没有版本握手**：升级后运行中的 SYSTEM 服务仍是旧版二进制，直到重启或手动 stop；目前 9 个 RPC 方法名一致、参数兼容，所以不会立即坏，但如果未来给 start-core 加了新参数，老 helper 会静默忽略（见风险 H-1）。
2. **前端主题缓存键新增 v2**：旧版只有 goclashz_theme，新版用 goclashz_theme_v2。升级后首次启动会走一次默认主题（暗色），但后端 behavior 的 themeMode 会在 initStore 后同步过来，用户视角只是首屏闪一下暗色，不丢设置（见风险 F-1）。

---

## 风险清单

| # | 风险 | 严重度 | 证据 | 影响 | 建议 |
|---|---|---|---|---|---|
| U-1 | 更新器版本比较：主版本跳跃 v1.4.0→v2.0.0 能正确识别 | ✅ 兼容 | core/downloader/app_update.go:271 CompareAppVersion 逐位比较 | 无 | 无需处理 |
| U-2 | 更新包资产匹配：旧版与新版 selectWindowsAsset 逻辑一致，优先 setup/installer | ✅ 兼容 | app_update.go:211-269（两版 diff 仅 json 包与注释） | 无 | 发布时资产名保持 GoclashZ_win_amd64_Setup.exe |
| U-3 | 安装器身份：AppName/AppVersion/AppMutex/DefaultDirName/PrivilegesRequired/OutputBaseFilename 两版一致 | ✅ 兼容 | package.iss 两版关键字段逐项相同 | 无 | 无需处理 |
| U-4 | 数据目录解析：GOCLASHZ_DATA_DIR 覆盖 → install-profile.json → .portable → 默认，两版逻辑一致 | ✅ 兼容 | paths.go:98/102（env）、21（profile）、111/115（portable） | 无 | 无需处理 |
| U-5 | AppBehavior 字段：旧版 30 个，新版 43 个，**只增不删不改名** | ✅ 兼容 | behavior.go 字段 diff | 无 | LoadSetting 自动修补 + migrateGlassStrengths 兜底 |
| U-6 | revision 乐观锁：旧备份/旧文件无该字段时=0，首次保存正常 | ✅ 兼容 | behavior.go SetActiveConfig:201 next.Revision=cache+1；CAS 只在 SetAndSave 且 revision!=0 时校验 | 无 | 无需处理 |
| U-7 | DesiredState 字段：两版完全相同（7 个字段） | ✅ 兼容 | desired_state.go 字段 diff | 无 | 无需处理 |
| H-1 | helper 服务无版本握手：升级后运行中的 SYSTEM 服务仍是旧二进制 | ⚠️ P2 | helper_protocol.go 无 version 字段；9 个 RPC 方法名两版一致 | 未来加新参数时旧 helper 静默忽略 | 见「升级前建议」 |
| F-1 | 前端主题缓存键新增 v2：goclashz_theme → goclashz_theme_v2 | ⚠️ P2 | theme.ts: THEME_CACHE_KEY=goclashz_theme_v2, LEGACY_THEME_KEY=goclashz_theme | 首屏闪默认暗色，后端 behavior 同步后恢复 | 可接受；如需消除可在 theme.ts 加一次性迁移 |

---

## 升级前建议

1. **helper 版本握手**（可选，非阻塞）：在 HelperRequest 加 version 字段，客户端启动时 ping 一次拿版本，不一致则提示重启 helper。当前不阻塞发版，因为 9 个 RPC 方法名一致。
2. **发布资产名**：保持 GoclashZ_win_amd64_Setup.exe 不变，确保 v1.4.0 客户端能识别。
3. **备份恢复**：见 UPGRADE-BACKUP-RESTORE.md，跨版本恢复安全。

---

## 手动验收步骤

1. 在 v1.4.0 上导出备份（设置→关于→导出备份）；
2. 升级到 v2.0.0（覆盖安装或自动更新）；
3. 检查：设置页所有开关、主题、订阅列表、激活配置是否与升级前一致；
4. 在 v2.0.0 上还原 v1.4.0 的备份（设置→关于→还原备份→选择模式）；
5. 检查：还原后设置、订阅、主题是否恢复；切换任意开关保存，确认无冲突错误。