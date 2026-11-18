//go:build windows

package appcore

import (
	"strings"
	"testing"
)

// 乐观锁（revision CAS）的回归测试。
//
// 背景：SaveAppBehavior 是「整对象覆盖」，写它的入口有多个（设置页 / 侧栏明暗 / 托盘 /
// 启动期自启同步）。各入口都是「GET → 改自己的字段 → SAVE」的读改写，两个读改写交错时
// 会互相覆盖（A GET、B GET、A SAVE、B SAVE → A 的改动丢失）。BehaviorStore 用 revision
// 做 CAS 拦这种情况：版本不一致就拒绝，让前端重读重合并（无感重试）。
//
// 这些测试锁住三件事：版本单调递增、过期版本被拒且不污染内存、revision==0 的
// 内部调用（重置等）仍然允许通过。

// newIsolatedBehaviorStore 把数据目录指向临时目录，避免碰到真实 user_behavior.json。
// GetDataDir 支持 GOCLASHZ_DATA_DIR 覆盖（core/utils/paths.go）。
func newIsolatedBehaviorStore(t *testing.T) *BehaviorStore {
	t.Helper()
	t.Setenv("GOCLASHZ_DATA_DIR", t.TempDir())
	return NewBehaviorStore()
}

func TestBehaviorRevisionIncrementsOnEverySave(t *testing.T) {
	store := newIsolatedBehaviorStore(t)

	base := store.Get()
	start := base.Revision

	next := store.Get()
	next.LogLevel = "info"
	if err := store.SetAndSave(next); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	if got := store.Get().Revision; got != start+1 {
		t.Fatalf("保存后 revision 应为 %d，实际 %d", start+1, got)
	}

	// Update 是内部锁内读改写，同样要推进版本，否则外部带旧版本的提交会被误拒。
	if err := store.Update(func(b *AppBehavior) { b.AppLogLevel = "warn" }); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if got := store.Get().Revision; got != start+2 {
		t.Fatalf("Update 后 revision 应为 %d，实际 %d", start+2, got)
	}
}

func TestBehaviorRevisionRejectsStaleSubmit(t *testing.T) {
	store := newIsolatedBehaviorStore(t)

	first := store.Get()
	first.LogLevel = "info"
	if err := store.SetAndSave(first); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}

	// 模拟另一个入口在我们之后写了一次。
	if err := store.Update(func(b *AppBehavior) { b.AppLogLevel = "error" }); err != nil {
		t.Fatalf("并发写入失败: %v", err)
	}
	latest := store.Get()

	// 拿过期版本提交（就是修复前会静默覆盖别人的那一幕）。
	// 用布尔字段做断言：LogLevel 会被 normalizeBehavior 归一化（warn → warning），不适合比字面量。
	stale := latest
	stale.Revision = latest.Revision - 1
	stale.HideLogs = !latest.HideLogs
	err := store.SetAndSave(stale)
	if err == nil {
		t.Fatal("过期版本提交必须被拒绝，实际通过了")
	}
	if !strings.Contains(err.Error(), ErrCodeBehaviorConflict) {
		t.Fatalf("错误信息必须带稳定 code %q，实际: %v", ErrCodeBehaviorConflict, err)
	}

	// 拒绝就必须完全不落盘、不改内存：否则等于"拒了又写了"。
	after := store.Get()
	if after.HideLogs != latest.HideLogs {
		t.Fatal("被拒绝的提交污染了内存状态")
	}
	if after.Revision != latest.Revision {
		t.Fatalf("被拒绝的提交不应推进 revision：期望 %d，实际 %d", latest.Revision, after.Revision)
	}

	// 带上正确版本再提交 → 通过（这就是前端无感重试的路径）。
	retry := store.Get()
	retry.HideLogs = !after.HideLogs
	if err := store.SetAndSave(retry); err != nil {
		t.Fatalf("带正确版本的重试应当成功: %v", err)
	}
	if store.Get().HideLogs != retry.HideLogs {
		t.Fatal("重试成功后字段未生效")
	}
}

func TestBehaviorRevisionZeroSkipsCAS(t *testing.T) {
	store := newIsolatedBehaviorStore(t)

	if err := store.SetAndSave(store.Get()); err != nil {
		t.Fatalf("初始化保存失败: %v", err)
	}

	// 内部重置路径（ResetComponentSettings）用的是 Behavior.Default()，revision 为 0，
	// 表示"不带版本"，必须仍然允许通过，并且版本继续单调推进。
	cfg := store.Default()
	if cfg.Revision != 0 {
		t.Fatalf("Default() 的 revision 应为 0，实际 %d", cfg.Revision)
	}
	before := store.Get().Revision
	if err := store.SetAndSave(cfg); err != nil {
		t.Fatalf("revision=0 的内部重置不应被 CAS 拒绝: %v", err)
	}
	if got := store.Get().Revision; got != before+1 {
		t.Fatalf("revision=0 保存后仍应推进版本：期望 %d，实际 %d", before+1, got)
	}
}

func TestBehaviorRevisionSurvivesReload(t *testing.T) {
	store := newIsolatedBehaviorStore(t)

	b := store.Get()
	if err := store.SetAndSave(b); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	saved := store.Get().Revision

	// 模拟进程重启：同一个数据目录重新 Load。
	reloaded := NewBehaviorStore()
	if got := reloaded.Get().Revision; got != saved {
		t.Fatalf("重启后 revision 应保持 %d，实际 %d", saved, got)
	}
}
