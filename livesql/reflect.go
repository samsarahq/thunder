package livesql

import "reflect"

// interfaceTyp is the reflect.Type of interface{}
var interfaceTyp reflect.Type

func init() {
	var x interface{}
	interfaceTyp = reflect.TypeOf(&x).Elem()
}

// toArray converts a []interface{} slice into an equivalent fixed-length array
// [...]interface{} for use as a comparable map key
//
func toArray(s []interface{}) interface{} {
	switch len(s) {
	// fast code paths for short arrays:
	case 0:
		return [...]interface{}{}
	case 1:
		return [...]interface{}{s[0]}
	case 2:
		return [...]interface{}{s[0], s[1]}
	case 3:
		return [...]interface{}{s[0], s[1], s[2]}
	case 4:
		return [...]interface{}{s[0], s[1], s[2], s[3]}
	case 5:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4]}
	case 6:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5]}
	case 7:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5], s[6]}
	case 8:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7]}
	default:
		// slow catch-all:
		array := reflect.New(reflect.ArrayOf(len(s), interfaceTyp)).Elem()
		for i, elem := range s {
			array.Index(i).Set(reflect.ValueOf(elem))
		}
		return array.Interface()
	}
}
