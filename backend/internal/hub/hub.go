// Package hub fans messages out to whoever is listening on a topic: a draft
// room, a sport's scoreboard, one game. The server is the only publisher;
// browsers listen over a websocket.
package hub

import "sync"

type Hub struct {
	mu     sync.Mutex
	topics map[string]map[chan []byte]struct{}
}

func New() *Hub {
	return &Hub{topics: map[string]map[chan []byte]struct{}{}}
}

// Subscribe returns a channel of messages for a topic and a function to
// stop listening. The channel is closed if the subscriber falls too far
// behind; it should then reconnect and start from a fresh snapshot.
func (h *Hub) Subscribe(topic string) (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.topics[topic] == nil {
		h.topics[topic] = map[chan []byte]struct{}{}
	}
	h.topics[topic][ch] = struct{}{}

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.drop(topic, ch)
	}
}

// Publish sends a message to everyone on a topic without blocking.
func (h *Hub) Publish(topic string, message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.topics[topic] {
		select {
		case ch <- message:
		default:
			h.drop(topic, ch)
		}
	}
}

// Listening reports whether anyone is subscribed to a topic.
func (h *Hub) Listening(topic string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.topics[topic]) > 0
}

// drop removes and closes a subscriber. The caller holds the lock.
func (h *Hub) drop(topic string, ch chan []byte) {
	if _, ok := h.topics[topic][ch]; ok {
		delete(h.topics[topic], ch)
		close(ch)
	}
}
