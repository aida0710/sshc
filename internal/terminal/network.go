package terminal

import "sync"

// NetworkWatch holds what the operating system says about this machine's
// network, for a machine that can tell (the Android app, from
// ConnectivityManager). A machine that cannot has none: a connection attempt
// that fails for lack of a network says so instead (ProblemNoNetwork).
//
// A nil *NetworkWatch is a machine that says nothing.
type NetworkWatch struct {
	mutex       sync.Mutex
	unavailable bool
	changed     chan struct{}
}

// NewNetworkWatch returns a watch that takes the network to be there until told otherwise.
func NewNetworkWatch() *NetworkWatch {
	return &NetworkWatch{changed: make(chan struct{})}
}

// SetAvailable records what the operating system said. Saying a network is
// available wakes the waiters even if there already was one: it may be a new one
// (Wi-Fi to mobile data), and an attempt that waits for its next turn can go now.
func (w *NetworkWatch) SetAvailable(available bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if !available && w.unavailable {
		return
	}
	w.unavailable = !available
	close(w.changed)
	w.changed = make(chan struct{})
}

// State returns whether a network is there, and a channel closed at the next
// thing the operating system says. The channel of a nil watch never closes.
func (w *NetworkWatch) State() (available bool, changed <-chan struct{}) {
	if w == nil {
		return true, nil
	}
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return !w.unavailable, w.changed
}
