package changes

import (
	"errors"
	"sync"

	shared "ledgermeadow/src/shared/types"
)

const (
	maxStreamsPerReplica = 1000
	maxStreamsPerUser    = 5
	streamBufferSize     = 1
)

var (
	ErrReplicaStreamLimit = errors.New("replica change-stream limit reached")
	ErrUserStreamLimit    = errors.New("user change-stream limit reached")
)

type Hub struct {
	mu          sync.Mutex
	subscribers map[shared.UserID]map[uint64]chan Event
	nextID      uint64
	total       int
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[shared.UserID]map[uint64]chan Event)}
}

func (h *Hub) Subscribe(userID shared.UserID) (<-chan Event, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= maxStreamsPerReplica {
		return nil, nil, ErrReplicaStreamLimit
	}
	userSubscribers := h.subscribers[userID]
	if len(userSubscribers) >= maxStreamsPerUser {
		return nil, nil, ErrUserStreamLimit
	}
	if userSubscribers == nil {
		userSubscribers = make(map[uint64]chan Event)
		h.subscribers[userID] = userSubscribers
	}
	h.nextID++
	id := h.nextID
	stream := make(chan Event, streamBufferSize)
	userSubscribers[id] = stream
	h.total++

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			current := h.subscribers[userID]
			if _, ok := current[id]; !ok {
				return
			}
			delete(current, id)
			h.total--
			if len(current) == 0 {
				delete(h.subscribers, userID)
			}
		})
	}
	return stream, unsubscribe, nil
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, stream := range h.subscribers[event.UserID] {
		select {
		case stream <- event:
		default:
			merged := event
			select {
			case queued := <-stream:
				merged = mergeEvents(queued, event)
			default:
			}
			select {
			case stream <- merged:
			default:
			}
		}
	}
}

func mergeEvents(queued Event, incoming Event) Event {
	resources := make([]string, 0, len(queued.Resources)+len(incoming.Resources))
	seen := make(map[string]struct{}, len(queued.Resources)+len(incoming.Resources))
	for _, event := range []Event{queued, incoming} {
		for _, resource := range event.Resources {
			if _, exists := seen[resource]; exists {
				continue
			}
			seen[resource] = struct{}{}
			resources = append(resources, resource)
		}
	}
	incoming.Resources = resources
	return incoming
}

func (h *Hub) DisconnectAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, userSubscribers := range h.subscribers {
		for id, stream := range userSubscribers {
			close(stream)
			delete(userSubscribers, id)
		}
		delete(h.subscribers, userID)
	}
	h.total = 0
}
