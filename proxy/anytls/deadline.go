package anytls

import (
	"sync"
	"time"
)

// streamDeadline signals expiry without changing the shared connection's deadline.
// Pending waiters keep the same channel when a deadline is extended or cleared.
type streamDeadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	cancel chan struct{}
	closed bool
}

func (d *streamDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	if d.cancel == nil || channelClosed(d.cancel) {
		d.cancel = make(chan struct{})
	}
	if t.IsZero() {
		return
	}
	if delay := time.Until(t); delay > 0 {
		var timer *time.Timer
		timer = time.AfterFunc(delay, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.timer == timer {
				close(d.cancel)
				d.timer = nil
			}
		})
		d.timer = timer
	} else {
		close(d.cancel)
	}
}

func (d *streamDeadline) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

func (d *streamDeadline) wait() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel == nil {
		d.cancel = make(chan struct{})
	}
	return d.cancel
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
