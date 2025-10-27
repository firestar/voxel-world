package migration

import "sync"

type Queue struct {
	mu      sync.Mutex
	pending []Request
}

func NewQueue() *Queue {
	return &Queue{
		pending: make([]Request, 0),
	}
}

func (q *Queue) Enqueue(req Request) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, req)
}

func (q *Queue) Drain(max int) []Request {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	if max <= 0 || max >= len(q.pending) {
		batch := append([]Request(nil), q.pending...)
		for i := range q.pending {
			q.pending[i] = Request{}
		}
		q.pending = nil
		return batch
	}
	batch := append([]Request(nil), q.pending[:max]...)
	for i := 0; i < max; i++ {
		q.pending[i] = Request{}
	}
	remaining := q.pending[max:]
	// Copy the remaining requests into a new slice so that the queue does not retain
	// references to the drained entries, allowing them to be garbage collected.
	q.pending = append([]Request(nil), remaining...)
	return batch
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}
