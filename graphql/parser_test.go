package graphql_test

import (
	"reflect"
	"testing"

	. "github.com/samsarahq/thunder/graphql"
)

func TestParseSupported(t *testing.T) {
	selections, err := Parse(`
{
	foo {
		alias: bar
		alias: bar
		baz(arg: 3) {
			bah(x: 1, y: "123", z: true)
			hum(foo: {x: $var})
		}
		... on Foo {
			asd
			... Bar
		}
	}
	xyz
}

fragment Bar on Foo {
	zxc
}`, map[string]interface{}{
		"var": "var value!!",
	})
	if err != nil {
		t.Error("unexpected error", err)
	}

	expected := &SelectionSet{
		Complex: true,
		Selections: []*Selection{
			{
				Name:  "foo",
				Alias: "foo",
				Args:  map[string]interface{}{},
				SelectionSet: &SelectionSet{
					Complex: true,
					Selections: []*Selection{
						{
							Name:  "bar",
							Alias: "alias",
							Args:  map[string]interface{}{},
						},
						{
							Name:  "bar",
							Alias: "alias",
							Args:  map[string]interface{}{},
						},
						{
							Name:  "baz",
							Alias: "baz",
							Args: map[string]interface{}{
								"arg": float64(3),
							},
							SelectionSet: &SelectionSet{
								Selections: []*Selection{
									{
										Name:  "bah",
										Alias: "bah",
										Args: map[string]interface{}{
											"x": float64(1),
											"y": "123",
											"z": true,
										},
									},
									{
										Name:  "hum",
										Alias: "hum",
										Args: map[string]interface{}{
											"foo": map[string]interface{}{
												"x": "var value!!",
											},
										},
									},
								},
							},
						},
					},
					Fragments: []*Fragment{
						{
							On: "Foo",
							SelectionSet: &SelectionSet{
								Selections: []*Selection{
									{
										Name:  "asd",
										Alias: "asd",
										Args:  map[string]interface{}{},
									},
								},
								Fragments: []*Fragment{
									{
										On: "Foo",
										SelectionSet: &SelectionSet{
											Selections: []*Selection{
												{
													Name:  "zxc",
													Alias: "zxc",
													Args:  map[string]interface{}{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Name:  "xyz",
				Alias: "xyz",
				Args:  map[string]interface{}{},
			},
		},
	}

	if !reflect.DeepEqual(selections, expected) {
		t.Error("unexpected parse")
	}
}

func TestParseUnsupported(t *testing.T) {
	_, err := Parse(`
mutation foo {
	bar
}`, map[string]interface{}{})
	if err == nil || err.Error() != "only support queries" {
		t.Error("expected mutation to fail", err)
	}

	_, err = Parse(``, map[string]interface{}{})
	if err == nil || err.Error() != "must have a single query" {
		t.Error("expected missing query to fail", err)
	}

	_, err = Parse(`
{
	bar
}

{
	baz
}`, map[string]interface{}{})
	if err == nil || err.Error() != "only support a single query" {
		t.Error("expected multiple queries to fail", err)
	}

	_, err = Parse(`
{
	b(a: 1)
	b(a: 2)
}`, map[string]interface{}{})
	if err == nil || err.Error() != "same alias with different args" {
		t.Error("expected different args to fail", err)
	}

	_, err = Parse(`
{
	a: a
	a: b
}`, map[string]interface{}{})
	if err == nil || err.Error() != "same alias with different name" {
		t.Error("expected different names to fail", err)
	}

	_, err = Parse(`
{
	a: a
	... on Foo {
		a: b
	}
}`, map[string]interface{}{})
	if err == nil || err.Error() != "same alias with different name" {
		t.Error("expected different names in fragment to fail", err)
	}

	_, err = Parse(`
{
	a @test
}`, map[string]interface{}{})
	if err == nil || err.Error() != "directives not supported" {
		t.Error("expected directives to fail", err)
	}

	_, err = Parse(`
{
	a(x: 1, x: 1)
}`, map[string]interface{}{})
	if err == nil || err.Error() != "duplicate arg" {
		t.Error("expected duplicate args to fail", err)
	}

	_, err = Parse(`
{
	... foo
}
fragment foo on Foo {
	... foo
}`, map[string]interface{}{})
	if err == nil || err.Error() != "fragment contains itself" {
		t.Error("expected fragment definition to fail", err)
	}

	_, err = Parse(`
{
	bar
}
fragment foo on Foo {
	x
}`, map[string]interface{}{})
	if err == nil || err.Error() != "unused fragment" {
		t.Error("expected unused fragment to fail", err)
	}
}
