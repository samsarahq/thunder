package federation

import (
	"errors"
	"fmt"
	"sort"

)

// MergeMode controls how to combine two different schemas. Union is used for
// two independent services, Intersection for two different versions of the same
// service.
type MergeMode string

const (
	// Union computes a schema that is supported by the two services combined.
	//
	// A Union is used to to combine the schema of two independent services.
	// The proxy will split a GraphQL query to ask each service the fields
	// it knows about.
	//
	// Two schemas must be compatible: Any overlapping types (eg. a field that
	// is implemented by both services, or two input types) must be compatible.
	// In practice, this means types must be identical except for non-nil
	// modifiers.
	//
	// XXX: take intersection on ENUM values to not confuse a service with a
	// type it doesn't support?
	Union MergeMode = "union"

	// Intersection computes a schema that is supported by both services.
	//
	// An Intersection is used to combine two schemas of different versions
	// of the same service. During a deploy, only of two versions might be
	// available, and so queries must be compatible with both schemas.
	//
	// Intersection computes a schema that can be executed by both services.
	// It only includes types and fields (etc.) exported by both services.
	// Overlapping types must be compatible as in a Union merge.
	//
	// One surprise might be that newly added ENUM values or UNION types might
	// be returned by the merged schema.
	Intersection MergeMode = "intersection"
)

// introspectionTypeRef is a type reference from the GraphQL introspection
// query.
type introspectionTypeRef struct {
	Kind   string                `json:"kind"`
	Name   string                `json:"name"`
	OfType *introspectionTypeRef `json:"ofType"`
}

func (t *introspectionTypeRef) String() string {
	if t == nil {
		return "<nil>"
	}
	switch t.Kind {
	case "SCALAR", "ENUM", "UNION", "OBJECT", "INPUT_OBJECT":
		return t.Name
	case "NON_NULL":
		return t.OfType.String() + "!"
	case "LIST":
		return "[" + t.OfType.String() + "]"
	default:
		return fmt.Sprintf("<kind=%s name=%s ofType%s>", t.Kind, t.Name, t.OfType)
	}
}

type introspectionInputField struct {
	Name string                `json:"name"`
	Type *introspectionTypeRef `json:"type"`
}

type introspectionField struct {
	Name string                    `json:"name"`
	Type *introspectionTypeRef     `json:"type"`
	Args []introspectionInputField `json:"args"`
}

type introspectionEnumValue struct {
	Name string `json:"name"`
}

type introspectionType struct {
	Name          string                    `json:"name"`
	Kind          string                    `json:"kind"`
	Fields        []introspectionField      `json:"fields"`
	InputFields   []introspectionInputField `json:"inputFields"`
	PossibleTypes []*introspectionTypeRef   `json:"possibleTypes"`
	EnumValues    []introspectionEnumValue  `json:"enumValues"`
}

type introspectionSchema struct {
	Types []introspectionType `json:"types"`
}

type introspectionQueryResult struct {
	Schema introspectionSchema `json:"__schema"`
}

