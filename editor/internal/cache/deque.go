package cache

import "fmt"

type Deque struct {
	buf        []interface{}
	head, tail int
	count      int
}

func NewDeque(max int) Deque {
	return Deque{
		buf: make([]interface{}, max),
	}
}

func (q *Deque) PushBack(elem interface{}) error {
	if q.count == len(q.buf) {
		return fmt.Errorf("queue is full")
	}

	q.buf[q.tail] = elem
	q.tail = q.next(q.tail)
	q.count++
	return nil
}

func (q *Deque) next(i int) int {
	return (i + 1) % len(q.buf)
}

func (q *Deque) PopFront() (elem interface{}) {
	if q.count == 0 {
		return nil
	}

	elem = q.buf[q.head]
	q.head = q.next(q.head)
	q.count--
	return
}

func (q *Deque) Count() int {
	return q.count
}

func (q *Deque) Max() int {
	return len(q.buf)
}

func (q *Deque) Find(match func(interface{}) bool) interface{} {
	if q.count == 0 {
		return nil
	}

	// A full Dequeue has head == tail, but so does an empty one.
	i := q.head
	for {
		if match(q.buf[i]) {
			return q.buf[i]
		}
		i = q.next(i)
		if i == q.tail {
			break
		}
	}
	return nil
}

// Del removes the first match from the Deque
func (q *Deque) Del(match func(interface{}) bool) {
	for i := 0; i < q.count; i++ {
		if match(q.buf[i]) {
			// Swap head and item to delete
			q.buf[q.head], q.buf[i] = q.buf[i], q.buf[q.head]
			q.PopFront()
			break
		}
	}
}

func (q *Deque) Clear() {
	q.buf = q.buf[:0]
	q.head = 0
	q.tail = 0
	q.count = 0
}
