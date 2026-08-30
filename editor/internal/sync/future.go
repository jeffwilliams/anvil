package sync

type future struct {
	finished chan struct{}
}

func NewFuture() Future {
	return future{finished: make(chan struct{})}
}

func (f future) Wait() {
	<-f.finished
}

func (f future) Done() {
	close(f.finished)
}

type Future interface {
	Wait()
	Done()
}

type completedFuture struct {
}

func (f completedFuture) Wait() {
}

func (f completedFuture) Done() {
}

var CompletedFuture completedFuture

type CompoundFuture []Future

func (cf CompoundFuture) Wait() {
	for _, f := range cf {
		f.Wait()
	}
}

func (cf CompoundFuture) Done() {
}
