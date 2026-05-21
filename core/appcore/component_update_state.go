package appcore

import (
	"sync"
)

type UpdateTaskState struct {
	Key        string  `json:"key"`
	Title      string  `json:"title"`
	Status     string  `json:"status"` // "running", "success", "error", "cancelled"
	Stage      string  `json:"stage"`  // "downloading", "extracting", "verifying"
	Progress   float64 `json:"progress"`
	BytesDone  int64   `json:"bytesDone"`
	BytesTotal int64   `json:"bytesTotal"`
	SpeedBps   int64   `json:"speedBps"`
	ETASeconds int64   `json:"etaSeconds"`
	Error      string  `json:"error"`
	StartedAt  int64   `json:"startedAt"`
	FinishedAt int64   `json:"finishedAt"`
}

type ComponentUpdateTaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]UpdateTaskState
	events EventSink
}

func NewComponentUpdateTaskStore(events EventSink) *ComponentUpdateTaskStore {
	return &ComponentUpdateTaskStore{
		tasks:  make(map[string]UpdateTaskState),
		events: events,
	}
}

func (s *ComponentUpdateTaskStore) Set(key string, state UpdateTaskState) {
	s.mu.Lock()
	s.tasks[key] = state
	list := s.snapshotLocked()
	s.mu.Unlock()

	if s.events != nil {
		s.events.Emit("component-update-tasks-sync", list)
	}
}

func (s *ComponentUpdateTaskStore) Snapshot() []UpdateTaskState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *ComponentUpdateTaskStore) snapshotLocked() []UpdateTaskState {
	list := make([]UpdateTaskState, 0, len(s.tasks))
	for _, v := range s.tasks {
		list = append(list, v)
	}
	return list
}

func (s *ComponentUpdateTaskStore) ClearFinished() {
	s.mu.Lock()
	changed := false
	for k, v := range s.tasks {
		if v.Status == "success" || v.Status == "error" || v.Status == "cancelled" {
			delete(s.tasks, k)
			changed = true
		}
	}
	var list []UpdateTaskState
	if changed {
		list = s.snapshotLocked()
	}
	s.mu.Unlock()

	if changed && s.events != nil {
		s.events.Emit("component-update-tasks-sync", list)
	}
}
