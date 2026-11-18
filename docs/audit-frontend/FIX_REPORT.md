# 前端审计修复总报告（2026-11-18）

> 依据：docs/audit-frontend/00-SUMMARY.md（审计总报告，0 个 P0 / 20 个 P1 / 约 95 个 P2）
> 方式：10 个代理分 3 个 Wave 并行修复 + 主控集成 + 2 个独立评审代理复核
> 备份基线：.fixbackup/20261118-auditfix/（src / bindings / scripts 全量，可整目录回滚）
> 规模：59 个文件改动，+2805 / -1999 行；新增 3 个工具模块（alertError / storage / clipboard）
> 评审：2 个独立只读评审代理覆盖全部改动 → BLOCKER 0 / MAJOR 2（均已修）/ MINOR 23（已修 9，其余为注释级）

---

## 0 验收闸门（全部通过）

| 闸门 | 命令 | 结果 |
|---|---|---|
| 设置清单锚点 | node scripts/validate-settings-manifest.mjs | **PASS**（103 项锚点） |
| i18n 对齐 + key 存在性 | node scripts/check-i18n.mjs | **PASS**（752 键 × 8 语言，死键 186 → **0**） |
| TypeScript 全量 | vue-tsc --noEmit | **PASS**（0 错） |
| 生产打包 | vite build | **PASS**（175 模块，dist 产出正常；仅有既存的 500kB chunk 体积警告） |
| Go 构建 | go build . | **PASS** |
| Go 静态检查 | go vet ./... | **PASS** |
| Go 单测 | go test . | **PASS**（含 services 映射计数校验） |

---

## 1 修复统计

| 项 | 数量 |
|---|---|
| 修复代理 | 9 个（7 个 Wave1 分片 + i18n + 公共层重构） |
| 修复条目（含 P2 与冗余项） | 约 140 条 |
| **P1 全部 20 条** | **20 / 20 已修**（详见 §2） |
| P2 | 约 85 条已修；6 条降级为「已知残余」（见 §4）；4 项大重构按期后移 |
| 死代码删除 | 102 个 i18n 死键 × 8 语言、1 个整文件（normalize.ts）、6 个死导出、约 250 行死 CSS/死分支、6 个死钩子 |
| 设计系统 | border-radius 90 处改走 --ui-radius-*（消费点 7 → 97） |
| 新增 i18n key | 23 个 × 8 语言 |

---

## 2 P1 清单最终状态（20/20）

| ID | 问题 | 状态 | 落点 |
|---|---|---|---|
| P1-A | AppBehavior 整对象写回多入口互相覆盖 | **已修** | store.ts 单字段读改写 + Settings.vue 读-合并-写 + 串行链 + SettingsAbout 还原后重拉 |
| P1-B | 局域网页重置错段名 | **已修** | SettingsLan emit lan + Settings.vue 映射生效 |
| P1-C | pending 看门狗误清合法等待 | **已修** | 有模态在台时不计时；复位前留日志 |
| P1-D | desired* 被 UI 直写无 revision 保护 | **已修** | overlay 记录发起 revision，落后即丢弃 |
| P1-E | 停止测速→立刻重测的批次竞态 | **部分（残余窗口 ≤600ms）** | 批次序号 + 启发式 + 结构化 payload；彻底闭环需后端下发 batchId（§5-B1） |
| P1-F | 连点确认框执行 N 次破坏性操作 | **已修** | Overview / TrafficCard / App.vue chip 三处闸门前置 |
| P1-G | 测速中点节点导致转圈失联 | **已修** | loadData 作废冻结快照 + 状态迁移 |
| P1-H | Connections 卸载竞态 | **已修** | 卸载判据 + 不写已卸载状态 |
| P1-I | YamlEditor 监听/Observer 泄漏 | **已修** | 注册前移 + disposed 判据 + view.destroy |
| P1-J | 批量测速失败全静默 | **已修** | 后端改结构化 payload（ok/message）+ 前端 readTestFinish 双兼容 |
| P1-K | 内核更新检查失败静默 | **已修** | 事件取 data 并 toast/reportError |
| P1-L | 订阅排序不落盘 | **已修** | 新增 Go App.ReorderSubs（落盘 sub_index）+ 前端串行落盘 + 失败回滚拉取 |
| P1-M | 表单校验结果被丢弃 | **已修** | LAN CIDR / Network hosts 校验拦截并提示 |
| P1-N | UWP 反选文案与行为不一致 | **已修** | 统一为「全部不选」语义 + 8 语言文案 |
| P1-O | 更新 chip 使设置页永久落「关于」 | **已修** | 离开设置页复位 targetSettingsView |
| P1-P | 波形自动缩放棘轮 | **已修** | scale 朝 instant 收敛（fallAlpha 生效）+ minScale 下限 |
| P1-Q | 还原备份后快照不刷新 | **已修** | @reload-behavior + 重拉快照 |
| P1-R | 全局 outline:none!important 打死焦点环 | **已修** | 改为仅鼠标态移除焦点环 |
| P1-S | 工具层硬编码中文 | **已修** | 相对时间/错误后缀/剪贴板失败全部走 i18n |
| P1-T | 圆角设置几乎不生效 | **已修** | 90 处 border-radius 接 token（7 → 97 个消费点） |