// mergeTypeRefs takes two types from two different services, makes sure
// they are compatible, and computes a merged type.
//
// Two types are compatible if they are the same besides non-nullable modifiers.
//
// The merged type gets non-nullable modifiers depending on how the type is used.
// For input types, the merged type should be accepted by both services, so it's
// nullable only if both services accept a nullable type.
// For output types, the merged type should work for either service, so it's
// nullable if either service might return null.
func mergeTypeRefs(a, b *introspectionTypeRef, isInput bool) (*introspectionTypeRef, error) {
	// If either a or b is non-nil, unwrap it, recurse, and maybe mark the
	// resulting type as non-nil.
	aNonNil := false
	if a.Kind == "NON_NULL" {
		aNonNil = true
		a = a.OfType
	}
	bNonNil := false
	if b.Kind == "NON_NULL" {
		bNonNil = true
		b = b.OfType
	}
	if aNonNil || bNonNil {
		merged, err := mergeTypeRefs(a, b, isInput)
		if err != nil {
			return nil, err
		}

		// Input types are non-nil if either type is non-nil, as one service will always
		// want an input. Output types are non-nil if both types are non-nil, as we can
		// only guarantee non-nil values if both services play along.
		resultNonNil := isInput || (aNonNil && bNonNil)

		if resultNonNil {
			return &introspectionTypeRef{Kind: "NON_NULL", OfType: merged}, nil
		}
		return merged, nil
	}

	// Otherwise, recursively assert that the input types are compatible.
	if a.Kind != b.Kind {
		return nil, fmt.Errorf("kinds %s and %s differ", a.Name, b.Kind)
	}
	switch a.Kind {
	// Basic types must be identical.
	case "SCALAR", "ENUM", "INPUT_OBJECT", "UNION", "OBJECT":
		if a.Name != b.Name {
			return nil, errors.New("types must be identical")
		}
		return &introspectionTypeRef{
			Kind: a.Kind,
			Name: a.Name,
		}, nil

	// Recursive must be compatible but don't have to be identical.
	case "LIST":
		inner, err := mergeTypeRefs(a.OfType, b.OfType, isInput)
		if err != nil {
			return nil, err
		}
		return &introspectionTypeRef{Kind: "LIST", OfType: inner}, nil

	default:
		return nil, errors.New("unknown type kind")
	}
}

// mergeInputFields combines two sets of input fields from two schemas.
//
// It checks the types are compabible and takes the union or intersection of the
// fields depending on the Mergemode
func mergeInputFields(a, b []introspectionInputField, mode MergeMode) ([]introspectionInputField, error) {
	types := make(map[string][]introspectionInputField)
	for _, a := range a {
		types[a.Name] = append(types[a.Name], a)
	}
	for _, b := range b {
		types[b.Name] = append(types[b.Name], b)
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]introspectionInputField, 0, len(names))
	for _, name := range names {
		p := types[name]
		if len(p) == 1 {
			if p[0].Type.Kind == "NON_NULL" {
				return nil, fmt.Errorf("new field %s is non-null: %v", name, p[0].Type)
			}
			if mode == Union {
				merged = append(merged, p[0])
			}
			continue
		}
		m, err := mergeTypeRefs(p[0].Type, p[1].Type, true)
		if err != nil {
			return nil, fmt.Errorf("field %s has incompatible types %s and %s: %v", name, p[0].Type, p[1].Type, err)
		}
		merged = append(merged, introspectionInputField{
			Name: name,
			Type: m,
		})
	}

	return merged, nil
}

func mergeFields(a, b []introspectionField, mode MergeMode) ([]introspectionField, error) {
	types := make(map[string][]introspectionField)
	for _, a := range a {
		types[a.Name] = append(types[a.Name], a)
	}
	for _, b := range b {
		types[b.Name] = append(types[b.Name], b)
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]introspectionField, 0, len(names))
	for _, name := range names {
		p := types[name]
		if len(p) == 1 {
			if mode == Union {
				merged = append(merged, p[0])
			}
			continue
		}

		typ, err := mergeTypeRefs(p[0].Type, p[1].Type, false)
		if err != nil {
			return nil, fmt.Errorf("field %s has incompatible types %v and %v: %v", name, p[0], p[1], err)
		}
		args, err := mergeInputFields(p[0].Args, p[1].Args, mode)
		if err != nil {
			return nil, fmt.Errorf("field %s has incompatible arguments: %v", name, err)
		}

		merged = append(merged, introspectionField{
			Name: name,
			Type: typ,
			Args: args,
		})
	}

	return merged, nil
}

func mergePossibleTypes(a, b []*introspectionTypeRef, mode MergeMode) ([]*introspectionTypeRef, error) {
	types := make(map[string][]*introspectionTypeRef)
	for _, a := range a {
		types[a.Name] = append(types[a.Name], a)
	}
	for _, b := range b {
		types[b.Name] = append(types[b.Name], b)
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]*introspectionTypeRef, 0, len(names))
	for _, name := range names {
		p := types[name]
		if len(p) == 1 {
			if mode == Union {
				merged = append(merged, p[0])
			}
			continue
		}

		merged = append(merged, p[0])
	}

	return merged, nil
}

