package merge

import (
	"fmt"
	"reflect"
	"strconv"
)

const indicesReorderedKey = "$"

// Merge merges an update diff into the previous version of a JSON object,
// and returns the updated version.
//
// It effectively reverses the diff algorithm implemented in package diff.
func Merge(prev interface{}, diff interface{}) (interface{}, error) {
	d, ok := diff.(map[string]interface{})
	if !ok {
		return mergeReplaced(diff)
	}

	switch prev := prev.(type) {
	case map[string]interface{}:
		return mergeMap(prev, d)
	case []interface{}:
		return mergeArray(prev, d)
	}
	return nil, nil
}

// mergeMap applies a delta to a map.
func mergeMap(prev map[string]interface{}, diff map[string]interface{}) (map[string]interface{}, error) {
	new := make(map[string]interface{})

	// Update existing fields.
	for k, v := range prev {
		d, ok := diff[k]
		if !ok {
			// No change.
			new[k] = v
		} else if !isRemoved(d) {
			// Updated, but not removed.
			newV, err := Merge(v, d)
			if err != nil {
				return nil, err
			}
			new[k] = newV
		}
	}

	// Merge in new fields.
	for k, d := range diff {
		if _, ok := prev[k]; !ok {
			newV, err := mergeReplaced(d)
			if err != nil {
				return nil, err
			}
			new[k] = newV
		}
	}

	return new, nil
}

// mergeArray applies a delta to an array.
func mergeArray(prev []interface{}, diff map[string]interface{}) ([]interface{}, error) {
	var new []interface{}

	// Reorder elements if needed.
	if compressedIndices, ok := diff[indicesReorderedKey]; ok {
		reorderedIndices, err := uncompressIndices(compressedIndices)
		if err != nil {
			return nil, err
		}
		new = make([]interface{}, len(reorderedIndices))
		for i, index := range reorderedIndices {
			new[i] = prev[index]
		}
	} else {
		new = make([]interface{}, len(prev))
		for i := range prev {
			new[i] = prev[i]
		}
	}

	// Update complex elements.
	for k := range diff {
		if k == indicesReorderedKey {
			continue
		}

		d, ok := diff[k].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("mergeArray: diff is not a map. key: %s, diff: %v", k, diff[k])
		}

		index, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("mergeArray: key cannot be converted to an integer. key: %s", k)
		}

		v := new[index]
		newV, err := Merge(v, d)
		if err != nil {
			return nil, err
		}
		new[index] = newV
	}

	return new, nil
}

// mergeReplaced applies a replacement delta of a scalar or complex field.
func mergeReplaced(diff interface{}) (interface{}, error) {
	switch diff := diff.(type) {
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		// Pass through scalar values.
		return diff, nil
	default:
		// Extract all other values.
		d, ok := diff.([]interface{})
		if !ok || len(d) == 0 {
			return nil, fmt.Errorf("mergeReplaced: diff is not an array of length 1: %v", diff)
		}
		return d[0], nil
	}
}

// isRemoved determines if the delta represents a remove;
// removes are represented by an empty array.
func isRemoved(delta interface{}) bool {
	d, ok := delta.([]interface{})
	return (ok && len(d) == 0)
}

// uncompressIndices uncompresses the compressed representation of reordered indices,
// and returns the expanded new ordering.
func uncompressIndices(indices interface{}) ([]int, error) {
	compressedIndices, ok := indices.([]interface{})
	if !ok {
		return nil, fmt.Errorf("uncompressIndices: indices is not an array: %v", indices)
	}

	var uncompressedIndices []int
	for _, index := range compressedIndices {
		switch index := index.(type) {
		case []interface{}:
			if len(index) != 2 {
				return nil, fmt.Errorf("uncompressIndices: unexpected index array length: %v", index)
			}

			start, ok := index[0].(float64)
			if !ok {
				return nil, fmt.Errorf("uncompressIndices: index array[0] is not a number: %v", index[0])
			}

			end, ok := index[1].(float64)
			if !ok {
				return nil, fmt.Errorf("uncompressIndices: index array[1] is not a number: %v", index[1])
			}

			for i := start; i <= end; i++ {
				uncompressedIndices = append(uncompressedIndices, int(i))
			}
		case float64:
			uncompressedIndices = append(uncompressedIndices, int(index))
		default:
			return nil, fmt.Errorf("uncompressIndices: unexpected index type: %v", reflect.TypeOf(index))
		}
	}
	return uncompressedIndices, nil
}
