package graphql

import (
	"reflect"
	"testing"
	"time"

	"github.com/samsarahq/thunder/reactive/diff"
)

func TestAwait(t *testing.T) {
	start := time.Now()

	list := &awaitableDiffableList{}
	obj := &awaitableDiffableObject{
		Fields: map[string]interface{}{
			"a": "b",
			"b": list,
		},
	}

	for i := 0; i < 10; i++ {
		copy := i
		list.Items = append(list.Items, fork(func() (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return copy, nil
		}))
	}

	final, err := await(obj)
	if err != nil {
		t.Error(err)
	}

	duration := time.Since(start)
	if duration >= 100*time.Millisecond {
		t.Errorf("fork did not run in parallel: %v > 100ms", duration)
	}

	if !reflect.DeepEqual(final, &diff.Object{
		Fields: map[string]interface{}{
			"a": "b",
			"b": &diff.List{
				Items: []interface{}{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
		}}) {
		t.Errorf("bad final %v", final)
	}
}
