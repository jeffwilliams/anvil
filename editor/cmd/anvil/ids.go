package main

import "sort"

type IdGen struct {
	next int
	free []int
}

func (g *IdGen) Get() int {
	if len(g.free) == 0 {
		n := g.next
		g.next++
		return n
	}

	sort.Ints(g.free)
	n := g.free[0]
	g.free = g.free[1:]
	return n
}

func (g *IdGen) Free(id int) {
	g.free = append(g.free, id)
}
