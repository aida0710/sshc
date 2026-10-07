package main

import "encoding/json"

type browserRequest struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
	Text    *string           `json:"text,omitempty"`
	Cols    uint16            `json:"cols,omitempty"`
	Rows    uint16            `json:"rows,omitempty"`
}

type browserReply struct {
	ID        string              `json:"id,omitempty"`
	Kind      string              `json:"kind"`
	Status    int                 `json:"status,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      []byte              `json:"body,omitempty"`
	Text      string              `json:"text,omitempty"`
	Bootstrap string              `json:"bootstrap,omitempty"`
}

func (b *browserBridge) reply(reply browserReply) {
	b.outputMutex.Lock()
	defer b.outputMutex.Unlock()
	_ = json.NewEncoder(b.serial).Encode(reply)
}
