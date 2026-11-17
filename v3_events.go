//go:build windows

package main

// 本文件是 v3「强类型事件」的静态声明层（第二轮原生化 #1）。
//
// 它只做一件事：把项目里**已存在**的事件名与 payload 类型登记给 Wails，
// 供 binding 生成器产出 frontend/bindings/.../eventdata.d.ts。
// 不发射任何事件、不改动任何 Emit 调用与 payload、不注册处理器，
// 因此对运行时行为零影响。
//
// v3.1.0 语义（pkg/application/events.go，已核对源码）：
//   - RegisterEvent[Data](name) 在 init() 调用，早于 application.New；
//   - 事件名必须全局唯一（重复注册 panic），且不得与系统事件名（events.*）冲突；
//   - 名字必须是常量、类型必须是具体类型，否则 binding 生成器无法静态发现；
//   - 注册后 EventManager.Emit（本项目所有发射点最终都走 app.Event.Emit）
//     会做**精确**类型校验：Data 的 reflect.Type 与注册类型不一致时，
//     事件会被取消并上抛错误 —— 所以下面每个类型都必须与真实 payload 逐个对齐；
//   - 无 payload 的发射点（Emit(name)，Data == nil）必须注册为 application.Void：
//     Void 分支只允许 nil 数据；若误注册成 any，reflect.TypeOf(nil) 为 nil，
//     校验路径会 panic（事件丢失）。
//
// **动态事件名不注册**（保持现有行为）：
//   - geo-update-<key>-{start,cancelled,error,success,busy}
//     <key> 由 core/appcore/geo_update_manager.go 运行时拼接
//     （geoip / geosite / mmdb / asn），无法静态命名。
//
// 未被任何发射点使用的历史常量（traffic-metrics / update-progress /
// behavior-changed）没有注册，避免凭空扩大对外契约。

