package slice

import "testing"

func TestRemoveFromEmptySlice(t *testing.T) {
	s := []int{}

	RemoveFirstMatchFromSlice(s, func(i int) bool {
		return true
	})

	if len(s) != 0 {
		t.Fatalf("Remove from empty list failed")
	}
}

func TestRemoveFromSingleElemSlice(t *testing.T) {
	s := []int{1}

	s = RemoveFirstMatchFromSlice(s, func(i int) bool {
		return true
	}).([]int)

	if len(s) != 0 {
		t.Fatalf("Remove from single element list failed. Slice is %v", s)
	}

	strs := []string{"abc"}
	strs = RemoveFirstMatchFromSlice(strs, func(i int) bool {
		return true
	}).([]string)

	if len(strs) != 0 {
		t.Fatalf("Remove from single element list failed. Slice is %v", s)
	}

	s = []int{1}

	s = RemoveFirstMatchFromSlice(s, func(i int) bool {
		return false
	}).([]int)

	if len(s) != 1 {
		t.Fatalf("Remove non-existant item from single element list failed. Slice is %v", s)
	}

}

func TestRemoveFromTwoElemSlice(t *testing.T) {

	s := []string{"pre", "post"}
	s = RemoveFirstMatchFromSlice(s, func(i int) bool {
		return i == 1
	}).([]string)

	if len(s) != 1 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "pre" {
		t.Fatalf("Remove last elem from two element list failed. Slice is %v", s)
	}

	s = []string{"pre", "post"}
	s = RemoveFirstMatchFromSlice(s, func(i int) bool {
		return i == 0
	}).([]string)

	if len(s) != 1 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "post" {
		t.Fatalf("Remove first elem from two element list failed. Slice is %v", s)
	}

	s = []string{"pre", "post"}
	s = RemoveFirstMatchFromSlice(s, func(i int) bool {
		return false
	}).([]string)

	if len(s) != 2 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "pre" || s[1] != "post" {
		t.Fatalf("Remove first elem from two element list failed. Slice is %v", s)
	}
}

func TestRemovePreserveOrderFromSingleElemSlice(t *testing.T) {
	s := []int{1}

	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return true
	}).([]int)

	if len(s) != 0 {
		t.Fatalf("Remove from single element list failed. Slice is %v", s)
	}

	strs := []string{"abc"}
	strs = RemoveFirstMatchFromSlicePreserveOrder(strs, func(i int) bool {
		return true
	}).([]string)

	if len(strs) != 0 {
		t.Fatalf("Remove from single element list failed. Slice is %v", s)
	}

	s = []int{1}

	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return false
	}).([]int)

	if len(s) != 1 {
		t.Fatalf("Remove non-existant item from single element list failed. Slice is %v", s)
	}

}

func TestRemovePreserveOrderFromTwoElemSlice(t *testing.T) {

	s := []string{"pre", "post"}
	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return i == 1
	}).([]string)

	if len(s) != 1 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "pre" {
		t.Fatalf("Remove last elem from two element list failed. Slice is %v", s)
	}

	s = []string{"pre", "post"}
	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return i == 0
	}).([]string)

	if len(s) != 1 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "post" {
		t.Fatalf("Remove first elem from two element list failed. Slice is %v", s)
	}

	s = []string{"pre", "post"}
	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return false
	}).([]string)

	if len(s) != 2 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "pre" || s[1] != "post" {
		t.Fatalf("Remove first elem from two element list failed. Slice is %v", s)
	}
}

func TestRemovePreserveOrderFromThreeElemSlice(t *testing.T) {

	s := []string{"pre", "mid", "post"}
	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return i == 0
	}).([]string)

	if len(s) != 2 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "mid" {
		t.Fatalf("Remove last elem from two element list failed. Slice is %v", s)
	}
	if s[1] != "post" {
		t.Fatalf("Remove last elem from two element list failed. Slice is %v", s)
	}

	s = []string{"pre", "mid", "post"}
	s = RemoveFirstMatchFromSlicePreserveOrder(s, func(i int) bool {
		return i == 1
	}).([]string)

	if len(s) != 2 {
		t.Fatalf("Remove from multi element list failed. Slice is %v", s)
	}

	if s[0] != "pre" {
		t.Fatalf("Remove second elem from three element list failed. Slice is %v", s)
	}
	if s[1] != "post" {
		t.Fatalf("Remove second elem from three element list failed. Slice is %v", s)
	}

}
