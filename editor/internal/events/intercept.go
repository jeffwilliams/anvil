package events

import (
	"gioui.org/io/event"
	"gioui.org/layout"
)

type EventInterceptor struct {
	interceptors []Interceptor
}

// An Interceptor is an object that is interested in receiving events,
// and possibly consuming them, thus intercepting them before they are processed by
// the normal widget.
type Interceptor interface {
	InterceptEvent(gtx layout.Context, ev event.Event) (processed bool)
}

func (in *EventInterceptor) RegisterInterceptor(i Interceptor) {
	in.interceptors = append(in.interceptors, i)
}

func (in *EventInterceptor) Filter(gtx layout.Context, ev event.Event) (processed bool) {
	for _, i := range in.interceptors {
		processed = i.InterceptEvent(gtx, ev)
		if processed {
			return
		}
	}
	return
}
