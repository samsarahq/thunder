package diff

import (
	"reflect"
	"testing"
)

func TestMergeReplaced(t *testing.T) {
	cases := []struct {
		Case        string
		Prev        map[string]interface{}
		Diff        map[string]interface{}
		ExpectedNew interface{}
	}{
		{
			Case:        "Scalar field",
			Prev:        map[string]interface{}{"name": "bob"},
			Diff:        map[string]interface{}{"name": "dean"},
			ExpectedNew: map[string]interface{}{"name": "dean"},
		},
		{
			Case: "Complex field",
			Prev: map[string]interface{}{
				"friends": []interface{}{"alice", "charlie"},
			},
			Diff: map[string]interface{}{
				"friends": []interface{}{
					[]interface{}{"eli"},
				},
			},
			ExpectedNew: map[string]interface{}{
				"friends": []interface{}{"eli"},
			},
		},
	}

	for _, c := range cases {
		new := Merge(c.Prev, c.Diff)
		if !reflect.DeepEqual(new, c.ExpectedNew) {
			t.Errorf("%s failed. expected: %v, got: %v", c.Case, c.ExpectedNew, new)
		}
	}
}

func TestMergeArray(t *testing.T) {
	cases := []struct {
		Case        string
		Prev        []interface{}
		Diff        map[string]interface{}
		ExpectedNew interface{}
	}{
		{
			Case: "No reordering",
			Prev: []interface{}{
				map[string]interface{}{"name": "bob", "age": 20},
				map[string]interface{}{"name": "alice"},
			},
			Diff: map[string]interface{}{
				"0": map[string]interface{}{"name": "dean"},
			},
			ExpectedNew: []interface{}{
				map[string]interface{}{"name": "dean", "age": 20},
				map[string]interface{}{"name": "alice"},
			},
		},
		{
			Case: "With reordering",
			Prev: []interface{}{
				map[string]interface{}{"name": "bob", "age": 20},
				map[string]interface{}{"name": "alice"},
			},
			Diff: map[string]interface{}{
				"$": []interface{}{1, 0},
				"1": map[string]interface{}{"age": 23},
			},
			ExpectedNew: []interface{}{
				map[string]interface{}{"name": "alice"},
				map[string]interface{}{"name": "bob", "age": 23},
			},
		},
	}

	for _, c := range cases {
		new := MergeArray(c.Prev, c.Diff)
		if !reflect.DeepEqual(new, c.ExpectedNew) {
			t.Errorf("%s failed. expected: %v, got: %v", c.Case, c.ExpectedNew, new)
		}
	}
}

func TestMerge(t *testing.T) {
	cases := []struct {
		Case        string
		Prev        map[string]interface{}
		Diff        map[string]interface{}
		ExpectedNew map[string]interface{}
	}{
		{
			Case: "Different field types",
			Prev: map[string]interface{}{
				"name":    "bob",
				"address": map[string]interface{}{"state": "ca", "city": "sf"},
				"age":     30,
			},
			Diff: map[string]interface{}{
				"name":    "alice",
				"address": map[string]interface{}{"city": "oakland"},
				"age":     []interface{}{},
				"friends": []interface{}{
					[]interface{}{"bob", "charlie"},
				},
			},
			ExpectedNew: map[string]interface{}{
				"name":    "alice",
				"address": map[string]interface{}{"state": "ca", "city": "oakland"},
				"friends": []interface{}{"bob", "charlie"},
			},
		},
	}

	for _, c := range cases {
		new := Merge(c.Prev, c.Diff)
		if !reflect.DeepEqual(new, c.ExpectedNew) {
			t.Errorf("%s failed. expected: %v, got: %v", c.Case, c.ExpectedNew, new)
		}
	}
}
