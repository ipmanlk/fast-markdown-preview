package main

import (
	"testing"
	"time"
)

// TestBroadcastDeliversToClients verifies that a broadcast reaches every
// registered client and is stored as the latest document.
func TestBroadcastDeliversToClients(t *testing.T) {
	b := NewBroker()

	ch1 := make(chan string, 8)
	ch2 := make(chan string, 8)
	b.Register(ch1)
	b.Register(ch2)

	b.Broadcast("<html>1</html>")

	got1 := <-ch1
	got2 := <-ch2
	if got1 != "<html>1</html>" || got2 != "<html>1</html>" {
		t.Fatalf("clients got %q, %q; want <html>1</html>", got1, got2)
	}

	if b.LastHTML() != "<html>1</html>" {
		t.Fatalf("LastHTML = %q", b.LastHTML())
	}
	if b.ClientCount() != 2 {
		t.Fatalf("ClientCount = %d, want 2", b.ClientCount())
	}
}

// TestUnregisterRemovesClient ensures unregistering drops a client and closes
// its channel so its /stream goroutine can exit.
func TestUnregisterRemovesClient(t *testing.T) {
	b := NewBroker()

	ch := make(chan string, 8)
	b.Register(ch)
	b.Unregister(ch)

	// Channel should be closed after unregister.
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed after unregister")
	}
	if b.ClientCount() != 0 {
		t.Fatalf("ClientCount = %d, want 0", b.ClientCount())
	}
}

// TestUnregisterUnknownClientIsSafe ensures unregistering a channel that was
// never registered (or already removed) does not panic.
func TestUnregisterUnknownClientIsSafe(t *testing.T) {
	b := NewBroker()
	ch := make(chan string, 8)
	// Unregistering an unknown channel must not panic or block.
	b.Unregister(ch)
}

// TestSlowClientDropped verifies that a client whose channel buffer is full is
// dropped rather than blocking the broadcast.
func TestSlowClientDropped(t *testing.T) {
	b := NewBroker()

	// A client with a tiny buffer that nobody reads from.
	ch := make(chan string, 1)
	b.Register(ch)

	// Fill the buffer, then broadcast more. The slow client should be dropped.
	b.Broadcast("a")
	b.Broadcast("b") // buffer now full
	b.Broadcast("c") // this should drop the slow client

	// Drain what we can; the client should have been removed and closed.
	deadline := time.Now().Add(time.Second)
	for {
		_, ok := <-ch
		if !ok {
			return // closed as expected
		}
		if time.Now().After(deadline) {
			t.Fatal("slow client channel was never closed")
		}
	}
}

// TestShutdownClosesClients verifies that Shutdown closes all client channels
// (so /stream goroutines exit cleanly).
func TestShutdownClosesClients(t *testing.T) {
	b := NewBroker()

	ch1 := make(chan string, 8)
	ch2 := make(chan string, 8)
	b.Register(ch1)
	b.Register(ch2)

	b.Shutdown()

	// Both channels should be closed.
	for i, ch := range []chan string{ch1, ch2} {
		if _, ok := <-ch; ok {
			t.Fatalf("client %d channel not closed after shutdown", i)
		}
	}
}

// TestRegisterAfterShutdown verifies that registering after shutdown closes
// the new channel immediately (so a late /stream goroutine exits).
func TestRegisterAfterShutdown(t *testing.T) {
	b := NewBroker()
	b.Shutdown()

	ch := make(chan string, 8)
	b.Register(ch)
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed when registered after shutdown")
	}
}

// TestTimeSinceLastUpdate verifies the idle metric used by the watchdog.
func TestTimeSinceLastUpdate(t *testing.T) {
	b := NewBroker()

	// Before any update, TimeSinceLastUpdate should be large (eligible for
	// idle shutdown).
	if b.TimeSinceLastUpdate() < time.Minute {
		t.Fatalf("TimeSinceLastUpdate before any update = %v, want >= 1m", b.TimeSinceLastUpdate())
	}

	b.Broadcast("x")
	if b.TimeSinceLastUpdate() > time.Second {
		t.Fatalf("TimeSinceLastUpdate after broadcast = %v, want < 1s", b.TimeSinceLastUpdate())
	}
}

// TestHasEverHadClient verifies the flag the no-connection watchdog relies on
// to detect orphaned servers. It must be false until a client registers, flip
// to true on the first Register, and stay true after the client disconnects
// (so a brief browser blip does not re-arm the no-connection watchdog).
func TestHasEverHadClient(t *testing.T) {
	b := NewBroker()
	if b.HasEverHadClient() {
		t.Fatal("expected false before any client registers")
	}

	ch := make(chan string, 8)
	b.Register(ch)
	if !b.HasEverHadClient() {
		t.Fatal("expected true after a client registers")
	}

	// Disconnecting the only client must not clear the flag.
	b.Unregister(ch)
	if !b.HasEverHadClient() {
		t.Fatal("expected flag to stay true after the client disconnects")
	}
}
