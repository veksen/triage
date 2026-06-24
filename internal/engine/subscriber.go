package engine

import (
	"context"
	"sync"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
)

// subscriber is a single streaming consumer. It is replace-latest: it holds at
// most one pending board, and a new push overwrites a superseded one rather than
// queuing it. A slow consumer therefore never blocks the engine and only ever
// drops snapshots it would have rendered stale anyway.
type subscriber struct {
	mu     sync.Mutex
	latest *triagev1.Board
	// notify is a size-1 buffered channel used purely as a coalescing signal:
	// "a newer board is waiting". The board itself travels via latest.
	notify chan struct{}
}

// push records b as the pending board and signals the waiter. Non-blocking by
// construction (the notify send is a select/default), so it is safe to call
// while holding the engine lock.
func (s *subscriber) push(b *triagev1.Board) {
	s.mu.Lock()
	s.latest = b
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// take returns the pending board and clears it, or (nil,false) if none pending.
func (s *subscriber) take() (*triagev1.Board, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.latest
	s.latest = nil
	return b, b != nil
}

// Subscription is the consumer handle returned by Engine.Subscribe.
type Subscription struct {
	s *subscriber
}

// Next blocks until a new board snapshot is available or ctx is done. It always
// returns the most recent board, skipping any superseded snapshots. ok is false
// only when ctx is done.
func (sub *Subscription) Next(ctx context.Context) (board *triagev1.Board, ok bool) {
	for {
		if b, has := sub.s.take(); has {
			return b, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-sub.s.notify:
			// loop: drain via take(), which also coalesces multiple signals.
		}
	}
}

// Subscribe registers a streaming consumer, primes it with the current board so
// the first Next yields the present snapshot, and returns a cancel func to
// unregister. Registration happens under the engine lock, so the subscriber is
// atomic with respect to broadcasts: it either predates a broadcast (and
// receives it) or postdates one (and its prime already reflects it).
func (e *Engine) Subscribe() (*Subscription, func()) {
	s := &subscriber{notify: make(chan struct{}, 1)}

	e.mu.Lock()
	id := e.nextID
	e.nextID++
	e.subs[id] = s
	s.push(e.board)
	e.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			e.mu.Lock()
			delete(e.subs, id)
			e.mu.Unlock()
		})
	}
	return &Subscription{s: s}, cancel
}