func mergeEnumValues(a, b []introspectionEnumValue, mode MergeMode) ([]introspectionEnumValue, error) {
	types := make(map[string][]introspectionEnumValue)
	for _, a := range a {
		types[a.Name] = append(types[a.Name], a)
	}
	for _, b := range b {
		types[b.Name] = append(types[b.Name], b)
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]introspectionEnumValue, 0, len(names))
	for _, name := range names {
		p := types[name]
		if len(p) == 1 {
			if mode == Union {
				merged = append(merged, p[0])
			}
			continue
		}

		merged = append(merged, p[0])
	}

	return merged, nil
}

func mergeTypes(a, b introspectionType, mode MergeMode) (*introspectionType, error) {
	if a.Kind != b.Kind {
		return nil, fmt.Errorf("conflicting kinds %s and %s", a.Kind, b.Kind)
	}

	merged := introspectionType{
		Name:          a.Name,
		Kind:          a.Kind,
		Fields:        []introspectionField{},
		InputFields:   []introspectionInputField{},
		PossibleTypes: []*introspectionTypeRef{},
		EnumValues:    []introspectionEnumValue{},
	}

	switch a.Kind {
	case "INPUT_OBJECT":
		inputFields, err := mergeInputFields(a.InputFields, b.InputFields, mode)
		if err != nil {
			return nil, fmt.Errorf("merging input fields: %v", err)
		}
		merged.InputFields = inputFields

	case "OBJECT":
		fields, err := mergeFields(a.Fields, b.Fields, mode)
		if err != nil {
			return nil, fmt.Errorf("merging fields: %v", err)
		}
		merged.Fields = fields

	case "UNION":
		possibleTypes, err := mergePossibleTypes(a.PossibleTypes, b.PossibleTypes, mode)
		if err != nil {
			return nil, fmt.Errorf("merging possible types: %v", err)
		}
		merged.PossibleTypes = possibleTypes

	case "ENUM":
		enumValues, err := mergeEnumValues(a.EnumValues, b.EnumValues, mode)
		if err != nil {
			return nil, fmt.Errorf("merging enum values: %v", err)
		}
		merged.EnumValues = enumValues

	case "SCALAR":

	default:
		return nil, fmt.Errorf("unknown kind %s", a.Kind)
	}

	return &merged, nil
}

func mergeSchemas(a, b *introspectionQueryResult, mode MergeMode) (*introspectionQueryResult, error) {
	types := make(map[string][]introspectionType)
	for _, a := range a.Schema.Types {
		types[a.Name] = append(types[a.Name], a)
	}
	for _, b := range b.Schema.Types {
		types[b.Name] = append(types[b.Name], b)
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]introspectionType, 0, len(names))
	for _, name := range names {
		p := types[name]
		if len(p) == 1 {
			// When intersection only include types that appear in both schemas
			if mode == Union {
				merged = append(merged, p[0])
			}
			continue
		}
		m, err := mergeTypes(p[0], p[1], mode)
		if err != nil {
			return nil, fmt.Errorf("can't merge type %s: %v", name, err)
		}
		merged = append(merged, *m)
	}

	return &introspectionQueryResult{
		Schema: introspectionSchema{
			Types: merged,
		},
	}, nil
}

func mergeSchemaSlice(schemas []*introspectionQueryResult, mode MergeMode) (*introspectionQueryResult, error) {
	if len(schemas) == 0 {
		return nil, errors.New("no schemas")
	}
	merged := schemas[0]
	for _, schema := range schemas[1:] {
		var err error
		merged, err = mergeSchemas(merged, schema, mode)
		if err != nil {
			return nil, err
		}
	}
	return merged, nil
}
