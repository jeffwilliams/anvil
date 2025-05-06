package debug

import (
	"bytes"
	"container/list"
	"fmt"
	"strings"
	"sync"
	"time"
)

type DebugLog struct {
	entries map[string]*list.List
	max     int
	lock    sync.Mutex
}

type entry struct {
	when     time.Time
	category string
	message  string
	flag     bool
}

func New(maxEntries int) *DebugLog {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &DebugLog{max: maxEntries}
}

func (l *DebugLog) Addf(category, message string, args ...interface{}) {
	m := fmt.Sprintf(message, args...)
	l.Add(category, m)
}

func (l *DebugLog) Add(category, message string) {
	l.lock.Lock()
	c := l.listForCategory(category)
	c.PushBack(&entry{time.Now(), category, message, false})
	if c.Len() > l.max && c.Front() != nil {
		c.Remove(c.Front())
	}
	l.lock.Unlock()
}

func (l *DebugLog) getEntries() map[string]*list.List {
	if l.entries == nil {
		l.entries = make(map[string]*list.List)
	}
	return l.entries
}

func (l *DebugLog) listForCategory(category string) *list.List {
	c, ok := l.getEntries()[category]
	if !ok {
		c = list.New()
		l.getEntries()[category] = c
	}
	return c
}

func (l *DebugLog) Categories() []string {
	l.lock.Lock()
	defer l.lock.Unlock()

	entries := l.getEntries()
	c := make([]string, len(entries))

	i := 0
	for k := range entries {
		c[i] = k
		i++
	}

	return c
}

// Merge together the logs from all the categories into one multi-line log.
// mark entries that are the start of a category with a special marker
// Format:
// 2022-05-21T12:43:12:123 <category><first> Message
func (l *DebugLog) String(categories ...string) string {
	var lists []*list.List

	l.lock.Lock()
	defer l.lock.Unlock()

	if len(categories) > 0 {
		lists = l.listsForCategories(categories...)
	} else {
		lists = l.lists()
	}

	var buf bytes.Buffer

	fronts := fronts(lists)
	for _, f := range fronts {
		f.Value.(*entry).flag = true
	}

	for {
		nxt, first := popLowest(fronts)
		if nxt == nil {
			break
		}

		s := format(nxt, first)
		buf.WriteString(format(nxt, first))
		if !strings.HasSuffix(s, "\n") {
			buf.WriteRune('\n')
		}
	}

	return buf.String()
}

func (l *DebugLog) lists() []*list.List {
	entries := l.getEntries()
	lists := make([]*list.List, 0, len(entries))
	for _, v := range entries {
		lists = append(lists, v)
	}
	return lists
}

func (l *DebugLog) listsForCategories(categories ...string) []*list.List {
	var lists []*list.List

	entries := l.getEntries()
	for _, cat := range categories {
		if c, ok := entries[cat]; ok {
			lists = append(lists, c)
		}
	}
	return lists
}

func fronts(lists []*list.List) []*list.Element {

	var fronts []*list.Element
	for _, c := range lists {
		fronts = append(fronts, c.Front())
	}
	return fronts
}

func popLowest(fronts []*list.Element) (smallest *entry, isFirstInList bool) {
	smallest = (*entry)(nil)
	smallestElem := (*list.Element)(nil)
	smallestIndex := -1

	for i, f := range fronts {
		if f == nil {
			continue
		}

		e := f.Value.(*entry)
		if smallest == nil || e.when.Before(smallest.when) {
			smallest = e
			smallestIndex = i
			smallestElem = f
		}
	}

	if smallestIndex >= 0 {
		fronts[smallestIndex] = smallestElem.Next()
		isFirstInList = smallest.flag
		smallest.flag = false
	}

	return
}

func format(e *entry, first bool) string {
	f := ""
	if first {
		f = "<first>"
	}
	return fmt.Sprintf("%s <%s>%s %s", e.when.Format("2006-01-02T15:04:05.000"), e.category, f, e.message)
}
