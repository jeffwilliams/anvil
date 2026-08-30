package main

import (
	"sort"
	"sync"
)

type IdGen struct {
	next int
	free []int
	lock sync.Mutex
}

func (g *IdGen) Get() int {
	g.lock.Lock()
	defer g.lock.Unlock()

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
	g.lock.Lock()
	defer g.lock.Unlock()

	for _, v := range g.free {
		if v == id {
			log(LogCatgWin, "multiple free for window id %d\n", id)
			return
		}
	}

	g.free = append(g.free, id)
}
