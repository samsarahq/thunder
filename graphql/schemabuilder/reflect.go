package schemabuilder

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/samsarahq/thunder/graphql"
)

func makeGraphql(s string) string {
	var b bytes.Buffer
	for i, c := range s {
		if i == 0 {
			b.WriteRune(unicode.ToLower(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func reverseGraphqlFieldName(s string) string {
	var b bytes.Buffer
	for i, c := range s {
		if i == 0 {
			b.WriteRune(unicode.ToUpper(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// graphQLFieldInfo contains basic struct field information related to GraphQL.
type graphQLFieldInfo struct {
	// Skipped indicates that this field should not be included in GraphQL.
	Skipped bool

	// Name is the GraphQL field name that should be exposed for this field.
	Name string

	// KeyField indicates that this field should be treated as a Object Key field.
	KeyField bool
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

	if len(tags) > 1 {
		for _, tag := range tags[1:] {
			if tag != "key" || key {
				return nil, fmt.Errorf("field %s has unexpected tag %s", name, tag)
			}
			key = true
		}
	}
	return &graphQLFieldInfo{Name: name, KeyField: key}, nil
}

var errType reflect.Type
var contextType reflect.Type
var selectionSetType reflect.Type

func init() {
	var err error
	errType = reflect.TypeOf(&err).Elem()
	var context context.Context
	contextType = reflect.TypeOf(&context).Elem()
	var selectionSet *graphql.SelectionSet
	selectionSetType = reflect.TypeOf(selectionSet)
}
