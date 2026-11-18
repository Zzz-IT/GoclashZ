# handoff-3 · fix-3-proxy 移交项（跨文件 / 需其他代理或主控决定）

> 我只改 `Proxies.vue` / `Connections.vue`。以下 6 项落在我的文件之外或需要主控拍板。

## 1. P-13 剩余：`Rules.vue` 的一对死钩子 → fix-4-pages

审计 P-13 覆盖三页，我只处理了自己的两页。`Rules.vue:131` 仍 import 了 `onActivated`，`:568` 仍是死钩子（本应用无 KeepAlive，永不会派发）。
- 动作：fix-4-pages 删 `onActivated` 导入与 `:568` 那段（如果它那轮没覆盖，请主控补一条任务）。

## 2. P-34：局部 storage 包装建议上提（Wave 3 重构）

我在两个页面各写了 6 行的 `readStorage`/`writeStorage`（不新建文件，因为 `utils/*` 属 fix-7-utils）。审计建议的 `utils/storage.ts` 仍是更好的收敛点（`Rules.vue` 也有同样两处）。
- 动作：Wave 3 抽 `safeGet/safeSet` 后，把两个页面的局部函数替换成 import（机械改动，行为不变）。

## 3. 新 i18n key 补 8 语言 → fix-i18n

`proxies.loadFailedHint`（8 语言文案见 `i18n-needs-3.md`）。补齐前该处会显示 key 字面量（`t()` 兜底返回 path，不会白屏）。

## 4. Wave 2 若改 `proxy-test-finished` payload / 加 batchId → fix-backend

- 消费侧已收敛到 `Proxies.vue` 的 `readTestFinish()`（`Proxies.vue:753-766`）一个函数：优先读 `{ok,message}`，回退旧 string。再改 payload 只改这里。
- 后端若下发 `batchId`，把监听器里的 `testBatchStarted`/"本批是否开测"判据换成 batchId 比较（batch 状态变量在 `Proxies.vue:740-746`）。
- **不要**把 `testAllDelays` 里的冻结快照改回 `filteredAndSortedNodes`（`Proxies.vue:798-800` 现在用的是未过滤的 `sortedNodes`），那会让 P-10 回归。

## 5. P-08 的 Proxies 侧（`config-changed` 监听）我**没有**动

`Proxies.vue:901-903` 的 `config-changed → loadData` 监听按审计是"当前不可达"的死监听（全仓唯一 emit 点在保存激活配置时，而那会儿本页已被 `v-if` 销毁）。P-08 归 fix-4-pages，且它无法改我的文件；我保留不删是因为 P-08 的方案 1 一旦落地（后端补 `config-content-changed`），这里就是现成挂载点。
- 请主控决定：删（1 行，去掉死代码）还是留（留着接新事件）。两页各自的判断都写在这里，不要两头都改。

## 6. R-7 / R-9 中 Proxies.vue 的其它死代码与矛盾注释（不在我的条目内，未动）

- `::-webkit-scrollbar-button/track/thumb`（`Proxies.vue` tail 一段，因 `display:none` 永不生效）
- `.size-standard` 与基础 `.node-grid` 重复、节点对象 `now` 字段无读者、两个 `onMounted`
- 注释里"滚动条 4px / 原生滚动条回到轨道内"与 `scrollbar-width: none` 矛盾
这些属于 R 类冗余，与本轮 ID 无关；若主控要清，请另派（不要与我的本轮 diff 混在一起）。

## 7. 验证建议（主控统一跑）

- `vue-tsc --noEmit` 关注点：`Events.On` 回调参数注解 `{ data?: unknown }`（`Proxies.vue:905`）、`hasPointerCapture` 可选调用（`Proxies.vue:255`）、`catch {}` 空绑定（两文件 `readStorage`）。
- 运行时重点手测三条：① 测速中点任意节点（P1-G）；② 停止测速后 600ms 内立刻再测（P1-E）；③ 暂停刷新 → 关全部连接（P-20）。
