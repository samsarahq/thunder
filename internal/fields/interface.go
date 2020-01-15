package fields

// These interfaces supports any type that implements Marshal/Unmarshal methods.

type marshaler interface {
	Marshal() ([]byte, error)
}

type unmarshaler interface {
	Unmarshal([]byte) error
}
