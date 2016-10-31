package reactive

import (
	"testing"
	"time"
)

func newNode() *node {
	return &node{
		out: make(map[*node]struct{}),
	}
}

func TestNodeBasic(t *testing.T) {
	a := newNode()
	b := newNode()

	invalidated := NewExpect()
	released := NewExpect()
	a.afterInvalidate = func() {
		invalidated.Trigger()
	}
	b.afterRelease = func() {
		released.Trigger()
	}

	b.addOut(a)

	b.invalidate()
	invalidated.Expect(t, "expected invalidate")

	a.release()
	released.Expect(t, "expected release")
}

func TestNodeAlreadyInvalidated(t *testing.T) {
	a := newNode()
	b := newNode()

	invalidated := NewExpect()
	released := NewExpect()
	a.afterInvalidate = func() {
		invalidated.Trigger()
	}
	b.afterRelease = func() {
		released.Trigger()
	}

	b.invalidate()

	b.addOut(a)

	invalidated.Expect(t, "expected invalidate")

	a.release()
	released.Expect(t, "expected release")
}

func TestNodeAlreadyReleased(t *testing.T) {
	a := newNode()
	b := newNode()

	released := NewExpect()
	b.afterRelease = func() {
		released.Trigger()
	}

	a.release()
	b.addOut(a)

	released.Expect(t, "expected release")
}

func TestNodeRefCount(t *testing.T) {
	a := newNode()
	aa := newNode()
	b := newNode()

	var released *Expect
	b.afterRelease = func() {
		released.Trigger()
	}

	b.addOut(a)
	// expected not yet released; if it runs, it will panic

	b.addOut(aa)
	a.release()
	// expected not yet released; if it runs, it will panic

	time.Sleep(100 * time.Millisecond)

	released = NewExpect()
	aa.release()
	released.Expect(t, "expected release")
}

func TestNodeChain(t *testing.T) {
	a := newNode()
	b := newNode()
	c := newNode()

	invalidated := NewExpect()
	released := NewExpect()
	a.afterInvalidate = func() {
		invalidated.Trigger()
	}
	c.afterRelease = func() {
		released.Trigger()
	}

	c.addOut(b)
	b.addOut(a)

	c.invalidate()
	invalidated.Expect(t, "expected invalidate")

	a.release()
	released.Expect(t, "expected release")
}
