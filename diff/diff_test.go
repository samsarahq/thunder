package diff_test

import (
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/diff"
	"github.com/samsarahq/thunder/internal"
)

func TestDiffListString(t *testing.T) {
	d := diff.Diff([]interface{}{
		"0",
		"1",
		"2",
		"3",
	}, []interface{}{
		"3",
		"-1",
		"0",
		"1",
		"4",
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"$": [3, -1, [0, 2], -1], "1": "-1", "4": "4"}
	`)) {
		t.Error("bad reorder")
	}
}

func TestDiffListOrder(t *testing.T) {
	d := diff.Diff([]interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "2"},
		map[string]interface{}{"__key": "3"},
	}, []interface{}{
		map[string]interface{}{"__key": "3"},
		map[string]interface{}{"__key": "-1"},
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "4"},
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"$": [3, -1, [0, 2], -1], "1": [{}], "4": [{}]}
	`)) {
		t.Error("bad reorder")
	}

	d = diff.Diff([]interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "2"},
		map[string]interface{}{"__key": "3"},
	}, []interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "2"},
		map[string]interface{}{"__key": "3"},
	})
	if d != nil {
		t.Error("bad identical")
	}

	d = diff.Diff([]interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "2"},
		map[string]interface{}{"__key": "3"},
	}, []interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"$": [[0, 2]]}
	`)) {
		t.Error("bad truncated")
	}

	d = diff.Diff([]interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
	}, []interface{}{
		map[string]interface{}{"__key": "0"},
		map[string]interface{}{"__key": "1"},
		map[string]interface{}{"__key": "2"},
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"$": [[0, 2], -1], "2": [{}]}
	`)) {
		t.Error("bad appended")
	}
}

func TestDiffObjects(t *testing.T) {
	d := diff.Diff(map[string]interface{}{
		"__key":   "a",
		"changed": 0,
		"removed": "foo",
		"same":    "bar",
	}, map[string]interface{}{
		"__key":   "a",
		"changed": 1,
		"same":    "bar",
	})
	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"changed": 1, "removed": []}
	`)) {
		t.Error("bad diff")
	}

	d = diff.Diff(map[string]interface{}{
		"__key": "a",
		"foo":   "bar",
	}, map[string]interface{}{
		"__key": "b",
		"foo":   "bar",
	})
	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		[{"foo": "bar"}]
	`)) {
		t.Error("bad changed key")
	}

	d = diff.Diff(map[string]interface{}{
		"__key": "a",
		"foo":   "bar",
	}, map[string]interface{}{
		"__key": "a",
		"foo":   "bar",
	})
	if d != nil {
		t.Error("bad identical")
	}
}

func TestKitchenSink(t *testing.T) {
	d := diff.Diff(map[string]interface{}{
		"__key": "a",
		"users": []interface{}{
			map[string]interface{}{
				"__key": "alice",
				"age":   30,
				"address": map[string]interface{}{
					"__key": "10",
					"city":  "sf",
				},
			},
			map[string]interface{}{
				"__key": "bob",
				"age":   300,
			},
			map[string]interface{}{
				"__key": "charlie",
				"age":   3000,
			},
		},
		"foo": "bar",
	}, map[string]interface{}{
		"__key": "a",
		"users": []interface{}{
			map[string]interface{}{
				"__key": "bob",
				"age":   300,
			},
			map[string]interface{}{
				"__key": "alice",
				"age":   30000,
				"address": map[string]interface{}{
					"__key": "10",
					"city":  "berkeley",
				},
			},
		},
		"bar": "baz",
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"foo": [], "bar": "baz", "users": {
			"$": [1, 0],
			"1": {"age": 30000, "address": {"city": "berkeley"}}
		}}
	`)) {
		t.Error("bad kitchen sink")
	}
}
