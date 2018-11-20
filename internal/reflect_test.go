package internal

import (
	"reflect"
	"testing"
)

func testMakeHashable(t *testing.T, s []interface{}, expected interface{}) {
	actual := MakeHashable(s)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("MakeHashable(%v) = %v, expected %v", s, actual, expected)
	}
}

func TestMakeHashable(t *testing.T) {
	testMakeHashable(t, nil, [0]interface{}{})
	testMakeHashable(t, []interface{}{}, [0]interface{}{})
	testMakeHashable(t, []interface{}{1}, [1]interface{}{1})
	testMakeHashable(t, []interface{}{1, 2}, [2]interface{}{1, 2})
	testMakeHashable(t, []interface{}{1, 2, 3}, [3]interface{}{1, 2, 3})
	testMakeHashable(t, []interface{}{1, 2, 3, 4}, [4]interface{}{1, 2, 3, 4})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5}, [5]interface{}{1, 2, 3, 4, 5})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5, 6}, [6]interface{}{1, 2, 3, 4, 5, 6})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5, 6, 7}, [7]interface{}{1, 2, 3, 4, 5, 6, 7})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8}, [8]interface{}{1, 2, 3, 4, 5, 6, 7, 8})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}, [9]interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9})
	testMakeHashable(t, []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, [10]interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// Byte slices are converted into strings.
	testMakeHashable(t, []interface{}{[]byte("hi"), []byte("there")}, [2]interface{}{"hi", "there"})
}
