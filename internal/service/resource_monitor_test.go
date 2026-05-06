package service

import (
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

func TestResourceMonitor_GetCurrent_Default(t *testing.T) {
	m := &ResourceMonitor{current: &model.SystemResources{}}
	got := m.GetCurrent()
	if got == nil {
		t.Fatal("GetCurrent returned nil")
	}
}

func TestResourceMonitor_Subscribe_ReceivesBroadcast(t *testing.T) {
	m := &ResourceMonitor{current: &model.SystemResources{}}

	ch, unsub := m.Subscribe()
	defer unsub()

	want := &model.SystemResources{MemoryTotal: 1024, MemoryUsed: 512}
	m.broadcast(want)

	select {
	case got := <-ch:
		if got != want {
			t.Errorf("received unexpected resource snapshot")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestResourceMonitor_Unsubscribe_ClosesChannel(t *testing.T) {
	m := &ResourceMonitor{current: &model.SystemResources{}}

	ch, unsub := m.Subscribe()
	unsub() // should close ch

	// After unsubscribe the channel must be closed; a receive must not block.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after unsubscribe")
		}
	default:
		// closed channels are readable, so reaching default means it wasn't closed
		t.Error("channel should be closed but default branch hit")
	}
}

func TestResourceMonitor_BroadcastFullBuffer_DoesNotBlock(t *testing.T) {
	m := &ResourceMonitor{current: &model.SystemResources{}}
	ch, unsub := m.Subscribe()
	defer unsub()

	stats := &model.SystemResources{MemoryTotal: 1}

	// Fill the buffer (capacity 1) without reading.
	m.broadcast(stats)
	// A second broadcast should not block even though the buffer is full.
	done := make(chan struct{})
	go func() {
		m.broadcast(stats)
		close(done)
	}()

	select {
	case <-done:
		// good — non-blocking
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on full subscriber buffer")
	}

	_ = ch
}

func TestResourceMonitor_MultipleSubscribers(t *testing.T) {
	m := &ResourceMonitor{current: &model.SystemResources{}}

	ch1, unsub1 := m.Subscribe()
	ch2, unsub2 := m.Subscribe()
	defer unsub1()
	defer unsub2()

	stats := &model.SystemResources{DiskTotal: 9999}
	m.broadcast(stats)

	for _, ch := range []chan *model.SystemResources{ch1, ch2} {
		select {
		case got := <-ch:
			if got.DiskTotal != 9999 {
				t.Errorf("subscriber received wrong data: %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive broadcast")
		}
	}
}
