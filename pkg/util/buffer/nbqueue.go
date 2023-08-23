package buffer

type NonBlockingQueue[T any] struct {
	queue    chan T
	capacity int
}

func NewNonBlockingQueue[T any](capacity int) *NonBlockingQueue[T] {
	return &NonBlockingQueue[T]{
		queue:    make(chan T, capacity),
		capacity: capacity,
	}
}

func (q *NonBlockingQueue[T]) Enqueue(item T) bool {
	select {
	case q.queue <- item:
		return true
	default:
		return false
	}
}

func (q *NonBlockingQueue[T]) Dequeue() (T, bool) {
	select {
	case item := <-q.queue:
		return item, true
	default:
		var t T
		return t, false
	}
}
