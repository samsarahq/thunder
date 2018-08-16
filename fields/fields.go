// The fields package is responsible for our interactions with struct fields.
// Its responsibilities are:
//   - Serializing and deserializing to and from SQL
//   - Copying to and from values while abstracting away pointer types and nil values.
package fields

// TagSet holds onto our tags for quick lookup.
// They are used to hang onto type overrides dictated by structs.
//
// In this example:
//   type Cat struct {
//     ID        int64 `sql:",primary"`
//     LifeStory Blob `sql:",binary,nullable"`
//   }
// LifeStory has a TagSet with the following:
//   TagSet{"binary":{}, "nullable":{}}
type TagSet map[string]struct{}

func newTagSet(tags ...string) TagSet {
	set := make(TagSet, len(tags))
	for _, tag := range tags {
		set[tag] = struct{}{}
	}
	return set
}

// Contains returns true if the set contains the tag.
func (t TagSet) Contains(tag string) bool {
	_, ok := t[tag]
	return ok
}
