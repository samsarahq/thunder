package fields

// These interfaces specifically supports gogo/protobuf - though there are other libraries
// that implement them as well.

type marshaler interface {
	Marshal() ([]byte, error)
}

type unmarshaler interface {
	Unmarshal([]byte) error
}
