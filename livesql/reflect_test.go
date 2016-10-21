package livesql

import (
	"reflect"
	"testing"
)

func testToArray(t *testing.T, s []interface{}, expected interface{}) {
	actual := toArray(s)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("toArray(%v) = %v, expected %v", s, actual, expected)
	}
}

func TestToArray(t *testing.T) {
	testToArray(t, nil, [0]interface{}{})
	testToArray(t, []interface{}{}, [0]interface{}{})
	testToArray(t, []interface{}{1}, [1]interface{}{1})
	testToArray(t, []interface{}{1, 2}, [2]interface{}{1, 2})
	testToArray(t, []interface{}{1, 2, 3}, [3]interface{}{1, 2, 3})
	testToArray(t, []interface{}{1, 2, 3, 4}, [4]interface{}{1, 2, 3, 4})
	testToArray(t, []interface{}{1, 2, 3, 4, 5}, [5]interface{}{1, 2, 3, 4, 5})
	testToArray(t, []interface{}{1, 2, 3, 4, 5, 6}, [6]interface{}{1, 2, 3, 4, 5, 6})
	testToArray(t, []interface{}{1, 2, 3, 4, 5, 6, 7}, [7]interface{}{1, 2, 3, 4, 5, 6, 7})
	testToArray(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8}, [8]interface{}{1, 2, 3, 4, 5, 6, 7, 8})
	testToArray(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}, [9]interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9})
	testToArray(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, [10]interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
}