---

## 3 主控集成阶段额外完成的跨文件修复

| # | 内容 | 文件 |
|---|---|---|
| 1 | 后端新增 ReorderSubs（含服务转发 + 映射表 + 计数测试），仅手工补一行 TS 绑定（ID 由生成器产出，未整目录重生成以保持既有生成物形状） | app.go / services_v3*.go / bindings/coreservice.ts |
| 2 | proxy-test-finished 改结构化 payload 并同步 v3_events 注册 | core/appcore/delay_manager.go、v3_events.go |
| 3 | 订阅排序 UI 恢复 + 串行落盘 + trailing flush + 失败回滚 | Subscriptions.vue |
| 4 | Toast 插入期去重（H-22 后半）+ 主题保存失败回滚与提示（H-17 后半） | store.ts |
| 5 | 波形采样改事件驱动（消除与后端 900ms 推流的拍频），并去掉 TrafficCard 的重复写入口 | trafficWaveState.ts、TrafficCard.vue、App.vue |
| 6 | 关窗路径补 try/catch + 真退出前未保存 YAML 确认（C-02 最后一条漏网路径） | App.vue |
| 7 | 删除 App.vue 死调用 initStore、Rules 死钩子 onActivated、Proxies 不可达 config-changed 监听 | App.vue / Rules.vue / Proxies.vue |
| 8 | 日志级别 reason: 补 warning 别名，避免选 warning 反而看到更多日志 | App.vue |
| 9 | 剪贴板三套实现收敛到 utils/clipboard（App / Diagnostics / Subscriptions 三处接线） | 3 个组件 + utils |
| 10 | 染色滑条改独立常量 GLASS_TINT_MAX | SettingsTheme.vue |
| 11 | border-radius 90 处 token 化（脚本批处理，仅精确单值匹配，计算值不变） | 25 个文件 |

---

## 4 有意后移 / 降级的项（附理由，非遗漏）

| 项 | 处理 | 理由 |
|---|---|---|
| R-A1 抽 useTabSlider（4 份同构滑块逻辑） | **后移** | 纯重构、无用户可见收益，但涉及 4 个页面的布局关键数学（量取/滚动/首次动画抑制）；本项目无法跑 GUI 自动验证，风险收益比不划算。建议后续配合一次可视化回归再做。 |
| R-A2 抽 useAsyncAction（11 处异步三件套） | **后移** | 同上；且各页错误处理语义已有差异（有 3 处不同模板），强行抽象会改文案。 |
| R-B3 11 个设置页样式合并（约 500 行） | **后移** | 纯样式去重，动 12 个文件的 scoped 样式，视觉回归面大；当前已有 .ui-page-* 公共层，收益递减。 |
| U-22⑤ 去掉 font-family 的 !important | **不做** | 确认有 4 处 monospace 依赖该权重，去掉会改变日志/代码块的等宽字体。 |
| U-14 错误环形缓冲 | **按删除处理** | 零读取方；06-contract 建议「并入诊断包」，属功能新增，已列后端需求。 |
| 组件级 alertError 剩余 3 处 | **保留原样** | 文案模板不同（无 i18n key / 正文用原始错误），强套会改文案。 |

---

## 5 遗留的后端需求（已写进各 backend-needs-*.md）

| ID | 需求 | 影响 |
|---|---|---|
| B1 | proxy-test-finished 增加 batchId（或新增 batch-start 事件） | 彻底关闭测速批次竞态（当前残余窗口 ≤600ms，失败可再点一次） |
| B2 | 配置读取失败返回 loadError 字段（而非折成空 groups） | 代理页错误态可从启发式改为确定态 |
| B3 | 保存配置「已落盘但重载失败」返回结构化错误码 | 前端可精确提示「已保存，但重载失败」（现靠文案匹配，后端改文案会退化） |
| B4 | AppBehavior 补丁语义（只写传入字段） | 彻底消除多写者 lost update（当前靠前端读-合并-写，仍有一个异步窗口） |
| B5 | 诊断包并入最近 N 条前端错误 | 目前错误只进 console |
| B6 | 托盘/后端若再次把 i18n key 传给前端渲染，需同步补 key | 本轮删了 tray.* 11 键，校验脚本看不到 Go 侧用法 |

