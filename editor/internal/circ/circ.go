package circ

// Circ implements a circular array
type Circ[V any] struct {
	entries            []V
	first, last, count int
}

func New[V any](max int) Circ[V] {
	if max < 1 {
		max = 1
	}

	return Circ[V]{
		entries: make([]V, max),
	}
}

func (c Circ[V]) Empty() bool {
	return c.count == 0
}

func (c Circ[V]) full() bool {
	return c.count == len(c.entries)
}

func (c *Circ[V]) Add(v V) {
	c.entries[c.last] = v
	c.last = c.mod(c.last + 1)
	if c.mod(c.first+1) == c.last && c.full() {
		// array was full; we just evicted the first entry.
		c.first = c.mod(c.first + 1)
		return
	}
	c.count++
}

func (c Circ[V]) mod(index int) int {
	return index % len(c.entries)
}

func (c Circ[V]) Each(f func(v V)) {
	if c.Empty() {
		return
	}

	i := c.first
	for {
		f(c.entries[i])
		i = c.mod(i + 1)
		if i == c.last {
			break
		}
	}
}
