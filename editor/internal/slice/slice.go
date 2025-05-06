package slice

import "reflect"

// RemoveFirstMatchFromSlice removes the first matching element from the slice `x` and returns the
// slice. it does NOT preserve the order of the slice.
func RemoveFirstMatchFromSlice(x interface{}, matches func(i int) bool) interface{} {
	v := reflect.ValueOf(x)
	length := v.Len()

	ndx := 0
	for ndx < length {
		if matches(ndx) {
			break
		}
		ndx++
	}
	if ndx >= length {
		return x // Not found
	}

	last := length - 1

	// Swap found element and the last element
	tmp := v.Index(ndx)
	v.Index(ndx).Set(v.Index(last))
	v.Index(last).Set(tmp)

	// Slice off the last element
	return v.Slice(0, last).Interface()
}

// RemoveFirstMatchFromSlice is the same a RemoveFirstMatchFromSlice but preserves the order of the slice.
// It may require making a copy of the slice and so is less efficient
func RemoveFirstMatchFromSlicePreserveOrder(x interface{}, matches func(i int) bool) interface{} {
	v := reflect.ValueOf(x)
	length := v.Len()

	ndx := 0
	for ndx = 0; ndx < length; ndx++ {
		if matches(ndx) {
			break
		}
	}

	if ndx >= length {
		return x // Not found
	}

	last := length - 1
	if ndx != last {
		dst := v.Slice(ndx, length)
		src := v.Slice(ndx+1, length)
		reflect.Copy(dst, src)
	}

	return v.Slice(0, last).Interface()
}

func SliceContains(x interface{}, matches func(i int) bool) bool {
	v := reflect.ValueOf(x)
	length := v.Len()

	ndx := 0
	for ndx = 0; ndx < length; ndx++ {
		if matches(ndx) {
			return true
		}
	}

	return false
}

// FindAndMoveToEnd finds the first element in the slice `x` for which `matches` returns true and
// swaps it with the element at the end of the slice
func FindAndMoveToEnd(x interface{}, matches func(i int) bool) {
	v := reflect.ValueOf(x)
	length := v.Len()

	if length < 2 {
		return
	}

	last := length - 1
	ndx := 0
	for ndx < length {
		if matches(ndx) {
			tmp := v.Index(ndx).Interface()
			v.Index(ndx).Set(v.Index(last))
			v.Index(last).Set(reflect.ValueOf(tmp))
			break
		}
		ndx++
	}
}