import (
	"goclashz/core/appcore"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	// ------------------------------------------------------------------
	// 应用全局状态 / 通知 / 日志 / 启动自检
	// ------------------------------------------------------------------
	// core/appcore/controller.go: SyncState() → state 为 appcore.AppState 值类型
	application.RegisterEvent[appcore.AppState]("app-state-sync")
	// core/appcore/controller.go notify() + tray_runtime_windows.go reportTrayError()
	application.RegisterEvent[appcore.Notification]("app-notification")
	// 注：「notify-error」的注册已删除（F-A5-23）。它由 notify()/reportTrayError 发射，
	// 但前端从未订阅、TypeScript 侧也没有消费者；需要给用户看的内容一律走
	// app-notification（带 level/code/source 的分类通道）。
	// core/appcore/log_dispatcher.go：logger.LogEntry 值类型
	application.RegisterEvent[logger.LogEntry]("log-message")
	// app.go runStartupRuntimeAssetMaintenance()：runtimeassets.MergeStatus(...) 的返回值
	application.RegisterEvent[runtimeassets.RuntimeAssetStatus]("runtime-assets-status")

	// ------------------------------------------------------------------
	// 内核 / 流量 / 代理组 / 连接
	// ------------------------------------------------------------------
	// core/appcore/streams.go：appcore.TrafficSnapshot（6 处发射点，类型一致）
	application.RegisterEvent[appcore.TrafficSnapshot]("traffic-data")
	// controller.go SaveAppBehavior：bool(next.ProxyTrafficOnly)
	application.RegisterEvent[bool]("traffic-stat-mode-changed")
	// controller.go 内核退出回调：e.Message (string)
	application.RegisterEvent[string]("clash-exited")
	// controller.go clash.SetMipsFallbackNotifier：Emit("tun-mips-fallback") 无数据
	application.RegisterEvent[application.Void]("tun-mips-fallback")
	// controller.go RestartCoreWithReason：map[string]any{"reason": reason}
	application.RegisterEvent[map[string]any]("core-restarted")
	// 4 处发射点（controller.go×2 / config_editor_controller.go / updater_orchestrator.go）均为 string
	application.RegisterEvent[string]("delay-cache-clear")
	// core/appcore/proxy_state.go×2：getProxyGroupStates() 返回 []ProxyGroupState
	application.RegisterEvent[[]appcore.ProxyGroupState]("proxy-state-sync")
	// core/appcore/connections.go：ConnectionMonitorManager.GetSnapshot() 返回 ConnectionsSnapshot
	application.RegisterEvent[appcore.ConnectionsSnapshot]("connections-update")
	// core/appcore/delay_manager.go：节点名字符串
	application.RegisterEvent[string]("proxy-test-start")
	// delay_manager.go：map[string]interface{}{current,total}（== map[string]any）
	application.RegisterEvent[map[string]any]("proxy-test-progress")
	// delay_manager.go×2："测速完成" / ErrDelayTestBusy.Error()，均为 string
	application.RegisterEvent[string]("proxy-test-finished")
	// delay_manager.go×2：map[string]interface{}{name,delay,status,message,source[,from]}
	application.RegisterEvent[map[string]any]("proxy-delay-update")

	// ------------------------------------------------------------------
	// 配置 / 开机自启任务
	// ------------------------------------------------------------------
	// config_editor_controller.go：id (string)
	application.RegisterEvent[string]("config-changed")
	// config_editor_controller.go：map[string]any{"id": id}
	application.RegisterEvent[map[string]any]("config-file-saved")
	// controller.go syncStartupTaskStateSafe：err.Error() (string)
	application.RegisterEvent[string]("startup-task-check-warning")
	// controller.go syncStartupTaskStateSafe：info.LastError (string)
	application.RegisterEvent[string]("startup-task-unhealthy")

	// ------------------------------------------------------------------
	// 组件更新任务快照（内核 / 驱动 / geo 共用）
	// ------------------------------------------------------------------
	// component_update_state.go×3：ComponentUpdateTaskStore.snapshotLocked() 返回 []UpdateTaskState
	application.RegisterEvent[[]appcore.UpdateTaskState]("component-update-tasks-sync")

	// ------------------------------------------------------------------
	// 内核 / 驱动组件更新（updater_orchestrator.go、component_update_service.go）
	// ------------------------------------------------------------------
	application.RegisterEvent[map[string]string]("core-version-updated")
	application.RegisterEvent[map[string]string]("core-update-none")
	application.RegisterEvent[map[string]string]("core-update-available")
	application.RegisterEvent[map[string]string]("wintun-version-updated")

	// ------------------------------------------------------------------
	// 软件本体更新（updater_orchestrator.go runAppUpdateCheck / downloadAppUpdateWithInfo）
	// ------------------------------------------------------------------
	// Emit("app-update-check-start") / defer Emit("app-update-check-end") / Emit("app-update-check-cancelled")
	application.RegisterEvent[application.Void]("app-update-check-start")
	application.RegisterEvent[application.Void]("app-update-check-end")
	application.RegisterEvent[application.Void]("app-update-check-cancelled")
	// 4 处发射点全部是 string（含 userFacingAppUpdateDownloadError 的返回值）
	application.RegisterEvent[string]("app-update-error")
	// map[string]string{"message": ...}
	application.RegisterEvent[map[string]string]("app-update-none")
	// map[string]any{"version": ..., "manual": ...} —— 注意与 app-update-none 的类型不同
	application.RegisterEvent[map[string]any]("app-update-available")
	// Emit("app-update-busy") 无数据
	application.RegisterEvent[application.Void]("app-update-busy")
	// map[string]string{"version": ...}
	application.RegisterEvent[map[string]string]("app-update-start")
	// map[string]interface{}{bytesDone,totalBytes,speedBps,etaSec}
	application.RegisterEvent[map[string]any]("app-update-progress")
	// map[string]string{"version","path"}
	application.RegisterEvent[map[string]string]("app-update-downloaded")

	// ------------------------------------------------------------------
	// Geo 数据库更新（core/appcore/geo_update_manager.go）
	// ------------------------------------------------------------------
	// emitActiveSnapshot()：activeKeysLocked() 返回 []string
	application.RegisterEvent[[]string]("geo-update-active-sync")
	// UpdateAllAsync()：Emit("geo-update-all-start") / Emit("geo-update-all-success") 无数据
	application.RegisterEvent[application.Void]("geo-update-all-start")
	application.RegisterEvent[application.Void]("geo-update-all-success")
	// UpdateAllAsync()：strings.Join(failed, "; ") (string)
	application.RegisterEvent[string]("geo-update-all-error")

	// ------------------------------------------------------------------
	// 任务框架派生事件（core/tasks/manager.go：Emit(任务名 + "-start"/"-error"/...）
	//
	// 任务名虽然是 Emit 处的变量，但取值在源码里都是编译期常量，
	// 因此这些事件名同样是静态的，一并注册以便前端拿到类型。
	// 约定：-start/-success/-cancelled 无数据（application.Void）；
	//       -error 携带 err.Error() (string)。
	// autoSuccess=false 的任务不会发 -success，故不注册该名。
	// ------------------------------------------------------------------

	// core-update-check：updater_orchestrator.go CheckCoreUpdateAsync（autoSuccess=true）
	application.RegisterEvent[application.Void]("core-update-check-start")
	application.RegisterEvent[application.Void]("core-update-check-success")
	application.RegisterEvent[string]("core-update-check-error")
	application.RegisterEvent[application.Void]("core-update-check-cancelled")

	// core-update：component_update_service.go runComponentUpdateTransaction（autoSuccess=true）
	// 注意：-start/-error/-success/-cancelled 现在**只由任务框架发一次**；
	// 事务内部原先的重复发射已删除（F-A5-23：同名双发 + 类型漂移会导致事件被 cancel）。
	application.RegisterEvent[application.Void]("core-update-start")
	application.RegisterEvent[application.Void]("core-update-success")
	application.RegisterEvent[string]("core-update-error")
	application.RegisterEvent[application.Void]("core-update-cancelled")

	// driver-install（Wintun 重装，同上）
	application.RegisterEvent[application.Void]("driver-install-start")
	application.RegisterEvent[application.Void]("driver-install-success")
	application.RegisterEvent[string]("driver-install-error")
	application.RegisterEvent[application.Void]("driver-install-cancelled")

	// subs-update：core/appcore/subscriptions.go UpdateAllSubsAsync（autoSuccess=true）
	application.RegisterEvent[application.Void]("subs-update-start")
	application.RegisterEvent[application.Void]("subs-update-success")
	application.RegisterEvent[string]("subs-update-error")
	application.RegisterEvent[application.Void]("subs-update-cancelled")

	// app-update-check-job / app-update-download-job：内部调度任务名
	// （updater_orchestrator.go 顶部常量，autoSuccess=false，前端刻意不消费）
	application.RegisterEvent[application.Void]("app-update-check-job-start")
	application.RegisterEvent[string]("app-update-check-job-error")
	application.RegisterEvent[application.Void]("app-update-check-job-cancelled")
	application.RegisterEvent[application.Void]("app-update-download-job-start")
	application.RegisterEvent[string]("app-update-download-job-error")
	application.RegisterEvent[application.Void]("app-update-download-job-cancelled")
}