---

## 5.5 提示策略修订（2026-11-18 晚，按用户口径返工）

用户明确产品意图：**无感自愈优先，只有"无法解决"和"严重"的问题才打扰**；测速这类不需要 toast。
本轮修复里有一批"静默失败补提示"改得过头（把可重试的小故障也做成了 toast），据此返工，并沉淀成 `NOTIFICATION_POLICY.md`。

| 场景 | 改动前 | 改动后 |
|---|---|---|
| 批量测速启动失败 / 测速忙 | error toast | **静默**（按钮复位 + 节点格显示超时/失败即是反馈） |
| 单点测速失败 | error toast | **静默**（该节点自己显示失败） |
| 内核更新检查失败 | error toast | **静默**（瞬时问题，按钮自动复位可重试） |
| 主题模式落盘失败 | error toast + 回滚 | **静默**：先自动重试一次，仍失败才回滚，不弹提示 |
| 订阅排序落盘失败 | error toast + 回拉 | **静默**：只回拉（列表弹回去就是反馈） |
| LAN CIDR / Network hosts 非法 | toast + 内联红字（重复） | **静默**：只留内联红字并阻止保存 |

保留（属"会丢东西 / 用户需知情 / 无法继续"）：设置保存失败、用户主动操作失败（导出/复制/打开目录/安装驱动/更新组件）、流量清零失败、各类确认与需装驱动/无配置类模态。
配套清理：3 个因此变成死键的 i18n key 已从 8 语言包删除（死键回到 0），`Proxies.vue` 未用的 `showToast` 导入已移除。

---

## 5.6 结构化契约硬化（2026-11-18 夜，按"最佳建议"执行的第 1 步）

原本列在 backend-needs 里的几项，能自己做完的都做了：

| 项 | 做法 | 落点 |
|---|---|---|
| 保存错误码（原 backend-needs-4 P-30） | Go 侧定义稳定错误码前缀 **CONFIG_SAVED_RELOAD_FAILED**（`core/appcore/config_editor_controller.go`），前端只解析前缀，中文文案匹配降级为旧后端兜底 → 后端改文案不再让判定退化 | config_editor_controller.go、YamlEditor.vue |
| AppBehavior 多写者窗口（原 backend-needs-1 / B4） | 给 `AppBehavior` 加 **Revision`json:"revision"`** 乐观锁：每次落盘 +1；提交版本与服务端不一致 → 返回 **BEHAVIOR_CONFLICT** 且**不落盘不改内存**；revision==0 表示内部重置（跳过校验但仍推进）。前端两处写入口（侧栏 setThemeMode / 设置页 runSaveBehavior）收到冲突后重读、重合并、重试一次 —— 用户无感 | behavior.go、store.ts、Settings.vue、bindings models |
| 代理页读取失败确定态（原 backend-needs-3 P-16） | `GetInitialData` 的两个失败分支带上 **loadError: err.Error()**；前端优先采信它，启发式只作旧后端兜底（顺带修掉"纯规则配置误报读取失败"） | app.go、Proxies.vue |
| 订阅排序（已完成于 Wave 2） | 新增 `App.ReorderSubs` 真正落盘 | 见 §3 |

**新增回归测试**：`core/appcore/behavior_revision_test.go` 4 条（版本单调递增 / 过期提交被拒且不污染 / revision=0 跳过 / 重启后保持）。用 `GOCLASHZ_DATA_DIR` 指向临时目录隔离真实数据，`go test ./core/appcore -run TestBehaviorRevision` 全绿。

### 为什么"顺手做 batchId"最后没做

测速批次这件事重新分析后结论变了：要让前端真正用上 batchId，必须**新增一个 batch-start 事件**（`proxy-test-start` 的 payload 是单节点名字符串，改类型属破坏性变更），否则前端拿不到"这一批的 id"。而当前残余症状只是"停止后立刻重测时按钮可能早弹回 ≤600ms、无数据损失、可再点一次"。为一个纯视觉瑕疵新增一条事件契约 + 注册 + 前端状态机，收益不抵风险 —— 记为**已知残余**，等真出现"测速状态错乱"的用户反馈再补。

### UI 冒烟：CDP 路线在这个技术栈上走不通（实测）

原本计划复刻 `.probe/cdp_*.cjs` 那套（连 WebView2 的 127.0.0.1:9333）。实测结论：
- Wails v3.1.0 创建 WebView2 环境时**显式传入了空的 additionalBrowserArguments**（`internal/webview2/pkg/edge/chromium.go:263`），这会覆盖 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 环境变量 → 远程调试端口起不来（已用沙箱数据目录实测：进程正常启动、9333 无监听）。
- Wails 也**没有**暴露给应用层的 browser args 选项（`AdditionalBrowserArgs` 仅存在于 internal edge 结构体，`pkg/application` 无对应字段）。
- 沙箱验证过程本身是安全的：`GOCLASHZ_DATA_DIR` 指向临时副本、exe 改名 `GoclashZ-dev.exe`（debug 模式跳过单实例）、`restoreOnStartup=false` 所以不会启动内核、进程已清理、真实 user_behavior.json 未被改写（无 revision 字段）。

