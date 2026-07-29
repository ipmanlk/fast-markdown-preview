package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// Broker fans out rendered HTML documents to all connected SSE clients.
//
// Each client gets its own buffered channel so a slow reader cannot block the
// broadcast path. The broadcast uses a non-blocking send: if a client's
// channel is full, that client is dropped (its /stream goroutine will observe
// the closed channel and exit). This keeps the hot path (POST /update) fast
// and bounded in memory.
//
// All mutations go through a single mutex. Register/unregister are rare (only
// on connect/disconnect), so the lock is uncontended on the hot path. The
// client count is mirrored in an atomic so the idle watchdog can read it
// without taking the lock.
type Broker struct {
	mu           sync.RWMutex
	clients      map[chan string]struct{}
	lastHTML     string
	clientCount  atomic.Int32
	lastUpdateAt atomic.Int64 // unix seconds of the most recent /update
	closed       atomic.Bool
	hadClient    atomic.Bool // set when the first SSE client connects
}

// NewBroker creates a ready-to-use broker.
func NewBroker() *Broker {
	return &Broker{
		clients: make(map[chan string]struct{}),
	}
}

// Register adds a client channel. If the broker has been shut down, the channel
// is closed immediately so the caller's /stream goroutine exits.
func (b *Broker) Register(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() {
		close(ch)
		return
	}
	b.clients[ch] = struct{}{}
	b.clientCount.Store(int32(len(b.clients)))
	b.hadClient.Store(true)
}

// Unregister removes a client channel and closes it. Safe to call for a channel
// that was never registered or already removed (no-op in that case).
func (b *Broker) Unregister(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
		b.clientCount.Store(int32(len(b.clients)))
	}
}

// Broadcast stores html as the latest document and fans it out to every
// client. It is called from the /update handler. The send is non-blocking: a
// client whose buffer is full is dropped rather than stalling the caller.
func (b *Broker) Broadcast(html string) {
	b.lastUpdateAt.Store(time.Now().Unix())

	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastHTML = html
	for ch := range b.clients {
		select {
		case ch <- html:
		default:
			// Slow client: drop it so the broadcast stays fast.
			delete(b.clients, ch)
			close(ch)
			b.clientCount.Store(int32(len(b.clients)))
		}
	}
}

// LastHTML returns the most recently broadcast document (sent to new clients on
// connect so they never see a blank page).
func (b *Broker) LastHTML() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastHTML
}

// ClientCount returns the number of connected SSE clients.
func (b *Broker) ClientCount() int {
	return int(b.clientCount.Load())
}

// TimeSinceLastUpdate reports how long since the last /update. Used by the idle
// watchdog. Returns a large duration if no update has ever been received, so
// the watchdog can treat "never updated" as eligible for idle shutdown.
func (b *Broker) TimeSinceLastUpdate() time.Duration {
	t := b.lastUpdateAt.Load()
	if t == 0 {
		return time.Hour
	}
	return time.Since(time.Unix(t, 0))
}

// HasEverHadClient returns true if at least one SSE client has ever connected
// since the server started. Used by the no-connection watchdog to detect
// orphaned servers (browser never opened).
func (b *Broker) HasEverHadClient() bool {
	return b.hadClient.Load()
}

// Shutdown closes all client channels and marks the broker closed so further
// registrations are rejected. Called during graceful server shutdown so /stream
// goroutines unblock and return.
func (b *Broker) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed.Store(true)
	for ch := range b.clients {
		close(ch)
	}
	b.clients = make(map[chan string]struct{})
	b.clientCount.Store(0)
}
