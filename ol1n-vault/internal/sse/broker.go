package sse

import (
	"encoding/json"
	"sync"
)

// Broker fans out JSON events keyed by a string (typically job_id) to any
// number of subscribers. Publishers never block — slow subscribers drop events.
type Broker struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[string]map[chan []byte]struct{})}
}

// Subscribe returns a channel that receives JSON-encoded events for the given
// key. Call the returned cancel function to unsubscribe.
func (b *Broker) Subscribe(key string) (<-chan []byte, func()) {
	ch := make(chan []byte, 16)
	b.mu.Lock()
	if _, ok := b.subs[key]; !ok {
		b.subs[key] = make(map[chan []byte]struct{})
	}
	b.subs[key][ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if set, ok := b.subs[key]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, key)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// Publish broadcasts v (serialized as JSON) to subscribers of key.
func (b *Broker) Publish(key string, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.RLock()
	subs := b.subs[key]
	chans := make([]chan []byte, 0, len(subs))
	for ch := range subs {
		chans = append(chans, ch)
	}
	b.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- payload:
		default:
		}
	}
}
