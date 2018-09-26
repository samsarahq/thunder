package merge_test

import (
	"reflect"
	"testing"

	"github.com/obad2015/thunder/internal"
	"github.com/obad2015/thunder/merge"
)

func TestMerge(t *testing.T) {
	cases := []struct {
		Case        string
		Prev        string
		Diff        string
		ExpectedNew string
	}{
		{
			Case:        "Scalar field",
			Prev:        `{"name": "bob"}`,
			Diff:        `{"name": "dean"}`,
			ExpectedNew: `{"name": "dean"}`,
		},
		{
			Case:        "Complex field",
			Prev:        `{"friends": ["alice", "charlie"]}`,
			Diff:        `{"friends": [["eli"]]}`,
			ExpectedNew: `{"friends": ["eli"]}`,
		},
		{
			Case:        "Array no reordering",
			Prev:        `[{"name": "bob", "age": 20}, {"name": "alice"}]`,
			Diff:        `{"0": {"name": "dean"}}`,
			ExpectedNew: `[{"name": "dean", "age": 20}, {"name": "alice"}]`,
		},
		{
			Case:        "Array with reordering",
			Prev:        `[{"name": "bob", "age": 20}, {"name": "alice"}]`,
			Diff:        `{"$": [1, 0], "1": {"age": 23}}`,
			ExpectedNew: `[{"name": "alice"}, {"name": "bob", "age": 23}]`,
		},
		{
			Case:        "Array with a run of reordering",
			Prev:        `[{"name": "alice"}, {"name": "bob"}, {"name": "carol"}, {"name": "dean"}]`,
			Diff:        `{"$": [[1, 3], -1], "3": [{"name": "eli"}]}`,
			ExpectedNew: `[{"name": "bob"}, {"name": "carol"}, {"name": "dean"}, {"name": "eli"}]`,
		},
		{
			Case:        "Map",
			Prev:        `{"name": "bob", "address": {"state": "ca", "city": "sf"}, "age": 30}`,
			Diff:        `{"name": "alice", "address": {"city": "oakland"}, "age": [], "friends": [["bob", "charlie"]]}`,
			ExpectedNew: `{"name": "alice", "address": {"state": "ca", "city": "oakland"}, "friends": ["bob", "charlie"]}`,
		},
	}

	for _, c := range cases {
		new, err := merge.Merge(internal.ParseJSON(c.Prev), internal.ParseJSON(c.Diff))
		if err != nil {
			t.Fatalf("%s failed. Merge error: %s", c.Case, err)
		}
		if !reflect.DeepEqual(new, internal.ParseJSON(c.ExpectedNew)) {
			t.Errorf("%s failed. expected: %v, got: %v", c.Case, c.ExpectedNew, new)
		}
	}
}
