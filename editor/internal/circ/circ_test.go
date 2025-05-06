package circ

import (
	"testing"
)

func count(c Circ[int]) int {
	count := 0
	c.Each(func(v int) { count++ })
	return count
}

func TestAdd(t *testing.T) {
	c := New[int](3)

	n := count(c)
	if n != 0 {
		t.Fatalf("Empty circ is not empty")
	}

	c.Add(1)
	n = count(c)
	if n != 1 {
		t.Fatalf("Wrong count")
	}

	c.Add(2)
	n = count(c)
	if n != 2 {
		t.Fatalf("Wrong count")
	}

	c.Add(3)
	n = count(c)
	if n != 3 {
		t.Fatalf("Wrong count. Expected %d but got %d", 3, n)
	}

	c.Add(4)
	n = count(c)
	if n != 3 {
		t.Fatalf("Wrong count")
	}

	// Should contain 2, 3, 4
	saw := []int{}
	c.Each(func(v int) { saw = append(saw, v) })

	if saw[0] != 2 || saw[1] != 3 || saw[2] != 4 {
		t.Fatalf("Wrong contents: %v", saw)
	}
}
