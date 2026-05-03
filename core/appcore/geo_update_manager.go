//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/downloader"
	"strings"
	"sync"
)

const GeoUpdateConcurrency = 4

type GeoResult struct {
	Key string
	Err error
}

type geoJob struct {
	waiters []chan GeoResult
}

type GeoUpdateManager struct {
	mu        sync.Mutex
	active    map[string]*geoJob
	sem       chan struct{}
	emit      EventSink
	updateOne func(ctx context.Context, key string) error
}

func NewGeoUpdateManager(
	emit EventSink,
	updateOne func(ctx context.Context, key string) error,
) *GeoUpdateManager {
	return &GeoUpdateManager{
		active:    make(map[string]*geoJob),
		sem:       make(chan struct{}, GeoUpdateConcurrency),
		emit:      emit,
		updateOne: updateOne,
	}
}

func isGeoKey(key string) bool {
	switch key {
	case "geoip", "geosite", "mmdb", "asn":
		return true
	default:
		return false
	}
}

func (m *GeoUpdateManager) beginOrJoin(key string) (wait <-chan GeoResult, owner bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if job, ok := m.active[key]; ok {
		ch := make(chan GeoResult, 1)
		job.waiters = append(job.waiters, ch)
		return ch, false
	}

	m.active[key] = &geoJob{}
	return nil, true
}

func (m *GeoUpdateManager) finish(key string, result GeoResult) {
	m.mu.Lock()

	job := m.active[key]
	delete(m.active, key)

	var waiters []chan GeoResult
	if job != nil && len(job.waiters) > 0 {
		waiters = append(waiters, job.waiters...)
	}

	m.mu.Unlock()

	m.emitActiveSnapshot()

	for _, ch := range waiters {
		ch <- result
		close(ch)
	}
}

func (m *GeoUpdateManager) activeKeysLocked() []string {
	active := make([]string, 0, len(m.active))
	for key := range m.active {
		active = append(active, key)
	}
	return active
}

func (m *GeoUpdateManager) ActiveKeys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeKeysLocked()
}

func (m *GeoUpdateManager) emitActiveSnapshot() {
	m.mu.Lock()
	active := m.activeKeysLocked()
	m.mu.Unlock()

	m.emit.Emit("geo-update-active-sync", active)
}

func (m *GeoUpdateManager) runKey(ctx context.Context, key string) GeoResult {
	m.emit.Emit("geo-update-" + key + "-start")

	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		result := GeoResult{Key: key, Err: ctx.Err()}
		m.emit.Emit("geo-update-" + key + "-cancelled")
		return result
	}

	err := m.updateOne(ctx, key)
	if err != nil {
		err = downloader.SanitizeDownloadError(err)
	}
	result := GeoResult{Key: key, Err: err}

	if err != nil {
		m.emit.Emit("geo-update-"+key+"-error", err.Error())
	} else {
		m.emit.Emit("geo-update-" + key + "-success")
	}

	return result
}

func (m *GeoUpdateManager) UpdateOneAsync(ctx context.Context, key string) {
	if !isGeoKey(key) {
		m.emit.Emit("geo-update-"+key+"-error", "未知库类型: "+key)
		return
	}

	wait, owner := m.beginOrJoin(key)

	// beginOrJoin 后再同步真实 active 状态
	m.emitActiveSnapshot()

	if !owner {
		m.emit.Emit("geo-update-" + key + "-busy")

		// 后台等待结果，但不重复发 success/error，避免前端重复提示
		go func() {
			select {
			case <-wait:
			case <-ctx.Done():
			}
		}()

		return
	}

	go func() {
		result := m.runKey(ctx, key)
		m.finish(key, result)
	}()
}

func (m *GeoUpdateManager) UpdateAllAsync(ctx context.Context) {
	go func() {
		keys := []string{"geoip", "geosite", "mmdb", "asn"}

		m.emit.Emit("geo-update-all-start")

		var wg sync.WaitGroup
		var mu sync.Mutex
		var failed []string

		for _, key := range keys {
			key := key

			wait, owner := m.beginOrJoin(key)

			// beginOrJoin 后再同步真实 active 状态
			m.emitActiveSnapshot()

			if !owner {
				// 已经有单项任务在跑，等待它的结果，避免重复下载
				wg.Add(1)
				go func() {
					defer wg.Done()

					select {
					case res := <-wait:
						if res.Err != nil {
							mu.Lock()
							failed = append(failed, fmt.Sprintf("%s: %v", key, res.Err))
							mu.Unlock()
						}
					case <-ctx.Done():
						mu.Lock()
						failed = append(failed, fmt.Sprintf("%s: %v", key, ctx.Err()))
						mu.Unlock()
					}
				}()

				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()

				result := m.runKey(ctx, key)
				m.finish(key, result)

				if result.Err != nil {
					mu.Lock()
					failed = append(failed, fmt.Sprintf("%s: %v", key, result.Err))
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		m.emitActiveSnapshot()

		if len(failed) > 0 {
			m.emit.Emit("geo-update-all-error", strings.Join(failed, "; "))
			return
		}

		m.emit.Emit("geo-update-all-success")
	}()
}
