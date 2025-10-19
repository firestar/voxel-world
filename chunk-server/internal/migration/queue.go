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
		q.pending = q.pending[:0]
		return batch
	}
	batch := append([]Request(nil), q.pending[:max]...)
	q.pending = q.pending[max:]
	return batch
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}
