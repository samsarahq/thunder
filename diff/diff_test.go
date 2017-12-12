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

func TestDiffListRepeatedStrings(t *testing.T) {
	d := diff.Diff([]interface{}{
		"1",
		"2",
		"1",
	}, []interface{}{
		"1",
		"2",
		"1",
	})

	if internal.AsJSON(d) != nil {
		t.Errorf("expected no diff, but received %s", d)
	}
}

func TestDiffUpdateSliceToNil(t *testing.T) {
	var emptySlice []interface{}
	d := diff.Diff([]interface{}{
		"1",
		"2",
		"1",
	}, emptySlice)

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`[null]`)) {
		t.Errorf("expected no diff, but received %s", d)
	}
}

func TestDiffUpdateMapToNil(t *testing.T) {
	var emptyMap map[string]interface{}
	d := diff.Diff(map[string]interface{}{
		"__key": "a",
		"foo":   "bar",
	}, emptyMap)

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`[null]`)) {
		t.Errorf("expected no diff, but received %s", d)
	}
}

func TestStripKey(t *testing.T) {
	d := diff.StripKey(map[string]interface{}{
		"__key": "foo",
		"arr": []interface{}{
			"x",
			"y",
			"z",
			map[string]interface{}{
				"__key": "bar",
				"q":     "w",
			},
		},
	})

	if !reflect.DeepEqual(internal.AsJSON(d), internal.ParseJSON(`
		{"arr": ["x", "y", "z", {"q": "w"}]}
	`)) {
		t.Error("bad reorder")
	}
}

func TestDiffIntNullable(t *testing.T) {
	var testcases = []struct {
		desc         string
		left         interface{}
		right        interface{}
		expectedDiff interface{}
	}{
		{"both nil", nil, nil, nil},
		{"nil, one", nil, 1, 1},
		{"one, nil", 1, nil, []interface{}{nil}},
		{"both one", 1, 1, nil},
	}

	for _, tc := range testcases {
		d := diff.Diff(tc.left, tc.right)
		if !reflect.DeepEqual(internal.AsJSON(d), internal.AsJSON(tc.expectedDiff)) {
			t.Errorf("%s: bad diff: %s", tc.desc, d)
		}
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
