package sync

import (
	"sync"
	"testing"
)

func TestFutureWaitBeforeDone(t *testing.T) {

	f := NewFuture()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		f.Wait()
		wg.Done()
	}()

	wg.Done()

	wg.Wait()
}

func TestFutureDoneBeforeWait(t *testing.T) {
	f := NewFuture()

	f.Done()
	f.Wait()
}
