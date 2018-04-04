package handlers

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func GetThing(id int) int {
	time.Sleep(1 * time.Second)
	return -id
}

func GetThings(n int) []int {
	r := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r[i] = GetThing(i)
		}(i)
	}
	wg.Wait()
	return r
}

func TestStuff(t *testing.T) {
	ts := GetThings(5)
	if !reflect.DeepEqual(ts, []int{0, -1, -2, -3, -4}) {
		t.Errorf("Got %v", ts)
	}
}

func TestThings(t *testing.T) {
	c := make(chan int)
	b := make(chan bool)
	d := make(chan bool)
	go func() {
		<-b
		_, ok := <-c
		if ok {
			t.Fatal("Didn't expect to be able to read from c")
		}
		d <- true
	}()
	b <- true
	close(c)
	if !<-d {
		t.Fatal("expected true from d")
	}
}

// The problem is: you have a periodic job that may fail (and should exhibit
// exponential backoff when it does) and you want to allow callers to request
// that their callback is processed with the next instance of the job.
// Also, if a callback is added to the next sync and there is no next sync
// scheduled then this triggers a sync.

type syncer struct {
	cond *sync.Cond
}

func NewSyncer() *syncer {
	return &syncer{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (s *syncer) WaitForSync() {
	s.cond.L.Lock()
	s.cond.Wait()
	s.cond.L.Unlock()
}

func (s *syncer) Sync() {
	s.cond.Broadcast()
}

// How condition variables work.
// Each condition variable is associated with a lock. That lock is used to
// synchronise goroutines that are waiting on the condition - goroutines are
// required to hold the lock when making use of the condition.
// The idea here is that a sync.Cond guards a piece of state (i.e. is a lock),
// but also has a facility for allowing goroutines to Wait() on the lock
// becoming available.
// But how is that different to a regular lock? Because if you just have N
// goroutines waiting on a lock, how do you implement Signal()? You could simply
// relinquish the lock, and then all the Wait()ing goroutines would be able to
// proceed. However, if you then re-take the lock you will block goroutines that
// hadn't had a chance to execute in response to the previous Signal().
// So a sync.Cond lets an arbitrary number of goroutines block on a condition.
// We can wake one or all of them up, which will cause them to all attempt to
// enter the critical section guarded by the associated lock.

// For our purposes we want to allow callers to Wait() for the next time a sync
// is completed. So callers in fact don't need the lock at all and really just
// want to sleep until some event occurs.

// So a simple way to implement what we want is that waiters take the lock and
// wait on the condition. Then when the condition is signaled, they will all
// wake up and take the lock in turn and do their thing. In our case waiters can
// relinquish the lock immediately upon waking up.

// That's not all though, because after the sync begins we don't want any new
// waiters to proceed until the sync after. This means that the Sync() method
// should acquire the cond lock, signal that

func TestCond(t *testing.T) {
	c := make(chan int)
	go func() {
		time.Sleep(time.Second)
		c <- 10
		close(c)
	}()
	_, ok := <-c
	if !ok {
		t.Fatal("Expected read to block until either a value is received or channel is closed")
	}
}
