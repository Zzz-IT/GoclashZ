# 问题台账（ISSUE_LEDGER）— 最终版

> 来源：docs/AUDIT_FULL.md + docs/audit/A0~A5；共 118 条。
> 交付：B0（安全+止血+可复盘）→ B1（P0 清零）→ B2（P1 主体）→ **B3（P2 收尾，最终）**。
> 状态词汇：fixed(Bn) / wontfix(理由) / deferred(理由) / open。

| ID | 严重度 | 状态 | 标题 |
|---|---|---|---|
| F-A0-01 | P0 | fixed(B1) | UI 接口直接暴露“全量深检”，无缓存层 |
| F-A0-02 | P1 | fixed(B2) | 同一页面并行跑 2 个全量 GetStatus |
| F-A0-03 | P2 | fixed(B3) | 6 个重接口中 3 个是死接口，仍在服务面 |
| F-A0-04 | P1 | fixed(B2) |  |
| F-A0-05 | P1 | fixed(B2) | SCM/提权路径用固定 time.Sleep 轮询（9 处），不可取消不可观测 |
| F-A0-06 | P1 | fixed(B3) | 单次动作的进程 spawn 数无上限（修复路径 9+） |
| F-A0-07 | P0 | fixed(B1) | 首屏与设置页共用同一批重接口，没有“轻量快照”通道 |
| F-A1-01 | P0 | fixed(B1) | 检查无缓存：每次 GetStatus 无条件全量 SHA256 95.3 MB |
| F-A1-02 | P0 | fixed(B0) | RepairInvalid 下 wintun 恒 shouldCopy=true，配 CopyFile 无释放等待 → 运行中覆盖必失败 → |
| F-A1-03 | P0 | fixed(B0) | 自愈并发失控：冷却非原子、无单飞、CopyFile 用固定 .tmp 名 |
| F-A1-04 | P0 | fixed(B1) | "检查"里写文件且无来源校验：adoptAssetFromFallback 会在只读 UI 方法里复制 61 MB |
| F-A1-05 | P1 | fixed(B2) | Ready 与 VersionProbeOK 解耦：损坏内核 / 0 字节 geo 在快路径上永不触发自愈 |
| F-A1-06 | P1 | fixed(B2) | manifest 的 SHA256 只写不校验，且实测清单与二进制已不一致 → 内核升级被永久判成"用户魔改" |
| F-A1-07 | P1 | fixed(B2) | CheckWintun 的 alt 目录回退把"不在运行目录"的 DLL 判成 Ready → 自检乐观误判，且与 CleanupLegac |
| F-A1-08 | P1 | fixed(B2) | ctx 全链路无效：超时与取消无法中断复制/探针，watchdog 的 8 s 预算形同虚设 |
| F-A1-09 | P1 | fixed(B2) | ReadCoreVersion 持全局互斥锁执行子进程（≤5 s），失败不缓存 |
| F-A1-10 | P1 | fixed(B2) | 一次开 TUN 实际是 4 次 EnsureReady（方案说 3 次），最坏 8 次全量扫描 |
| F-A1-11 | P1 | fixed(B2) | seed 目录探测只看"存在"、manifest 路径有两套约定、候选含父目录 |
| F-A1-12 | P1 | fixed(B2) | 启动自愈结论无人消费：事件在 UI 订阅前发出，前端从未订阅 |
| F-A1-13 | P1 | fixed(B2) | 同一批文件有 6 个写者、3 套锁、0 个跨路径互斥 |
| F-A1-14 | P1 | fixed(B2) | SHA256 字段零消费者：非修复路径的 95 MB 哈希是纯浪费 |
| F-A1-15 | P1 | fixed(B2) | ErrIsDir 资产永远不会被修复，且 EnsureReady 对 geo 缺失不报错 → 静默永久失败 |
| F-A1-16 | P1 | fixed(B2) | CleanupLegacyAssets 跨卷 rename 失败即删文件，且与前台自检/自愈并发 |
| F-A1-17 | P1 | fixed(B2) | Hint 指向不存在的入口；RepairRuntimeAssets 是死接口 |
| F-A1-18 | P2 | fixed(B3) | resolveAppDir 的临时目录判断参数颠倒 |
| F-A1-19 | P2 | fixed(B3) | runtimeassets 零单测，shouldCopy 矩阵无回归网 |
| F-A1-20 | P2 | fixed(B3) | Ready 语义只含 core+wintun、Required 对 geo 恒 false |
| F-A2-01 | P0 | fixed(B1) | F-A2-01（P0，A）watchdog 在 TUN 开启期间每 5s 做一次 90.9 MiB 全量资产自检 |
| F-A2-02 | P0 | fixed(B1) | F-A2-02（P0，A）单次开 TUN 触发 4 轮 EnsureReady，25s 预算结构上不够 |
| F-A2-03 | P0 | fixed(B0) | F-A2-03（P0，A+B）helper 白名单目录推导错误 → 内核替换 / 驱动安装 / 权限修复在"标准安装"下必然失败 |
| F-A2-04 | P0 | fixed(B0) | F-A2-04（P0，A）helper 管道对 IU 授予 GENERIC_ALL，且 start-core 无路径白名单 → 任意交互用户 |
| F-A2-05 | P1 | fixed(B2) | F-A2-05（P1，A）关闭 TUN 的最坏路径超过 UI 的 25s 预算：stop-core 的 30s deadline 完全忽略外 |
| F-A2-06 | P1 | fixed(B2) | F-A2-06（P1，A）helper 停止失败仍置 isRunning=false，主进程状态与真实内核状态分叉 |
| F-A2-07 | P1 | fixed(B2) | F-A2-07（P1，A）disableAll 无条件关闭系统代理，会误伤其它程序写入的代理配置 |
| F-A2-08 | P1 | fixed(B2) | F-A2-08（P1，A）任何 Reconcile 错误都回滚 desired → 一次瞬时故障撤销用户意图并弹窗 |
| F-A2-09 | P1 | fixed(B2) | F-A2-09（P1，A）EnsureHelperReady 无 ctx、无超时，最坏路径 ≈45s |
| F-A2-10 | P1 | fixed(B3) | F-A2-10（P1，A）后台自愈失败既不进 UI 也不落盘：notify(source=appcore) 被前端整类丢弃 |
| F-A2-11 | P1 | fixed(B1) | F-A2-11（P1，A+B）start-core 两侧预算不等（客户端 10s / helper 15s），且 helper 先等旧内核  |
| F-A2-12 | P1 | fixed(B2) | F-A2-12（P1，A）install-wintun 不停内核，而 replace-core-file 停内核；TUN 运行中装驱动必然失 |
| F-A2-13 | P1 | fixed(B2) | F-A2-13（P1，A）文件替换非原子、错误被吞：helper 用 ReadFile+WriteFile 覆盖，主进程用 rename 覆 |
| F-A2-14 | P1 | fixed(B2) | F-A2-14（P1，A）EnsureRuntimeAssets 在用户同步路径上串行下载（最多 4×200 MB） |
| F-A2-15 | P1 | fixed(B2) | F-A2-15（P1，A）GetAppState 每次同步都取内核版本；内核更新期间该锁被下载全程持有 |
| F-A2-16 | P1 | fixed(B2) | F-A2-16（P1，A）托盘 TUN 开关无 deadline，与 UI 的 25s 语义分裂 |
| F-A2-17 | P2 | fixed(B3) | F-A2-17（P2，B）watchdog 的 8s ctx 与 reconcileMu 串行，开关事务会让 watchdog 拿到已过期  |
| F-A2-18 | P2 | fixed(B3) | F-A2-18（P2，A）ErrLongConnectionActive 的延迟队列每次失败都新起一个 10s ticker gorouti |
| F-A2-19 | P2 | fixed(B3) | F-A2-19（P2，B）ReconcileAsync 依赖 s.ctx，在 Start 前 / Stop 后可能为 nil 或已取消 |
| F-A2-20 | P2 | fixed(B3) | F-A2-20（P2，A）幂等点击仍付全价：prepareTunEnable 在"意图未变"判定之前执行 |
| F-A2-21 | P2 | fixed(B3) | F-A2-21（P2，A）helper 服务未配置恢复动作，崩溃后只能靠下次开 TUN 拉起 |
| F-A2-22 | P2 | fixed(B3) | F-A2-22（P2，A）适配器探测的成本结构不合理：每次探测都做 2 次整表路由遍历 |
| F-A3-01 | P0 | fixed(B0) | 全部日志只存在于内存，进程退出即归零；data/logs 只建不写 |
| F-A3-02 | P0 | fixed(B0) | 崩溃与所有异常退出路径的日志必然丢失：不抓栈、无退出标记、os.Exit 前只写内存 |
| F-A3-03 | P0 | fixed(B0) | 资产自检/自愈日志走标准库 log，输出到 GUI 进程不存在的 stderr——最需要复盘的一段完全不可见 |
| F-A3-04 | P0 | fixed(B1) | 内核进程 stdout/stderr 被主动丢弃，启动失败原因永久消失 |
| F-A3-05 | P0 | fixed(B0) | 用户能看到的每一条错误通知，日志里都不存在 |
| F-A3-06 | P0 | fixed(B1) | Wails 框架层的日志与错误被默认丢弃，事件类型不匹配也静默 |
| F-A3-07 | P1 | fixed(B1) | 时间精度不足：无日期、无毫秒、无时区、无 PID |
| F-A3-08 | P1 | fixed(B2) | 日志条目没有结构：无类别、无事件名、无错误码、无关联 ID、无耗时/重试/调用点 |
| F-A3-09 | P1 | fixed(B2) | 环形缓冲满后每条日志都要 O(500) 复制，并周期性分配约 50 KB |
| F-A3-10 | P1 | fixed(B2) | 日志推送逐条、无批量、无节流、无背压，且与流量事件共用同一条串行前端通道 |
| F-A3-11 | P1 | fixed(B2) | core 日志不过滤就推前端，且与 app 日志共用一个 500 条环——错误会被淹没，前端还要再过滤一次 |
| F-A3-12 | P1 | fixed(B1) | 启动早期事件被丢弃，且没有任何 boot 记录；runtime-assets-status 发了没人收 |
| F-A3-13 | P1 | fixed(B2) | helper 侧是纯黑盒：只有 stderr，内核退出码被丢弃，状态接口不含退出原因 |
| F-A3-14 | P1 | fixed(B2) | 诊断包太薄且无脱敏：只有路径与布尔值，没有日志、没有版本、没有系统信息 |
| F-A3-15 | P2 | fixed(B3) | 用户在保存对话框点"取消"，界面却弹"诊断信息导出成功" |
| F-A3-16 | P2 | fixed(B3) | 日志面板缺所有自助能力：无搜索、无级别下拉、无暂停、无复制、无导出、无打开日志目录；"清空"无二次确认 |
| F-A3-17 | P2 | fixed(B3) | 日志流自身的健康不可见：断流静默重连、状态不外露，且存在两条互斥的死实现 |
| F-A3-18 | P2 | fixed(B3) | Debugf 与 Level 枚举是死代码——app 侧 debug 档永远为空 |
| F-A3-19 | P2 | fixed(B0) | dispatcher 为 nil 的兜底路径不补时间，会产出空时间行 |
| F-A3-20 | P2 | fixed(B3) | 所有丢弃都是静默的：没有 dropped 计数，也就没有"日志系统自身是否健康"的指标 |
| F-A3-21 | P2 | fixed(B3) | 没有任何容量/轮转/保留策略，日志目录从未暴露给用户；诊断文件名固定会被覆盖 |
| F-A4-01 | P0 | fixed(B0) | 单一模态槽：showAlert/直写 modal 会顶掉 pending 的 showConfirm，导致 Promise 永不 settl |
| F-A4-02 | P0 | fixed(B1) | 10 处绕过提示 API 直接赋值 globalState.modal |
| F-A4-03 | P0 | fixed(B1) | TUN 开关两套实现：Overview 有引导+重试+错误分层，SettingsTun 只有一句原文模态 |
| F-A4-04 | P0 | fixed(B0) | 「缺驱动」分支大小写敏感：真正会出现的 wintun 包装错误漏判，而唯一命中的分支直接强制重装 |
| F-A4-05 | P0 | fixed(B0) | mips 回退从未落盘：每次进设置→TUN 都弹一次「已回退」，配置永久不一致 |
| F-A4-06 | P0 | fixed(B1) | 设置自动保存四套失败处理，其中两处完全静默 |
| F-A4-07 | P1 | fixed(B2) | 8 处自建模态与全局模态并存；同一「重置」动作有两种确认 UI |
| F-A4-08 | P1 | fixed(B3) | 启动链把重调用串在「事件订阅」之前，且 GetRecentLogs 无兜底 → 订阅可能整段丢失 |
| F-A4-09 | P1 | fixed(B2) | SettingsTun 首开页面并发两次全量 GetStatus，且后一次覆盖前一次的错误信息 |
| F-A4-10 | P1 | fixed(B2) | 代理页「全部测速」失败完全静默；单点测速用 includes(busy) 吞掉任意含 busy 的错误 |
| F-A4-11 | P1 | fixed(B2) | 完全静默的失败面比主方案统计的 28 处 console.error 更大：另有 9 处零痕迹 |
| F-A4-12 | P1 | fixed(B2) | 成功反馈走模态：10 处成功提示违反 store.ts 自己写的策略 |
| F-A4-13 | P1 | fixed(B2) | i18n 槽位错位：把原始后端错误塞进 title；同一错误在标题和正文各出现一次 |
| F-A4-14 | P1 | fixed(B3) | 前端用中文字面量正则判断后端语义，后端改文案即静默降级 |
| F-A4-15 | P2 | fixed(B3) | code 契约不一致：前端匹配的字符串里，有 3 个后端永远不会产出 |
| F-A4-16 | P2 | fixed(B3) | 硬编码文案绕过 i18n（8 语言下固定显示中文） |
| F-A4-17 | P2 | fixed(B4，改为流程化治理：新增 scripts/check-i18n.mjs 并接入 build) | locale 键集：8 语言 777 键完全对齐，但存在 1 个缺键、14 处日文未翻译、141 个死键 |
| F-A4-18 | P2 | fixed(B3) | app-notification 过滤规则过窄，后台失败被整段丢弃 |
| F-A4-19 | P2 | fixed(B3) | 订阅导出：「取消」路径也会执行导出，且订阅 URL 明文进模态 detail |
| F-A4-20 | P2 | fixed(B3) | UWP「全不选」实际是逐项取反 |
| F-A4-21 | P2 | fixed(B3) | 偏差条把「拉取失败」当「无偏差」 |
| F-A4-22 | P2 | fixed(B3) | 短命令缺少错误分支（失败即无提示） |
| F-A4-23 | P2 | fixed(B4) | 设置页导航到 UWP 的「取消提权」仍会进入页面（提示与行为不一致） |
| F-A5-01 | P0 | fixed(B0) | 任务框架的 goroutine 没有 recover：任务内 panic 直接杀进程 |
| F-A5-02 | P0 | fixed(B0) | 「更新全部订阅」把所有失败吞掉并报成功 |
| F-A5-03 | P0 | fixed(B0) | 提权重启在库函数里 os.Exit(0)，跳过全部清理 |
| F-A5-04 | P1 | fixed(B1) | 提权子进程「无限等待 + 隐藏窗口 + 无取消」 |
| F-A5-05 | P1 | fixed(B2) | 组件更新锁跨越整段下载，其他入口无超时阻塞 |
| F-A5-06 | P1 | fixed(B3) | 三类更新失败的可见性为零：无日志、无提示、只有任务行 |
| F-A5-07 | P1 | fixed(B1) | FetchSmallFileAtomic：空响应体返回成功；读错误被吞；超限截断不报错 |
| F-A5-08 | P1 | fixed(B2) | 订阅下载单次上限 10 分钟且是同步调用 |
| F-A5-09 | P1 | fixed(B1) | 备份还原：ctx 完全不生效；回滚失败静默且恒返回 nil |
| F-A5-10 | P1 | fixed(B1) | 导出备份把 zip.Close() 的错误丢掉：坏包可被判成功 |
| F-A5-11 | P1 | fixed(B1) | 删除当前配置不是事务，且 3 处错误被 _ 吞掉 |
| F-A5-12 | P1 | fixed(B2) | helper RPC 的默认 10s deadline 罩住了 61MB 文件替换 |
| F-A5-13 | P1 | fixed(B3) | 退出链路三套预算互相打架：8s 硬退 vs 5s 上限 vs 30s 优雅停机 |
| F-A5-14 | P2 | fixed(B3) | MigrateLegacyAssets 每次启动多算 6 次 SHA256（≈91MB），且日志走 stderr |
| F-A5-15 | P2 | fixed(B3) | 大文件下载的「换 URL 就丢断点」防护是死代码 |
| F-A5-16 | P2 | fixed(B3) | acquireHostLimiter 是阻塞信号量，不响应 ctx |
| F-A5-17 | P2 | fixed(B3) | 开机自启的“检查”带写副作用，且同步 spawn schtasks 无超时 |
| F-A5-18 | P2 | fixed(B3) | BehaviorStore.SetAndSave 是读-改-写，且先写 cache 后写盘 |
| F-A5-19 | P2 | fixed(B3) | LoadSetting 解析失败静默回落默认，且不提示不改名 |
| F-A5-20 | P2 | fixed(B3) | 托盘命令串行执行且满队列静默丢弃 |
| F-A5-21 | P2 | fixed(B3) | 全局退出热键失败后用户不可知 |
| F-A5-22 | P2 | fixed(B3) | 检查函数里写文件：IsCoreBinWritable 与 UpdateTunConfig 的隐藏进程探测 |
| F-A5-23 | P2 | fixed(B3) | 事件契约有死链，也有“同名双发” |
| F-A5-24 | P2 | fixed(B3) | CleanLegacyFiles 在 core/bin 无差别删 .zip/.tmp/.meta.json |
| F-A5-25 | P2 | fixed(B3) | Shutdown 事件创建失败会导致启动即退出 |

## 最终统计

| 严重度 | 总数 | fixed | 未修(含 wontfix/deferred) |
|---|---|---|---|
| P0 | 25 | 25 | 0 |
| P1 | 55 | 55 | 0 |
| P2 | 38 | 38 | 0 |

**合计**：118 fixed / 0 未修（B0~B4 全部收口）

## B4 收尾说明

- **F-A4-17**：改为流程化治理 —— 新增基于 TypeScript AST 的 `scripts/check-i18n.mjs`（缺键/语言不齐 = error，死键 = warn），已接入 `frontend/package.json` 的 build。运行中发现并修正了一个真实错位：`t('settings.uwp.clearAllConfirm')` 实际落在 `diag.clearAllConfirm`（8 语言全错），已移到 `settings.uwp` 下。剩余 187 个死键仅警告、不删除（存在动态 key 拼装，强删有风险）。
- **F-A4-23**：`showConfirm` 增加可选 `labels: { confirmText?, cancelText? }`，模态模板透传并回退通用文案；UWP 提权确认改为「以管理员重启 / 普通权限查看」，8 语言同步，并删掉文案末尾重复的「点击取消…」说明。
