package graphql_test

// import (
// 	. "github.com/samsarahq/thunder/graphql"
// )

// func TestParseSupported(t *testing.T) {
// 	query, err := Parse(`
// {
// 	foo {
// 		alias: bar
// 		alias: bar
// 		baz(arg: 3) {
// 			bah(x: 1, y: "123", z: true)
// 			hum(foo: {x: $var}, bug: [1, 2, [4, 5]])
// 		}
// 		... on Foo {
// 			asd
// 			... Bar
// 		}
// 	}
// 	xyz
// }

// fragment Bar on Foo {
// 	zxc
// }`, map[string]interface{}{
// 		"var": "var value!!",
// 	})
// 	if err != nil {
// 		t.Error("unexpected error", err)
// 	}

// 	expected := &Query{
// 		Name: "",
// 		Kind: "query",
// 		SelectionSet: &SelectionSet{
// 			Selections: []*Selection{
// 				{
// 					Name:  "foo",
// 					Alias: "foo",
// 					Args:  map[string]interface{}{},
// 					SelectionSet: &SelectionSet{
// 						Selections: []*Selection{
// 							{
// 								Name:  "bar",
// 								Alias: "alias",
// 								Args:  map[string]interface{}{},
// 							},
// 							{
// 								Name:  "bar",
// 								Alias: "alias",
// 								Args:  map[string]interface{}{},
// 							},
// 							{
// 								Name:  "baz",
// 								Alias: "baz",
// 								Args: map[string]interface{}{
// 									"arg": float64(3),
// 								},
// 								SelectionSet: &SelectionSet{
// 									Selections: []*Selection{
// 										{
// 											Name:  "bah",
// 											Alias: "bah",
// 											Args: map[string]interface{}{
// 												"x": float64(1),
// 												"y": "123",
// 												"z": true,
// 											},
// 										},
// 										{
// 											Name:  "hum",
// 											Alias: "hum",
// 											Args: map[string]interface{}{
// 												"foo": map[string]interface{}{
// 													"x": "var value!!",
// 												},
// 												"bug": []interface{}{
// 													float64(1), float64(2),
// 													[]interface{}{float64(4), float64(5)},
// 												},
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 						Fragments: []*Fragment{
// 							{
// 								On: "Foo",
// 								SelectionSet: &SelectionSet{
// 									Selections: []*Selection{
// 										{
// 											Name:  "asd",
// 											Alias: "asd",
// 											Args:  map[string]interface{}{},
// 										},
// 									},
// 									Fragments: []*Fragment{
// 										{
// 											On: "Foo",
// 											SelectionSet: &SelectionSet{
// 												Selections: []*Selection{
// 													{
// 														Name:  "zxc",
// 														Alias: "zxc",
// 														Args:  map[string]interface{}{},
// 													},
// 												},
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 				{
// 					Name:  "xyz",
// 					Alias: "xyz",
// 					Args:  map[string]interface{}{},
// 				},
// 			},
// 		},
// 	}

// 	if !reflect.DeepEqual(query, expected) {
// 		t.Error("unexpected parse")
// 	}

// 	query, err = Parse(`
// mutation foo($var: bar) {
// 	baz
// }
// `, map[string]interface{}{
// 		"var": "var value!!",
// 	})
// 	if err != nil {
// 		t.Error("unexpected error", err)
// 	}

// 	expected = &Query{
// 		Name: "foo",
// 		Kind: "mutation",
// 		SelectionSet: &SelectionSet{
// 			Selections: []*Selection{
// 				{
// 					Name:  "baz",
// 					Alias: "baz",
// 					Args:  map[string]interface{}{},
// 				},
// 			},
// 		},
// 	}
// 	if !reflect.DeepEqual(query, expected) {
// 		t.Error("unexpected parse")
// 	}
// }

// func TestParseUnsupported(t *testing.T) {
// 	_, err := Parse(``, map[string]interface{}{})
// 	if err == nil || err.Error() != "must have a single query" {
// 		t.Error("expected missing query to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	bar
// }

// {
// 	baz
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "only support a single query" {
// 		t.Error("expected multiple queries to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	b(a: 1)
// 	b(a: 2)
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "same alias with different args" {
// 		t.Error("expected different args to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	a: a
// 	a: b
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "same alias with different name" {
// 		t.Error("expected different names to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	a: a
// 	... on Foo {
// 		a: b
// 	}
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "same alias with different name" {
// 		t.Error("expected different names in fragment to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	a @test
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "directives not supported" {
// 		t.Error("expected directives to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	a(x: 1, x: 1)
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "duplicate arg" {
// 		t.Error("expected duplicate args to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	... foo
// }
// fragment foo on Foo {
// 	... foo
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "fragment contains itself" {
// 		t.Error("expected fragment definition to fail", err)
// 	}

// 	_, err = Parse(`
// {
// 	bar
// }
// fragment foo on Foo {
// 	x
// }`, map[string]interface{}{})
// 	if err == nil || err.Error() != "unused fragment" {
// 		t.Error("expected unused fragment to fail", err)
// 	}
// }

// func TestParseRequiredVariableDefinitionWithDefaultValue(t *testing.T) {
// 	// Expect required variables to be provided.
// 	_, err := Parse(`
// query Operation($x: int64! = 2) {
// 	field(x: $x)
// }	`, map[string]interface{}{})

// 	if err == nil || err.Error() != "required variable cannot provide a default value: $x" {
// 		t.Error("expected required argument with default value to fail, but got", err)
// 	}
// }

// func TestParseFillInDefaultValues(t *testing.T) {
// 	// Fill in default values when provided.
// 	query, err := Parse(`
// query Operation($x: int64 = 2) {
// 	field(x: $x)
// }	`, map[string]interface{}{})

// 	if err != nil {
// 		t.Error("expected default value to be used, but received", err)
// 	}

// 	args := query.SelectionSet.Selections[0].Args.(map[string]interface{})

// 	if len := len(args); len != 1 {
// 		t.Errorf("expected 1 argument, received %d", len)
// 	}

// 	if val := args["x"]; val != float64(2) {
// 		t.Errorf("expected 2, received %v", val)
// 	}
// }
