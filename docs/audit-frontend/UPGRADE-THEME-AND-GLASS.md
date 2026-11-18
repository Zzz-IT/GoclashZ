# 主题系统与备份的兼容性（2026-11-19 补充）

> 用户提问：「新设计的主题系统备份时会被备份吗？以及玻璃模式下滑条的默认值」

## 1 主题系统会被备份吗？

**会，而且备份的是完整配置。**

主题系统的所有字段都存在 `AppBehavior` 里（`core/appcore/behavior.go:637-649`）：

| 字段 | 含义 | 备份位置 |
|---|---|---|
| themeMode | 明暗偏好（light/dark/system） | Settings/user_behavior.json |
| themePreset | 预设配色 id | Settings/user_behavior.json |
| themeAccent | 自定义强调色 hex | Settings/user_behavior.json |
| themeRadius | 圆角档位 | Settings/user_behavior.json |
| themeAura | 光晕开关 | Settings/user_behavior.json |
| glassBlurEnabled | 玻璃开关 | Settings/user_behavior.json |
| glassBlurStrength / glassBlurContentStrength / glassBlurOverlayStrength / glassBlurControlStrength | 四级模糊强度 | Settings/user_behavior.json |
| glassTintStrength | 着色浓度 | Settings/user_behavior.json |
| glassDepth | 厚度总控 | Settings/user_behavior.json |

`theme_setting.txt` 也在备份包里（archive.go:47），它是"已解析的明暗真源"，用于首屏避免闪烁；恢复时会一起还原。

**唯一不在备份里的是前端 localStorage 缓存**（`goclashz_theme_v2`），但那是纯缓存，后端 behavior 才是真源，恢复后 Behavior.Load() 会同步过来，不会丢设置。

## 2 玻璃滑条默认值（前后端一致）

| 滑条 | 后端默认值 | 前端常量 | 范围 |
|---|---|---|---|
| 厚度总控（glassDepth） | 50 | GLASS_DEPTH_DEFAULT = 50 | 0..100 |
| 着色浓度（glassTintStrength） | 58 | GLASS_TINT_DEFAULT = 58 | 0..100 |
| 内容卡片模糊 | 12 | （旧字段迁移用） | 0..80 |
| 浮层模糊 | 18 | — | 0..80 |
| 控件凹槽模糊 | 8 | — | 0..80 |

**设计说明**（behavior.go:471 附近）：
- 玻璃模糊分四级（内容卡片 / 浮层 / 控件凹槽 / 旧版单强度），每级有独立默认值；
- 厚度总控（glassDepth）是"一条滑条同时缩放窗口着色层与四级面板填充"的总开关，不给每一级单独开滑条，避免拖出"凹槽比卡片还实"的组合；
- 着色浓度（glassTint）在强调色为黑白灰时自动收起（`isNeutralColor` 判定），因为中性色没有色相可染；
- 旧版配置只有 glassBlurStrength（单强度），升级时 migrateGlassStrengths 会把它播种到四级模糊里。

## 3 恢复备份后玻璃设置会生效吗？

会。恢复流程里 Behavior.Load() 会重新读 user_behavior.json，玻璃字段一起回来；前端 SettingsTheme.vue 的 ensureGlassDefaults 会补默认值（如果旧备份缺字段），然后随下一次 save 持久化。

唯一注意点：如果恢复的是 v1.4.0 的备份，里面没有 glassDepth / glassTintStrength 等新字段，恢复后这些字段走默认值（depth=50, tint=58），用户之前调过的玻璃参数不会丢（因为 v1.4.0 根本没有这些参数）。