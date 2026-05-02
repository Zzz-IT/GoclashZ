package appcore

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"goclashz/core/clash"
	"goclashz/core/traffic"
)

type ConnectionMetadataDTO struct {
	Network         string `json:"network"`
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	DestinationIP   string `json:"destinationIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
}

type ConnectionDTO struct {
	ID          string                `json:"id"`
	Metadata    ConnectionMetadataDTO `json:"metadata"`
	Upload      int64                 `json:"upload"`
	Download    int64                 `json:"download"`
	Start       string                `json:"start"`
	Chains      []string              `json:"chains"`
	Rule        string                `json:"rule"`
	RulePayload string                `json:"rulePayload"`
	UploadStr   string                `json:"uploadStr"`
	DownloadStr string                `json:"downloadStr"`
	DurationStr string                `json:"durationStr"`
}

type ConnectionsSnapshot struct {
	Connections []ConnectionDTO `json:"connections"`
}

type ConnectionMonitorManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	gen    int
	emit   EventSink
}

func NewConnectionMonitorManager(emit EventSink) *ConnectionMonitorManager {
	return &ConnectionMonitorManager{
		emit: emit,
	}
}

func toConnectionDTO(v traffic.ConnectionVO) ConnectionDTO {
	return ConnectionDTO{
		ID: v.ID,
		Metadata: ConnectionMetadataDTO{
			Network:         v.Metadata.Network,
			Type:            v.Metadata.Type,
			SourceIP:        v.Metadata.SourceIP,
			DestinationIP:   v.Metadata.DestinationIP,
			SourcePort:      v.Metadata.SourcePort,
			DestinationPort: v.Metadata.DestinationPort,
			Host:            v.Metadata.Host,
		},
		Upload:      v.Upload,
		Download:    v.Download,
		Start:       v.Start,
		Chains:      v.Chains,
		Rule:        v.Rule,
		RulePayload: v.RulePayload,
		UploadStr:   v.UploadStr,
		DownloadStr: v.DownloadStr,
		DurationStr: v.DurationStr,
	}
}

func (m *ConnectionMonitorManager) GetSnapshot() (ConnectionsSnapshot, error) {
	raw, err := clash.GetConnectionsRaw()
	if err != nil {
		return ConnectionsSnapshot{}, err
	}

	var payload struct {
		Connections []traffic.RawConnection `json:"connections"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		return ConnectionsSnapshot{}, err
	}

	vos := traffic.ProcessConnections(payload.Connections)
	out := make([]ConnectionDTO, 0, len(vos))
	for _, v := range vos {
		out = append(out, toConnectionDTO(v))
	}

	return ConnectionsSnapshot{
		Connections: out,
	}, nil
}

func (m *ConnectionMonitorManager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}

	m.gen++
	currentGen := m.gen
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		defer func() {
			m.mu.Lock()
			if m.gen == currentGen {
				m.cancel = nil
			}
			m.mu.Unlock()
		}()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				snap, err := m.GetSnapshot()
				if err != nil {
					continue
				}

				m.mu.Lock()
				alive := m.gen == currentGen && m.cancel != nil
				m.mu.Unlock()

				if !alive {
					return
				}

				if m.emit != nil {
					m.emit.Emit("connections-update", snap)
				}
			}
		}
	}()
}

func (m *ConnectionMonitorManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
	}
}
