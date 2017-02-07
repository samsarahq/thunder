package graphql

import "fmt"

type awaitable interface {
	await() (interface{}, error)
}

func await(value interface{}) (interface{}, error) {
	if f, ok := value.(awaitable); ok {
		return f.await()
	}
	return value, nil
}

type thunk struct {
	value interface{}
	err   error
	done  chan struct{}
}

func fork(f func() (interface{}, error)) *thunk {
	t := &thunk{
		done: make(chan struct{}, 0),
	}

	go func() {
		t.value, t.err = f()
		close(t.done)
	}()

	return t
}

func (t *thunk) await() (interface{}, error) {
	<-t.done
	return t.value, t.err
}

type awaitableDiffableObject DiffableObject

func (a *awaitableDiffableObject) await() (interface{}, error) {
	for k, v := range a.Fields {
		if f, ok := v.(awaitable); ok {
			value, err := f.await()
			if err != nil {
				return nil, nestPathError(k, err)
			}
			a.Fields[k] = value
		}
	}

	if f, ok := a.Key.(awaitable); ok {
		value, err := f.await()
		if err != nil {
			return nil, err
		}
		a.Key = value
	}

	return (*DiffableObject)(a), nil
}

type awaitableDiffableList DiffableList

func (a *awaitableDiffableList) await() (interface{}, error) {
	for i, v := range a.Items {
		if f, ok := v.(awaitable); ok {
			value, err := f.await()
			if err != nil {
				return nil, nestPathError(fmt.Sprint(i), err)
			}
			a.Items[i] = value
		}
	}
	return (*DiffableList)(a), nil
}
