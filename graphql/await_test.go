package graphql

import (
	"reflect"
	"testing"
	"time"
)

func TestAwait(t *testing.T) {
	start := time.Now()

	list := []interface{}{}
	for i := 0; i < 10; i++ {
		copy := i
		list = append(list, fork(func() (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return copy, nil
		}))
	}
	obj := map[string]interface{}{
		"a": "b",
		"b": list,
	}

	final, err := await(obj)
	if err != nil {
		t.Error(err)
	}

	duration := time.Since(start)
	if duration >= 100*time.Millisecond {
		t.Errorf("fork did not run in parallel: %v > 100ms", duration)
	}

	if !reflect.DeepEqual(final, map[string]interface{}{
		"a": "b",
		"b": []interface{}{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	}) {
		t.Errorf("bad final %v", final)
	}
}
