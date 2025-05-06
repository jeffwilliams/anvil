package main

import "testing"

func TestOverlap(t *testing.T) {
	// [20,25) (half open)
	s1 := textRange{20, 25}
	s2 := textRange{30, 40}
	s3 := textRange{25, 27}
	s4 := textRange{15, 20}
	s5 := textRange{15, 27}
	s6 := textRange{10, 11}

	if s1.Overlaps(&s2) {
		t.Fatalf("selection %s overlaps %s\n", s1, s2)
	}
	if s1.Overlaps(&s3) {
		t.Fatalf("selection %s overlaps %s\n", s1, s3)
	}
	if s1.Overlaps(&s4) {
		t.Fatalf("selection %s overlaps %s\n", s1, s4)
	}
	if !s1.Overlaps(&s5) {
		t.Fatalf("selection %s does not overlap %s\n", s1, s5)
	}
	if !s5.Overlaps(&s1) {
		t.Fatalf("selection %s does not overlap %s\n", s5, s1)
	}
	if s1.Overlaps(&s6) {
		t.Fatalf("selection %s does not overlap %s\n", s1, s6)
	}

}
