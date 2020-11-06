package schemabuilder

import (
	"context"
	"encoding"
	"fmt"
	"reflect"
	"strings"

	"github.com/northvolt/thunder/graphql"
	"github.com/pascaldekloe/name"
)

var rewrites = []func(string) string{
	rewrite("iD", "id"),
}

var reverseRewrites = []func(string) string{
	rewrite("id", "iD"),
}

func rewrite(from, to string) func(string) string {
	return func(s string) string {
		if strings.HasPrefix(s, from) {
			return strings.Replace(s, from, to, 1)
		}
		return s
	}
}

// makeGraphql converts a field name "MyField" into a graphQL field name "myField".
func makeGraphql(s string) string {
	res := name.CamelCase(s, false)
	for _, fn := range rewrites {
		res = fn(res)
	}
	return res
}

// reverseGraphqlFieldName converts a graphql field name "myField" into a
// non-graphQL field name "MyField".
func reverseGraphqlFieldName(s string) string {
	res := name.CamelCase(s, true)
	for _, fn := range reverseRewrites {
		res = fn(res)
	}
	return res
}

// graphQLFieldInfo contains basic struct field information related to GraphQL.
type graphQLFieldInfo struct {
	// Skipped indicates that this field should not be included in GraphQL.
	Skipped bool

	// Name is the GraphQL field name that should be exposed for this field.
	Name string

	// KeyField indicates that this field should be treated as a Object Key field.
	KeyField bool

	// OptionalInputField indicates that this field should be treated as an optional
	// field on graphQL input args.
	OptionalInputField bool
}

// parseGraphQLFieldInfo parses a struct field and returns a struct with the
// parsed information about the field (tag info, name, etc).
func parseGraphQLFieldInfo(field reflect.StructField) (*graphQLFieldInfo, error) {
	if field.PkgPath != "" {
		return &graphQLFieldInfo{Skipped: true}, nil
	}
	tags := strings.Split(field.Tag.Get("graphql"), ",")
	var name string
	if len(tags) > 0 {
		name = tags[0]
	}
	if name == "" {
		name = makeGraphql(field.Name)
	}
	if name == "-" {
		return &graphQLFieldInfo{Skipped: true}, nil
	}

	var key bool
	var optional bool

	if len(tags) > 1 {
		for _, tag := range tags[1:] {
			if tag == "key" && !key {
				key = true
			} else if tag == "optional" && !optional {
				optional = true
			} else {
				return nil, fmt.Errorf("field %s has unexpected tag %s", name, tag)
			}
		}
	}
	return &graphQLFieldInfo{Name: name, KeyField: key, OptionalInputField: optional}, nil
}

// Common Types that we will need to perform type assertions against.
var errType = reflect.TypeOf((*error)(nil)).Elem()
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var selectionSetType = reflect.TypeOf(&graphql.SelectionSet{})
var textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