因此 UI 自动化只剩两条路：(a) 用 `.probe/act.ps1` 的 Win32 坐标点击 + 截图人眼判断（脆弱，但已有先例）；(b) 给 debug 版加一个 `--smoke-script` 钩子（`w.ExecJS` + 回传 localhost）—— 后者能给真正的 DOM/计算样式断言，但要在 main.go 里加调试专用代码。
**结论**：本轮不为此改生产代码；9 条运行时手测仍按 §7 清单执行。

---

## 6 独立评审（2 个只读评审代理，覆盖全部改动）

| 评审 | 范围 | BLOCKER | MAJOR | MINOR |
|---|---|---|---|---|
| reviewer-core | store/App/api/trafficWave/utils/styles/locales/scripts + 全部 Go 改动 | **0** | **1** | 13 |
| reviewer-ui | 全部组件（App.vue 除外）+ manifest | **0** | **1** | 10 |

两个 MAJOR 已修：
1. **Overview.vue 的 `.orb-glow` 动画失效**：U-16 删掉了 style.css 里唯一的全局 `@keyframes pulse`（当时判定"消费方都有同名定义"），但 Overview 仍在用 → 首页"运行中"光晕的呼吸/扩散静默失效。已补 scoped 定义（数值与 TrafficCard 一致）。
2. **ModernNumberInput 的 Esc 并非真取消**：onInput 实时 emit 使 props.modelValue 已是新值，Esc 用它回填等于空操作，且 blur 走 cancel 分支不 clamp → 越界值（如 99999）留在 model，下次任意保存就落盘。已改为 focus 时快照原值、Esc 回退到原值（不发 change）并跳过 blur 提交。

另外按评审建议修的 6 项：
- Rules.vue 删除闸门提前到确认框之前（双击会排两个确认框的时代结束）
- ModernSelect 只对"空值"自动补第一项，不再静默改写真实但未知的值（脏字段污染）
- 删 3 条调试/死代码残留（Subscriptions console.log、SettingsTheme 死导入、TrafficCard 死导入）
- App.vue 补回 `warning` 死别名清理 + 矛盾注释合并；`logLevels.warning` 确认是死键（rankOf 已先行归一化）
- bindings 的 `eventdata.d.ts` 同步 `proxy-test-finished` 类型（生成物与 v3_events.go 对齐）
- api.ts / services_v3_mapping.go / tokens.css 的计数与注释漂移修正、store.ts 裸 setItem 收口

评审同时确认（节选）：模态管理器自洽（activeModal 非空 ⟺ show）、Events.On/unsub 21/21 与 15/15 全配对、删除的 103 个 i18n 键全仓 0 引用、新增 23 键 8 语言占位符一致、`ReorderSubs` 链路 109=31+63+15 且 binding ID 经 FNV-1a 复算与生成器一致、圆角 token 计算值与改前逐像素相同、无未定义 CSS 变量。

---

## 7 运行时手测清单（闸门覆盖不到的部分，按优先级）

1. **主题多写者**：开设置页 → 侧栏切明暗 → 改任意一个开关 → 重进设置页 → 重启应用，确认明暗没有被顶回。
2. **YAML 守卫**：编辑器里改一行 → 点返回 / 点侧栏 / 关窗口（closeToTray=false 时），三处都应只弹一次确认，取消后留在编辑器。
3. **测速**：测速全部 → 期间点任意节点（转圈仍更新）→ 停止 → 600ms 内再测（按钮状态正确）→ 制造失败（例如内核未运行）看是否有明确提示。
4. **订阅排序**：排序模式里上移/下移 → 切页再回来 → 顺序保持；重启后仍保持。
5. **局域网/网络页**：填非法 CIDR 或 hosts → 任意开关保存应被拦住并提示；改回合法后立即恢复。
6. **UWP**：「全部不选」按钮 → 应用 → 确认全部豁免被清空；过滤搜索后再点批量按钮，只作用于过滤结果。
7. **日志页**：设内核级别为 silent → 面板不再刷 info/warn；「全部」级别显示历史行；向上翻看时不被自动拽回底部。
8. **流量波形**：跑一段高速流量后回落，波形应逐步恢复正常幅度（不再永久压平）。
9. **键盘可达性**：Tab 键在主题页的按钮/预设块上应能看到焦点环（本轮修好的 a11y 回归点）。
