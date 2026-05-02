package appcore

import (
	"context"
	"goclashz/core/clash"
	"sync"
	"time"
)

type DelayTestManager struct {
	mu      sync.Mutex
	running bool
	emit    EventSink
	ctrl    *Controller // 引用总控，用于启停内核
}

func NewDelayTestManager(emit EventSink, ctrl *Controller) *DelayTestManager {
	return &DelayTestManager{
		emit: emit,
		ctrl: ctrl,
	}
}

func (m *DelayTestManager) TestAllProxies(ctx context.Context, nodeNames []string) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()

		// 🛡️ 核心修复：忙碌时也要对请求的节点发通知，防止前端一直转圈
		for _, name := range nodeNames {
			m.emit.Emit("proxy-delay-update", map[string]interface{}{
				"name":   name,
				"delay":  0,
				"status": "busy",
			})
		}
		m.emit.Emit("proxy-test-finished", "当前已有测速任务正在运行")
		return
	}
	m.running = true
	m.mu.Unlock()

	finishMsg := "测速完成"
	silentCore := false

	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		if silentCore {
			m.ctrl.StopCoreProcess()
		}
		m.emit.Emit("proxy-test-finished", finishMsg)
	}()

	// 1. 如果内核未运行，由 m.ctrl.EnsureCoreRunning 静默拉起
	if !clash.IsRunning() {
		silentCore = true
		if err := m.ctrl.EnsureCoreRunning(ctx); err != nil {
			finishMsg = "测速启动失败：" + err.Error()
			return
		}
	}

	// 2. 如果未传节点，先从内核获取并补全
	if len(nodeNames) == 0 {
		if data, err := clash.GetInitialData(); err == nil {
			// 🛡️ 核心修复：获取节点的 key 必须是 "groups"，且类型是 map[string]interface{}
			if groups, ok := data["groups"].(map[string]interface{}); ok {
				for name, raw := range groups {
					nm, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					typ, _ := nm["type"].(string)

					// 排除策略组和内置节点，只测真实节点
					switch typ {
					case "Selector", "URLTest", "Fallback", "LoadBalance":
						continue
					}
					if name == "GLOBAL" || name == "DIRECT" || name == "REJECT" {
						continue
					}
					nodeNames = append(nodeNames, name)
				}
			}
		}
	}

	if len(nodeNames) == 0 {
		finishMsg = "没有可测速节点"
		return
	}

	testUrl := "http://www.gstatic.com/generate_204"
	// 获取用户自定义测速链接
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
		testUrl = netCfg.TestURL
	}

	// 3. 启动 Worker Pool 测速
	jobs := make(chan string, len(nodeNames))
	for _, name := range nodeNames {
		jobs <- name
	}
	close(jobs)

	workerCount := 32
	if len(nodeNames) < workerCount {
		workerCount = len(nodeNames)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				m.emit.Emit("proxy-test-start", name)

				reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				delay, err := clash.GetProxyDelay(reqCtx, name, testUrl)
				cancel()

				status := "success"
				if err != nil || delay <= 0 {
					status = "timeout"
					delay = 0
				}

				m.emit.Emit("proxy-delay-update", map[string]interface{}{
					"name":   name,
					"delay":  delay,
					"status": status,
				})
			}
		}()
	}
	wg.Wait()
}

func (m *DelayTestManager) TestProxy(name string) (int, error) {
	return clash.TestProxy(name)
}
