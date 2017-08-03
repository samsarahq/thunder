package diff

import (
	"log"
	"strconv"
)

const indicesReorderedKey = "$"

// Merge merges an update diff into the previous version of a JSON object,
// and returns the updated version.
//
// It effectively reverses the diff algorithm implemented in package diff.
func Merge(prev map[string]interface{}, diff map[string]interface{}) interface{} {
	new := make(map[string]interface{})

	// Update exisiting fields.
	for k, v := range prev {
		d, ok := diff[k]
		// No change.
		if !ok {
			new[k] = v
			continue
		}
		// Removed.
		if isRemoved(d) {
			continue
		}
		// Updated.
		var newV interface{}
		switch v := v.(type) {
		case map[string]interface{}:
			newV = MergeMap(v, d)
		case []interface{}:
			newV = MergeArray(v, d)
		default:
			newV = MergeReplaced(d)
		}
		new[k] = newV
	}

	// Merge in new fields.
	for k, d := range diff {
		if _, ok := prev[k]; !ok {
			new[k] = MergeReplaced(d)
		}
	}

	return new
}

// MergeMap applies a delta to a map field.
func MergeMap(prev map[string]interface{}, delta interface{}) interface{} {
	d, ok := delta.(map[string]interface{})
	if !ok {
		return MergeReplaced(delta)
	}
	return Merge(prev, d)
}

// MergeArray applies a delta to an array field.
func MergeArray(prev []interface{}, delta interface{}) interface{} {
	d, ok := delta.(map[string]interface{})
	if !ok {
		return MergeReplaced(delta)
	}

	// Reorder elements if needed.
	var new []interface{}
	if compressedIndices, ok := d[indicesReorderedKey]; ok {
		reorderedIndices := uncompressIndices(compressedIndices)
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
	for k := range d {
		if k == indicesReorderedKey {
			continue
		}

		diff, ok := d[k].(map[string]interface{})
		if !ok {
			log.Printf("MergeArray: diff of key %s is not a map[string]interface{}: %v",
				k, d[k])
			return nil
		}

		index, err := strconv.Atoi(k)
		if err != nil {
			log.Printf("MergeArray: key %s cannot be converted to an integer", k)
			return nil
		}

		v, ok := new[index].(map[string]interface{})
		if !ok {
			log.Printf("MergeArray: value of key %s is not a map[string]interface{}: %v",
				k, new[index])
			return nil
		}

		newV := Merge(v, diff)
		new[index] = newV
	}

	return new
}

// MergeReplaced applies a replacement delta of a scalar or complex field.
func MergeReplaced(diff interface{}) interface{} {
	switch diff := diff.(type) {
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		// Pass through scalar values.
		return diff
	default:
		// Extract all other values.
		d, ok := diff.([]interface{})
		if !ok || len(d) == 0 {
			log.Printf("MergeReplaced: diff is not an array of length 1: %v", diff)
			return nil
		}
		return d[0]
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
func uncompressIndices(indices interface{}) []int {
	compressedIndices, ok := indices.([]interface{})
	if !ok {
		log.Printf("uncompressIndices: indices is not an array: %v", indices)
		return nil
	}

	var uncompressedIndices []int
	for _, index := range compressedIndices {
		switch index := index.(type) {
		case []int:
			if len(index) == 2 {
				for i := index[0]; i <= index[1]; i++ {
					uncompressedIndices = append(uncompressedIndices, i)
				}
			} else {
				log.Printf("uncompressIndices: unexpected index array length: %v", index)
				return nil
			}
		case int:
			uncompressedIndices = append(uncompressedIndices, index)
		default:
			log.Printf("uncompressIndices: unexpected index type: %v", index)
			return nil
		}
	}
	return uncompressedIndices
}
