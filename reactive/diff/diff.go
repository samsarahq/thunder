package diff

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// A Diffable type can compute a delta from an old value
//
// old is not guaranteed to be the same type as the callee
type Diffable interface {
	Diff(old interface{}) (delta interface{}, changed bool)
}

// Diff computes a deltas between two values
//
// All values passed into Diff must be comparable
func Diff(old interface{}, new interface{}) (interface{}, bool) {
	if old == new {
		return nil, false
	}

	if new, ok := new.(Diffable); ok {
		return new.Diff(old)
	}

	return new, true
}

// Update is a type of delta
//
// Deltas represent updates to JSON trees. An updates modifies the named keys
// from a container.
type Update map[string]interface{}

// Delete is a type of delta
//
// A Delete{} deletes a value from its container.
type Delete struct{}

// MarshalJSON implements the json.Marshal interface.
func (d Delete) MarshalJSON() ([]byte, error) {
	return []byte("[]"), nil
}

// tagged is a helper to wrap a value in array brackets when marshaled
type tagged []interface{}

// untagged is a helper to signal to PrepareForMarshal that a value should not
// be tagged with array brackets
type untagged struct {
	inner interface{}
}

// PrepareForMarshal converts a delta into a value ready to be sent over a JSON
// socket
//
// PrepareForMarshal passes Update and Delte objects straight through. All
// other values that could look like deltas are tagged by wrapping them in []
// array brackets so the receiver can distinguish them from deltas.
func PrepareForMarshal(delta interface{}) interface{} {
	switch delta := delta.(type) {
	case Update:
		// recursively wrap values inside of an Update
		for k, v := range delta {
			delta[k] = PrepareForMarshal(v)
		}
		return delta

	case Delete:
		// pass through the Delete{}
		return delta

	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		// pass through values that don't look like deltas
		return delta

	case untagged:
		return delta.inner

	default:
		// wrap all other values
		return tagged{delta}
	}
}

// A Object is a diffable value representing a JSON object
//
// Normal map[string]interface{} are not diffable because they are not
// comparable (that is, Go panics when you try a == b).  Additionally,
// Objects have a Key useful for lining up objects in two different
// arrays.
type Object struct {
	Key    interface{}
	Fields map[string]interface{}
}

// MarshalJSON implements the json.Marshal interface.
func (o *Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.Fields)
}

// Diff implements the Diffable interface.
//
// An object delta consists of a delta for each changed child element
func (o *Object) Diff(old interface{}) (interface{}, bool) {
	oldObject, ok := old.(*Object)
	if !ok || o.Key != oldObject.Key {
		return o, true
	}
	oldFields, newFields := oldObject.Fields, o.Fields

	delta := make(Update)
	// find deleted fields
	for k := range oldFields {
		if _, ok := newFields[k]; !ok {
			delta[k] = Delete{}
		}
	}
	// find changed and added fields
	for k := range newFields {
		if _, ok := oldFields[k]; ok {
			if d, changed := Diff(oldFields[k], newFields[k]); changed {
				delta[k] = d
			}

		} else {
			delta[k] = newFields[k]
		}
	}

	if len(delta) == 0 {
		return nil, false
	}
	return delta, true
}

type List struct {
	Items []interface{}
}

// MarshalJSON implements the json.Marshal interface.
func (l *List) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.Items)
}

func reorderKey(i interface{}) interface{} {
	if object, ok := i.(*Object); ok && object.Key != "" {
		return object.Key
	}
	if reflect.TypeOf(i).Comparable() {
		return i
	}
	return nil
}

// computeReorderIndices returns an array containing the index in old of each
// item in new
//
// If an item in new is not present in old, the index is -1. Objects are
// identified using the Key field.
func computeReorderIndices(oldItems, newItems []interface{}) []int {
	oldIndices := make(map[interface{}]int)
	for i, item := range oldItems {
		oldIndices[reorderKey(item)] = i
	}

	indices := make([]int, len(newItems))
	for i, item := range newItems {
		if index, ok := oldIndices[reorderKey(item)]; ok {
			indices[i] = index
		} else {
			indices[i] = -1
		}
	}

	return indices
}

// compressReorderIndices compresses a set of indices
//
// Runs of incrementing non-negative consecutive values are represented by a
// two-element array [first, count].
func compressReorderIndices(indices []int) []interface{} {
	compressed := make([]interface{}, 0)
	i := 0
	for i < len(indices) {
		// j represents the end of the current run
		j := i
		for j < len(indices) && indices[j] != -1 && indices[j]-indices[i] == j-i {
			// increment j while the run continues
			j++
		}

		if i == j {
			compressed = append(compressed, -1)
			i = j + 1
		} else if j-i == 1 {
			compressed = append(compressed, indices[i])
			i = j
		} else {
			compressed = append(compressed, [2]int{indices[i], j - i})
			i = j
		}
	}

	return compressed
}

// Diff implements the Diffable interface.
//
// A list delta consists of a set of reorder indices stored in $ (as in
// computeReorderIndices) and for each new index an optional delta with the
// previous value.
func (l *List) Diff(old interface{}) (interface{}, bool) {
	oldList, ok := old.(*List)
	if !ok {
		return l, true
	}
	oldItems, newItems := oldList.Items, l.Items

	delta := make(Update)

	// compute reorder indices
	indices := computeReorderIndices(oldItems, newItems)

	// same order
	orderChanged := len(oldItems) != len(newItems)
	for i := range newItems {
		if indices[i] != i {
			orderChanged = true
		}
	}
	if orderChanged {
		compressed := compressReorderIndices(indices)
		delta["$"] = untagged{inner: compressed}
	}

	// compute child deltas
	for i, new := range newItems {
		var old interface{}
		if indices[i] != -1 {
			old = oldItems[indices[i]]
		}

		if d, changed := Diff(old, new); changed {
			delta[fmt.Sprint(i)] = d
		}
	}

	// an empty delta modifies no objects and reorders no items
	if len(delta) == 0 {
		return nil, false
	}
	return delta, true
}
