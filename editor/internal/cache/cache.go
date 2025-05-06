package cache

type Cache[K comparable, V any] struct {
	entries    map[K]*Entry[K, V]
	orderAdded Deque
}

type Entry[K comparable, V any] struct {
	Key K
	Val V
}

func New[K comparable, V any](max int) Cache[K, V] {
	return Cache[K, V]{
		entries:    make(map[K]*Entry[K, V]),
		orderAdded: NewDeque(max),
	}
}

func (c Cache[K, V]) Get(key K) *Entry[K, V] {
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	return entry
}

func (c *Cache[K, V]) Set(key K, val V) *Entry[K, V] {
	entry := c.Get(key)
	if entry == nil {
		return c.addNewEntry(key, val)
	}

	return entry
}

func (c *Cache[K, V]) addNewEntry(key K, val V) *Entry[K, V] {
	c.removeOldestIfNeeded()

	entry := &Entry[K, V]{Key: key, Val: val}
	c.entries[key] = entry
	c.orderAdded.PushBack(entry)
	return entry
}

func (c *Cache[K, V]) removeOldestIfNeeded() {
	if c.orderAdded.Count() < c.orderAdded.Max() {
		return
	}

	e := c.orderAdded.PopFront().(*Entry[K, V])
	delete(c.entries, e.Key)
}

func (c *Cache[K, V]) Del(key K) {
	match := func(i interface{}) bool {
		return i.(*Entry[K, V]).Key == key
	}

	c.orderAdded.Del(match)
	delete(c.entries, key)
}

func (c *Cache[K, V]) Clear() {
	c.entries = make(map[K]*Entry[K, V])
	c.orderAdded.Clear()
}

func (c *Cache[K, V]) Len() int {
	return len(c.entries)
}
