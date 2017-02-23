package reactive

import "testing"

type user struct {
	name string
}

func TestDefensiveCopy(t *testing.T) {
	var err error
	m := map[string]interface{}{
		"foo":   "bar",
		"bar":   "baz",
		"int":   10,
		"int16": int16(10),
		"iface": &err,
		"slice": []int{1, 2, 3},
		"arr":   [...]string{"foo", "bar"},
		"u": &user{
			name: "bob",
		},
	}

	m["m"] = m
	m["mPtr"] = &m

	s1 := defensiveCopy(m)

	if !verifyDefensiveCopy(m, s1) {
		t.Error("expected equal")
	}

	m["u"].(*user).name = "alice"
	s2 := defensiveCopy(m)

	if !verifyDefensiveCopy(m, s2) {
		t.Error("expected equal")
	}
	if verifyDefensiveCopy(m, s1) {
		t.Error("expected unequal")
	}
}
