# 修复总台账（评审 → 修复 → 验收）

> 备份基线：`.fixbackup/20261118-auditfix/`（src / bindings / scripts 全量副本，可回滚）
> 状态：⏳ 未开始 · 🔧 进行中 · ✅ 已完成 · ⏭ 有意不做（附理由）
>
> **最终状态（2026-11-18）**：Wave 1 ✅ 全部完成（7 个分片）、Wave 2 ✅ 完成（i18n + 后端 ReorderSubs/结构化 payload）、
> Wave 3 部分完成（r1 公共层收敛 ✅、圆角 token 化 ✅；R-A1/R-A2/R-B3 三个纯重构 ⏭ 后移，理由见 FIX_REPORT.md §4）。
> 四道闸门全绿：manifest 0 / i18n 0 / vue-tsc 0 / vite build 成功；Go：build 0 / vet 0 / test 0。
> 逐条结论见 `FIX_REPORT.md`，各分片细节见 `fix-*.md`。

## 分片归属（Wave 1，文件互不重叠）

| 代理 | 独占文件 | 负责条目 |
|---|---|---|
| fix-1-state | frontend/src/store.ts, frontend/src/main.ts, frontend/src/api.ts | S-01, S-02, S-03, S-04, S-05, S-06, S-07, S-09, S-10, S-11, C-04, C-10, C-11, C-12, C-20 |
| fix-2-shell | frontend/src/App.vue | H-01, H-03(chip 部分), H-04, H-05, H-06, H-07, H-08, H-09, H-10, H-11, H-12, H-13, H-19, H-23, C-02(App 侧守卫), S-09(App 侧 normalizeLogType) |
| fix-3-proxy | frontend/src/components/Proxies.vue, frontend/src/components/Connections.vue | P1-E(C-06), P1-G(P-03), P-02, P-04, P-09, P-10, P-11, P-12, P-13, P-14, P-15, P-16, P-17, P-18, P-19, P-20, P-21, P-34 |
| fix-4-pages | frontend/src/components/Rules.vue, frontend/src/components/Subscriptions.vue, frontend/src/components/YamlEditor.vue | P-01, P-05, P-06, P-07, P-08, P-22, P-23, P-24, P-25, P-26, P-27, P-28, P-29, P-30, P-31, P-32, P-33, P-34 |
| fix-5-settings | frontend/src/components/Settings*.vue(13 个，含 Settings.vue), frontend/src/settings/manifest.ts, frontend/src/components/ModernSelect.vue, ModernNumberInput.vue, UpdateTaskPanel.vue, UpdateTaskRow.vue | G-01, G-02, G-03, G-04, G-05, G-06, G-07, G-08, G-09, G-10, G-11, G-12, G-13, G-14, G-15, G-16, G-17, G-18, G-19, G-20, G-21, G-22, G-23, G-24, U-20 |
| fix-6-cards | frontend/src/components/Overview.vue, TrafficCard.vue, DeviationStrip.vue, Toast.vue, EmptyState.vue, Sidebar.vue, frontend/src/composables/useTunToggle.ts, frontend/src/trafficWaveState.ts | H-02, H-14, H-15, H-16, H-17, H-18, H-20, H-21, H-22, S-08, C-19 |
| fix-7-utils | frontend/src/utils/*.ts(10 个), frontend/src/styles/*.css, frontend/src/style.css | U-03(代码侧), U-05, U-06, U-07, U-08, U-09, U-10, U-11, U-12, U-13, U-14, U-15, U-16, U-18, U-19, U-22 |

## Wave 2（依赖 Wave 1 完成）

| 代理 | 范围 | 内容 |
|---|---|---|
| fix-i18n | frontend/src/locales/*（8 个语言包）+ scripts/check-i18n.mjs | 合并各代理的 i18n-needs；删 188 死键（先修脚本动态 key 盲区）；8 语言对齐 |
| fix-backend | Go 侧 delay_manager/updater/subs + bindings 重生成 + store.ts/Proxies.vue 的消费侧 | proxy-test-finished 结构化 payload；SaveSubOrder 排序落盘；NOTIFICATION_UI 与后端 code 对齐 |

## Wave 3（重构，风险最高，最后做）

| 项 | 内容 |
|---|---|
| R-A | 抽 composables：useTabSlider（4 份同构）、useAsyncAction（11 处三件套）、alertError（9 处） |
| R-B | 设计系统：border-radius 走 163 处 token 化、z-index token 落地、11 个设置页样式合并 |

## 验收闸门

1. `node scripts/validate-settings-manifest.mjs` → 0
2. `node scripts/check-i18n.mjs` → 0（死键允许 warn，但 key 必须齐全）
3. `frontend/node_modules/.bin/vue-tsc --noEmit` → 0
4. `go build ./... && go vet ./...` → 0（若动了 Go）
5. 独立评审代理逐文件 review diff，确认无行为回归
