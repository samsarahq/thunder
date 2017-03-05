package diff_test

import (
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/internal"
	"github.com/samsarahq/thunder/reactive/diff"
)

func obj(key string, fields map[string]interface{}) *diff.Object {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	return &diff.Object{
		Key:    key,
		Fields: fields,
	}
}

func list(items ...interface{}) *diff.List {
	return &diff.List{
		Items: items,
	}
}

func TestDiffListString(t *testing.T) {
	delta, _ := diff.Diff(list(
		"0",
		"1",
		"2",
		"3",
	), list(
		"3",
		"-1",
		"0",
		"1",
		"4",
	))

	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"$": [3, -1, [0, 2], -1], "1": "-1", "4": "4"}
	`)) {
		t.Error("bad reorder")
	}
}

func TestDiffListOrder(t *testing.T) {
	delta, _ := diff.Diff(list(
		obj("0", nil),
		obj("1", nil),
		obj("2", nil),
		obj("3", nil),
	), list(
		obj("3", nil),
		obj("-1", nil),
		obj("0", nil),
		obj("1", nil),
		obj("4", nil),
	))

	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"$": [3, -1, [0, 2], -1], "1": [{}], "4": [{}]}
	`)) {
		t.Error("bad reorder")
	}

	_, changed := diff.Diff(list(
		obj("0", nil),
		obj("1", nil),
		obj("2", nil),
		obj("3", nil),
	), list(
		obj("0", nil),
		obj("1", nil),
		obj("2", nil),
		obj("3", nil),
	))
	if changed {
		t.Error("bad identical")
	}

	delta, _ = diff.Diff(list(
		obj("0", nil),
		obj("1", nil),
		obj("2", nil),
		obj("3", nil),
	), list(
		obj("0", nil),
		obj("1", nil),
	))

	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"$": [[0, 2]]}
	`)) {
		t.Error("bad truncated")
	}

	delta, _ = diff.Diff(list(
		obj("0", nil),
		obj("1", nil),
	), list(
		obj("0", nil),
		obj("1", nil),
		obj("2", nil),
	))

	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"$": [[0, 2], -1], "2": [{}]}
	`)) {
		t.Error("bad appended")
	}
}

func TestDiffObjects(t *testing.T) {
	delta, _ := diff.Diff(obj("a", map[string]interface{}{
		"changed": 0,
		"removed": "foo",
		"same":    "bar",
	}), obj("a", map[string]interface{}{
		"changed": 1,
		"same":    "bar",
	}))
	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"changed": 1, "removed": []}
	`)) {
		t.Error("bad diff")
	}

	delta, _ = diff.Diff(obj("a", map[string]interface{}{
		"foo": "bar",
	}), obj("b", map[string]interface{}{
		"foo": "bar",
	}))
	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		[{"foo": "bar"}]
	`)) {
		t.Error("bad changed key")
	}

	_, changed := diff.Diff(obj("a", map[string]interface{}{
		"foo": "bar",
	}), obj("a", map[string]interface{}{
		"foo": "bar",
	}))
	if changed {
		t.Error("bad identical")
	}

}

func TestKitchenSink(t *testing.T) {
	delta, _ := diff.Diff(obj("a", map[string]interface{}{
		"users": list(
			obj("alice", map[string]interface{}{
				"age": 30,
				"address": obj("10", map[string]interface{}{
					"city": "sf",
				}),
			}),
			obj("bob", map[string]interface{}{
				"age": 300,
			}),
			obj("charlie", map[string]interface{}{
				"age": 3000,
			}),
		),
		"foo": "bar",
	}), obj("a", map[string]interface{}{
		"users": list(
			obj("bob", map[string]interface{}{
				"age": 300,
			}),
			obj("alice", map[string]interface{}{
				"age": 30000,
				"address": obj("10", map[string]interface{}{
					"city": "berkeley",
				}),
			}),
		),
		"bar": "baz",
	}))

	if !reflect.DeepEqual(internal.AsJSON(diff.PrepareForMarshal(delta)), internal.ParseJSON(`
		{"foo": [], "bar": "baz", "users": {
			"$": [1, 0],
			"1": {"age": 30000, "address": {"city": "berkeley"}}
		}}
	`)) {
		t.Error("bad kitchen sink")
	}
}
