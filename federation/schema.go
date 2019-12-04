package federation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
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

type introspectionType struct {
	Name          string                    `json:"name"`
	Kind          string                    `json:"kind"`
	Fields        []introspectionField      `json:"fields"`
	InputFields   []introspectionInputField `json:"inputFields"`
	PossibleTypes []*introspectionTypeRef   `json:"possibleTypes"`
}

type introspectionSchema struct {
	Types []introspectionType `json:"types"`
}

type introspectionQueryResult struct {
	Schema introspectionSchema `json:"__schema"`
}

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

		// Input types are non-nil if either type is non-nil, as one service
		// will always want an input. Output types are non-nil if both
		// types are non-nil, as we can only guarantee non-nil values if both
		// services play along.
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

// XXX: for types missing __federation, take intersection?

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

func mergeTypes(a, b introspectionType, mode MergeMode) (*introspectionType, error) {
	if a.Kind != b.Kind {
		return nil, fmt.Errorf("conflicting kinds %s and %s", a.Kind, b.Kind)
	}

	merged := introspectionType{
		Name: a.Name,
		Kind: a.Kind,
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

	case "SCALAR":

	case "ENUM":
		// XXX: merge values

	default:
		return nil, fmt.Errorf("unknown kind %s", a.Kind)
	}

	return &merged, nil
}

func mergeSchemas(a, b *introspectionQueryResult, mode MergeMode) (*introspectionQueryResult, error) {
	// XXX: should we surface orphaned types? complain about them?
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
			// For new objects, hide all fields. Otherwise we might end up
			// sending awkward queries to a service.
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

func lookupTypeRef(t *introspectionTypeRef, all map[string]graphql.Type) (graphql.Type, error) {
	if t == nil {
		return nil, errors.New("malformed typeref")
	}

	switch t.Kind {
	case "SCALAR", "OBJECT", "UNION", "INPUT_OBJECT":
		// XXX: enforce type?
		typ, ok := all[t.Name]
		if !ok {
			return nil, fmt.Errorf("type %s not found among top-level types", t.Name)
		}
		return typ, nil

	case "LIST":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &graphql.List{
			Type: inner,
		}, nil

	case "NON_NULL":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &graphql.NonNull{
			Type: inner,
		}, nil

	default:
		return nil, fmt.Errorf("unknown type kind %s", t.Kind)
	}
}

func parseInputFields(source []introspectionInputField, all map[string]graphql.Type) (map[string]graphql.Type, error) {
	fields := make(map[string]graphql.Type)

	for _, field := range source {
		here, err := lookupTypeRef(field.Type, all)
		if err != nil {
			return nil, fmt.Errorf("field %s has bad typ: %v",
				field.Name, err)
		}
		// XXX check this is an input type
		fields[field.Name] = here
	}

	return fields, nil
}

func parseSchema(schema *introspectionQueryResult) (map[string]graphql.Type, error) {
	all := make(map[string]graphql.Type)

	for _, typ := range schema.Schema.Types {
		if _, ok := all[typ.Name]; ok {
			return nil, fmt.Errorf("duplicate type %s", typ.Name)
		}

		switch typ.Kind {
		case "OBJECT":
			all[typ.Name] = &graphql.Object{
				Name: typ.Name,
			}

		case "INPUT_OBJECT":
			all[typ.Name] = &graphql.InputObject{
				Name: typ.Name,
			}

		case "SCALAR":
			all[typ.Name] = &graphql.Scalar{
				Type: typ.Name,
			}

		case "UNION":
			all[typ.Name] = &graphql.Union{
				Name: typ.Name,
			}

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	// XXX: should we surface orphaned types? complain about them?

	// Initialize barebone types
	for _, typ := range schema.Schema.Types {
		switch typ.Kind {
		case "OBJECT":
			fields := make(map[string]*graphql.Field)
			for _, field := range typ.Fields {
				fieldTyp, err := lookupTypeRef(field.Type, all)
				if err != nil {
					return nil, fmt.Errorf("typ %s field %s has bad typ: %v",
						typ.Name, field.Name, err)
				}

				parsed, err := parseInputFields(field.Args, all)
				if err != nil {
					return nil, fmt.Errorf("field %s input: %v", field.Name, err)
				}

				fields[field.Name] = &graphql.Field{
					Args: parsed,   // xxx
					Type: fieldTyp, // XXX
				}
			}

			all[typ.Name].(*graphql.Object).Fields = fields

		case "INPUT_OBJECT":
			parsed, err := parseInputFields(typ.InputFields, all)
			if err != nil {
				return nil, fmt.Errorf("typ %s: %v", typ.Name, err)
			}

			all[typ.Name].(*graphql.InputObject).InputFields = parsed

		case "UNION":
			types := make(map[string]*graphql.Object)
			for _, other := range typ.PossibleTypes {
				if other.Kind != "OBJECT" {
					return nil, fmt.Errorf("typ %s has possible typ not OBJECT: %v", typ.Name, other)
				}
				typ, ok := all[other.Name].(*graphql.Object)
				if !ok {
					return nil, fmt.Errorf("typ %s possible typ %s does not refer to obj", typ.Name, other.Name)
				}
				types[typ.Name] = typ
			}

			all[typ.Name].(*graphql.Union).Types = types

			// XXX: for (merged) unions, make sure we only send possible types
			// to each service

		case "SCALAR":
			// pass

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	return all, nil
}

/*
type ServiceSchemas map[string]introspectionQueryResult

type ServicesSchemas map[string]ServiceSchemas
*/

type FieldInfo struct {
	Service  string
	Services map[string]bool
}

type SchemaWithFederationInfo struct {
	Schema *graphql.Schema
	Fields map[*graphql.Field]*FieldInfo
}

func convertSchema(schemas map[string]introspectionQueryResult, mode MergeMode) (*SchemaWithFederationInfo, error) {
	schemaNames := make([]string, 0, len(schemas))
	for name := range schemas {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)

	var merged *introspectionQueryResult
	first := true

	// Initialize barebone types
	for _, service := range schemaNames {
		schema := schemas[service]
		if first {
			merged = &schema
			first = false
		} else {
			var err error
			merged, err = mergeSchemas(merged, &schema, mode)
			if err != nil {
				return nil, fmt.Errorf("merging %s: %v", service, err)
			}
		}
	}

	types, err := parseSchema(merged)
	if err != nil {
		return nil, err
	}

	fieldInfos := make(map[*graphql.Field]*FieldInfo)

	for _, service := range schemaNames {
		for _, typ := range schemas[service].Schema.Types {
			if typ.Kind == "OBJECT" {
				obj := types[typ.Name].(*graphql.Object)

				for _, field := range typ.Fields {
					f := obj.Fields[field.Name]

					info, ok := fieldInfos[f]
					if !ok {
						info = &FieldInfo{
							Service:  service,
							Services: map[string]bool{},
						}
						fieldInfos[f] = info
					}
					info.Services[service] = true
				}
			}
		}
	}

	return &SchemaWithFederationInfo{
		Schema: &graphql.Schema{
			Query:    types["Query"],    // XXX
			Mutation: types["Mutation"], // XXX
		},
		Fields: fieldInfos,
	}, nil
}
