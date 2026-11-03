package mobile

import (
	"encoding/json"
	"sync"

	"sshc/internal/terminal"
)

// blockedPrograms carries the engine's terminal.BlockedProgramChange to the Android service. It belongs to the process,
// not to one engine: what an engine says while stopping (its sessions no longer wait) still has to arrive, and the
// thread that waits here does not have to be told about the next engine.
var blockedPrograms = newBlockedProgramQueue()

// blockedProgramQueue lets the engine go on without waiting for Java, so add only queues. A newer change of a session
// replaces one not taken yet, which keeps the queue to one change per session.
type blockedProgramQueue struct {
	mutex   sync.Mutex
	ready   *sync.Cond
	pending []terminal.BlockedProgramChange
}

func newBlockedProgramQueue() *blockedProgramQueue {
	queue := &blockedProgramQueue{}
	queue.ready = sync.NewCond(&queue.mutex)
	return queue
}

func (q *blockedProgramQueue) add(change terminal.BlockedProgramChange) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	defer q.ready.Signal()
	for index := range q.pending {
		if q.pending[index].SessionID == change.SessionID {
			q.pending[index] = change
			return
		}
	}
	q.pending = append(q.pending, change)
}

// take waits for the next change.
func (q *blockedProgramQueue) take() terminal.BlockedProgramChange {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for len(q.pending) == 0 {
		q.ready.Wait()
	}
	change := q.pending[0]
	q.pending = q.pending[1:]
	return change
}

// blockedProgramChangeJSON is what NextBlockedProgramChange returns to Java.
type blockedProgramChangeJSON struct {
	SessionID    string `json:"sessionId"`
	SessionAlias string `json:"sessionAlias"`
	SessionTitle string `json:"sessionTitle"`
	Blocked      bool   `json:"blocked"`
	// Kind is what the program waits for (permission, question, auth), or "".
	Kind string `json:"kind"`
	// Said is what the program said about it (its message, or its title when it
	// sent none), as the session list shows it.
	Said string `json:"said"`
}

func encodeBlockedProgramChange(change terminal.BlockedProgramChange) string {
	encoded := blockedProgramChangeJSON{SessionID: change.SessionID, SessionAlias: change.SessionAlias, SessionTitle: change.SessionTitle}
	if status := change.Status; status != nil {
		encoded.Blocked = true
		encoded.Kind = string(status.Kind)
		encoded.Said = status.Message
		if encoded.Said == "" {
			encoded.Said = status.Title
		}
	}
	// Strings alone cannot fail to encode.
	body, _ := json.Marshal(encoded)
	return string(body)
}

// NextBlockedProgramChange waits until a session's program starts or stops waiting for the person, and returns that as
// JSON (blockedProgramChangeJSON).
//
// Android calls this over and over on one thread of its own, for as long as the process lives. A blocking call keeps
// the boundary to strings; a callback would need an interface across it.
func NextBlockedProgramChange() string {
	return encodeBlockedProgramChange(blockedPrograms.take())
}
