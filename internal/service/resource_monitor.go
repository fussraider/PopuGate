package service

import (
	"context"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var (
	monitor   *ResourceMonitor
	monitorMu sync.Mutex
)

// ResourceMonitor periodically collects system resources.
type ResourceMonitor struct {
	current *model.SystemResources
	mu      sync.RWMutex
	subs    []chan *model.SystemResources
	subsMu  sync.Mutex
	cancel  context.CancelFunc
	notify  NotifyFunc
}

// GetResourceMonitor returns the singleton instance of ResourceMonitor.
func GetResourceMonitor() *ResourceMonitor {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	return monitor
}

// InitResourceMonitor initializes the singleton instance of ResourceMonitor.
func InitResourceMonitor(notify NotifyFunc) *ResourceMonitor {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if monitor == nil {
		ctx, cancel := context.WithCancel(context.Background())
		monitor = &ResourceMonitor{
			current: &model.SystemResources{},
			cancel:  cancel,
			notify:  notify,
		}
		go monitor.run(ctx)
	}
	return monitor
}

func (m *ResourceMonitor) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.WithScope("resource-monitor").Errorf("goroutine panic (resource monitor): %v", r)
		}
	}()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Use a separate ticker for threshold checks to avoid spamming alerts
	// although CheckResources already has alertCooldown.
	checkTicker := time.NewTicker(1 * time.Minute)
	defer checkTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := GetResources()
			m.mu.Lock()
			m.current = stats
			m.mu.Unlock()

			m.broadcast(stats)
		case <-checkTicker.C:
			if m.notify != nil {
				_ = CheckResources(ctx, m.notify)
			}
		}
	}
}

// GetCurrent returns the latest collected resources.
func (m *ResourceMonitor) GetCurrent() *model.SystemResources {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Subscribe returns a channel that receives resource updates.
func (m *ResourceMonitor) Subscribe() (chan *model.SystemResources, func()) {
	ch := make(chan *model.SystemResources, 1)
	m.subsMu.Lock()
	m.subs = append(m.subs, ch)
	m.subsMu.Unlock()

	unsubscribe := func() {
		m.subsMu.Lock()
		defer m.subsMu.Unlock()
		for i, sub := range m.subs {
			if sub == ch {
				m.subs = append(m.subs[:i], m.subs[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return ch, unsubscribe
}

func (m *ResourceMonitor) broadcast(stats *model.SystemResources) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	for _, ch := range m.subs {
		select {
		case ch <- stats:
		default:
			// Buffer full, skip this update for this subscriber
		}
	}
}

// Stop stops the monitor.
func (m *ResourceMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}
