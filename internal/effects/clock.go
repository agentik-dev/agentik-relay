package effects

import (
	"sync"
	"time"
)

// SystemClock is the machine's clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time                         { return time.Now() }
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// StepClock is a clock a test moves (§15: time moves through the clock
// seam): Now reads T, After waits until Advance brings T to the deadline.
// Use it by pointer.
type StepClock struct {
	T time.Time

	mu      sync.Mutex
	waiters []stepWaiter
}

type stepWaiter struct {
	at time.Time
	ch chan time.Time
}

func (c *StepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.T
}

// After fires once T reaches now+d; at once when d is not positive.
func (c *StepClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.T
		return ch
	}
	c.waiters = append(c.waiters, stepWaiter{at: c.T.Add(d), ch: ch})
	return ch
}

// Advance moves T by d and fires every waiter whose deadline it reached.
func (c *StepClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.T = c.T.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if w.at.After(c.T) {
			kept = append(kept, w)
			continue
		}
		w.ch <- c.T
	}
	c.waiters = kept
}
