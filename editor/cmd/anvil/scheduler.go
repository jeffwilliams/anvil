package main

import (
	"time"
)

// A Scheduler schedules some work to happen in the main editor thread.
type Scheduler struct {
	work   chan Work
	timers map[string]*time.Timer
}

func NewScheduler(work chan Work) *Scheduler {
	return &Scheduler{
		work: work,
	}
}

// AfterFunc waits for the duration to elapse and then calls f in its own goroutine.
// If there is already a timer started for "id" it is stopped and a new one created (the durection is reset).
func (s *Scheduler) AfterFunc(id string, d time.Duration, f func()) {
	s.init()
	t, ok := s.timers[id]
	if ok {
		return
	}

	t = time.AfterFunc(d, func() {
		s.work <- scheduledWork{f, id, s}
	})
	s.timers[id] = t
}

func (s *Scheduler) init() {
	if s.timers == nil {
		s.timers = make(map[string]*time.Timer)
	}
}

func (s *Scheduler) stopTimerIfAlreadyCreated(id string) {
	t, ok := s.timers[id]
	if ok {
		/*if !t.Stop() {
			<-t.C
		}*/
		t.Stop()
	}
}

type scheduledWork struct {
	f  func()
	id string
	s  *Scheduler
}

func (w scheduledWork) Service() (done bool) {
	w.f()
	delete(w.s.timers, w.id)
	return true
}

func (w scheduledWork) Job() Job {
	return nil
}

type basicWork struct {
	f func()
}

func (w basicWork) Service() (done bool) {
	w.f()
	return true
}

func (w basicWork) Job() Job {
	return nil
}
