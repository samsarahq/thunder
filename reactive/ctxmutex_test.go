package reactive

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// HammerMutex is copied from https://golang.org/src/sync/mutex_test.go.
func HammerMutex(m *ctxMutex, loops int) {
	for i := 0; i < loops; i++ {
		m.Lock(context.Background())
		m.Unlock()
	}
}

func TestMutex(t *testing.T) {
	var m ctxMutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			HammerMutex(&m, 1000)
		}()
	}
	wg.Wait()
}

func TestMutexMutualExclusivity(t *testing.T) {
	var m ctxMutex
	var wg sync.WaitGroup

	var c int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				m.Lock(context.Background())
				if atomic.AddInt64(&c, 1) != 1 {
					t.Error("more than one goroutine in critical section")
				}
				atomic.AddInt64(&c, -1)
				m.Unlock()
			}
		}()
	}
	wg.Wait()
}

func TestMutexLockTimeout(t *testing.T) {
	var m ctxMutex
	m.Lock(context.Background())
	defer m.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.Equal(t, m.Lock(ctx), context.DeadlineExceeded)
}

func TestMutexLockCanceled(t *testing.T) {
	var m ctxMutex
	_ = m.Lock(context.Background()) // Grab the lock
	defer m.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, m.Lock(ctx), context.Canceled)
}

func TestMutexUnlockBeforeLock(t *testing.T) {
	var m ctxMutex
	require.PanicsWithValue(t, "Unlock called before Lock", func() {
		m.Unlock()
	})
}
