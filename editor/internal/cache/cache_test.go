package cache

import (
	"testing"
)

func TestDeque(t *testing.T) {
	d := NewDeque(3)
	if d.Count() != 0 {
		t.Fatalf("Bad count when empty")
	}

	d.PushBack(1)
	d.PushBack(2)
	d.PushBack(3)

	mkMatch := func(v int) func(i interface{}) bool {
		return func(i interface{}) bool {
			return i.(int) == v
		}
	}

	if d.Find(mkMatch(2)) == nil {
		t.Fatalf("Can't find 2 in the list when it's there")
	}

	if d.Find(mkMatch(20)) != nil {
		t.Fatalf("Found 20 when it's not there")
	}

	if d.Count() != 3 {
		t.Fatalf("Bad count when having 3 elements")
	}

	if d.Max() != 3 {
		t.Fatalf("Bad max when having 3 elements")
	}

	v := d.PopFront()
	if v.(int) != 1 {
		t.Fatalf("Expected 1 but got %d", v.(int))
	}

	v = d.PopFront()
	if v.(int) != 2 {
		t.Fatalf("Expected 2 but got %d", v.(int))
	}

	v = d.PopFront()
	if v.(int) != 3 {
		t.Fatalf("Expected 3 but got %d", v.(int))
	}
}

func TestDequeDel(t *testing.T) {
	d := NewDeque(3)
	if d.Count() != 0 {
		t.Fatalf("Bad count when empty")
	}

	d.PushBack(1)
	d.PushBack(2)
	d.PushBack(3)

	mkMatch := func(v int) func(i interface{}) bool {
		return func(i interface{}) bool {
			return i.(int) == v
		}
	}

	d.Del(mkMatch(2))

	if d.Count() != 2 {
		t.Fatalf("Deleting didn't decrement count")
	}

	if d.Find(mkMatch(1)) == nil {
		t.Fatalf("Can't find 1 in the list when 2 was deleted")
	}
	if d.Find(mkMatch(3)) == nil {
		t.Fatalf("Can't find 3 in the list when 2 was deleted")
	}
	if d.Find(mkMatch(2)) != nil {
		t.Fatalf("Found 2 in the list after deletion")
	}

}

func TestDequeOverfill(t *testing.T) {
	d := NewDeque(2)
	if d.Count() != 0 {
		t.Fatalf("Bad count when empty")
	}

	d.PushBack(1)
	d.PushBack(2)
	err := d.PushBack(3)
	if err == nil {
		t.Fatalf("Expected error when pushing to full Deque")
	}

	if d.Count() != 2 {
		t.Fatalf("Bad count when having 2 elements")
	}

	if d.Max() != 2 {
		t.Fatalf("Bad max when having 2 elements")
	}

	v := d.PopFront()
	if v.(int) != 1 {
		t.Fatalf("Expected 1 but got %d", v.(int))
	}

	v = d.PopFront()
	if v.(int) != 2 {
		t.Fatalf("Expected 2 but got %d", v.(int))
	}

	v = d.PopFront()
	if v != nil {
		t.Fatalf("Expected nil but got %v", v)
	}
}

func TestCache(t *testing.T) {
	cache := New[int, int](2)

	cache.Set(1, 1)
	e := cache.Get(1)
	if e.Val != 1 || e.Key != 1 {
		t.Fatalf("Expected 1,1 but got %v\n", e)
	}

	cache.Set(2, 2)

	for i := 1; i <= 2; i++ {
		e = cache.Get(i)
		if e.Val != i || e.Key != i {
			t.Fatalf("Expected %d,%d but got %v\n", i, i, e)
		}
	}

	cache.Set(3, 3)
	// 1 should now be evicted
	e = cache.Get(1)
	if e != nil {
		t.Fatalf("Expected nil but got %v\n", e)
	}

	for i := 2; i <= 3; i++ {
		e = cache.Get(i)
		if e.Val != i || e.Key != i {
			t.Fatalf("Expected %d,%d but got %v\n", i, i, e)
		}
	}
}

func TestCacheDel(t *testing.T) {
	cache := New[int, int](2)

	cache.Set(1, 1)
	cache.Set(2, 2)

	cache.Del(1)
	e := cache.Get(1)
	if e != nil {
		t.Fatalf("Expected nil but got %v\n", e)
	}
	e = cache.Get(2)
	if e == nil {
		t.Fatalf("Expected to get 2 but got nil\n")
	}
}
