package federation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

type FieldInfo struct {
	Service  string
	Services map[string]bool
}

type SchemaWithFederationInfo struct {
	Schema *graphql.Schema
	Fields map[*graphql.Field]*FieldInfo
}

type introspectionTypeRef struct {
	Kind   string                `json:"kind"`
	Name   string                `json:"name"`
	OfType *introspectionTypeRef `json:"ofType"`
}

type introspectionInputField struct {
	Name string                `json:"name"`
	Type *introspectionTypeRef `json:"type"`
}

type introspectionQueryResult struct {
	Schema struct {
		Types []struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Fields []struct {
				Name string                    `json:"name"`
				Type *introspectionTypeRef     `json:"type"`
				Args []introspectionInputField `json:"args"`
			} `json:"fields"`
			InputFields   []introspectionInputField `json:"inputFields"`
			PossibleTypes []*introspectionTypeRef   `json:"possibleTypes"`
		} `json:"types"`
	} `json:"__schema"`
}

// mergeInputFieldTypes takes the type of a field from two different schemas and
// computes a compatible type, if possible.
//
// Two types must be identical, besides non-nil flags, to be compatible. If one
// type is non-nil but the other is not, the combined typed will be non-nil.
func mergeInputFieldTypes(a, b graphql.Type) (graphql.Type, error) {
	// If either a or b is non-nil, unwrap it, recurse, and mark the resulting
	// type as non-nil.
	aNonNil := false
	if specific, ok := a.(*graphql.NonNull); ok {
		aNonNil = true
		a = specific.Type
	}
	bNonNil := false
	if specific, ok := b.(*graphql.NonNull); ok {
		bNonNil = true
		b = specific.Type
	}
	if aNonNil || bNonNil {
		merged, err := mergeInputFieldTypes(a, b)
		if err != nil {
			return nil, err
		}
		return &graphql.NonNull{Type: merged}, nil
	}

	// Otherwise, recursively assert that the input types are compatible.
	switch a := a.(type) {
	case *graphql.Scalar:
		// Scalars must be identical.
		b, ok := b.(*graphql.Scalar)
		if !ok {
			return nil, errors.New("both types must be scalar")
		}
		if a != b {
			return nil, errors.New("scalars must be identical")
		}
		return a, nil

	case *graphql.Enum:
		// Enums must be identical.
		b, ok := b.(*graphql.Enum)
		if !ok {
			return nil, errors.New("both types must be enum")
		}
		if a != b {
			return nil, errors.New("enums must be identical")
		}
		return a, nil

	case *graphql.List:
		// Lists must be compatible but don't have to be identical.
		b, ok := b.(*graphql.List)
		if !ok {
			return nil, errors.New("both types must be list")
		}
		inner, err := mergeInputFieldTypes(a.Type, b.Type)
		if err != nil {
			return nil, err
		}
		return &graphql.List{Type: inner}, nil

	case *graphql.InputObject:
		// InputObjects must be identical. The types might be different on the
		// servers and will be merged when their fields are merged, but the type
		// names of the fields must be equal.
		b, ok := b.(*graphql.InputObject)
		if !ok {
			return nil, errors.New("both types must be input object")
		}
		if a != b {
			return nil, errors.New("input objects must be identical")
		}
		return a, nil

	default:
		return nil, errors.New("unknown type kind")
	}
}

// XXX: for types missing __federation, take intersection?

func mergeObjectFieldTypes(a, b graphql.Type) (graphql.Type, error) {
	// If either a or b is non-nil, unwrap it, recurse, and mark the resulting
	// type as nilable.
	aNonNil := false
	if specific, ok := a.(*graphql.NonNull); ok {
		aNonNil = true
		a = specific.Type
	}
	bNonNil := false
	if specific, ok := b.(*graphql.NonNull); ok {
		bNonNil = true
		b = specific.Type
	}
	if aNonNil || bNonNil {
		merged, err := mergeObjectFieldTypes(a, b)
		if err != nil {
			return nil, err
		}
		return merged, nil
	}

	// Otherwise, recursively assert that the types are compatible.
	switch a := a.(type) {
	case *graphql.Scalar:
		// Scalars must be identical.
		b, ok := b.(*graphql.Scalar)
		if !ok {
			return nil, errors.New("both types must be scalar")
		}
		if a != b {
			return nil, errors.New("scalars must be identical")
		}
		return a, nil

	case *graphql.Enum:
		// Enums must be identical.
		b, ok := b.(*graphql.Enum)
		if !ok {
			return nil, errors.New("both types must be enum")
		}
		if a != b {
			return nil, errors.New("enums must be identical")
		}
		return a, nil

	case *graphql.List:
		// Lists must be compatible but don't have to be identical.
		b, ok := b.(*graphql.List)
		if !ok {
			return nil, errors.New("both types must be list")
		}
		inner, err := mergeObjectFieldTypes(a.Type, b.Type)
		if err != nil {
			return nil, err
		}
		return &graphql.List{Type: inner}, nil

	case *graphql.Object:
		// Objects must be identical. The types might be different on the
		// servers and will be merged when their fields are merged, but the type
		// names of the fields must be equal.
		b, ok := b.(*graphql.Object)
		if !ok {
			return nil, errors.New("both types must be object")
		}
		if a != b {
			return nil, errors.New("objects must be identical")
		}
		return a, nil

	case *graphql.Union:
		// Unions must be identical. The types might be different on the
		// servers and will be merged when their fields are merged, but the type
		// names of the fields must be equal.
		b, ok := b.(*graphql.Union)
		if !ok {
			return nil, errors.New("both types must be union")
		}
		if a != b {
			return nil, errors.New("unions must be identical")
		}
		return a, nil

	default:
		return nil, errors.New("unknown type kind")
	}

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

func mergeInputFields(a, b map[string]graphql.Type) (map[string]graphql.Type, error) {
	merged := make(map[string]graphql.Type)
	for name, here := range b {
		current, ok := a[name]
		if !ok {
			if _, ok := here.(*graphql.NonNull); ok {
				return nil, fmt.Errorf("new field %s is non-null: %s", name, here)
			}
			merged[name] = here
		} else {
			m, err := mergeInputFieldTypes(current, here)
			if err != nil {
				return nil, fmt.Errorf("field %s has incompatible types %s and %s: %s",
					a, current, here, err)
			}
			merged[name] = m
		}
	}
	for name, here := range a {
		_, ok := b[name]
		if ok {
			// already done above
		} else {
			if _, ok := here.(*graphql.NonNull); ok {
				return nil, fmt.Errorf("new field %s is non-null: %s", name, here)
			}
			merged[name] = here
		}
	}
	return merged, nil
}

func convertSchema(schemas map[string]introspectionQueryResult) (*SchemaWithFederationInfo, error) {
	all := make(map[string]graphql.Type)
	typeKinds := make(map[string]string)

	// XXX: should we surface orphaned types? complain about them?

	schemaNames := make([]string, 0, len(schemas))
	for name := range schemas {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)

	// Initialize barebone types
	for _, service := range schemaNames {
		schema := schemas[service]
		for _, typ := range schema.Schema.Types {
			if kind, ok := typeKinds[typ.Name]; ok {
				if kind != typ.Kind {
					return nil, fmt.Errorf("conflicting kinds for typ %s", typ.Name)
				}
				continue
			}
			typeKinds[typ.Name] = typ.Kind

			switch typ.Kind {
			case "OBJECT":
				all[typ.Name] = &graphql.Object{
					Name:   typ.Name,
					Fields: make(map[string]*graphql.Field),
				}

			case "INPUT_OBJECT":
				all[typ.Name] = &graphql.InputObject{
					Name:        typ.Name,
					InputFields: make(map[string]graphql.Type),
				}

			case "SCALAR":
				all[typ.Name] = &graphql.Scalar{
					Type: typ.Name,
				}

			case "UNION":
				all[typ.Name] = &graphql.Union{
					Name:  typ.Name,
					Types: make(map[string]*graphql.Object),
				}

			default:
				return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
			}
		}
	}

	fieldInfos := make(map[*graphql.Field]*FieldInfo)

	seenInput := make(map[string]bool)

	for _, service := range schemaNames {
		schema := schemas[service]
		for _, typ := range schema.Schema.Types {
			switch typ.Kind {
			case "INPUT_OBJECT":
				obj := all[typ.Name].(*graphql.InputObject)
				parsed, err := parseInputFields(typ.InputFields, all)
				if err != nil {
					return nil, fmt.Errorf("service %s typ %s: %v", service, typ.Name, err)
				}

				if !seenInput[typ.Name] {
					obj.InputFields = parsed
					seenInput[typ.Name] = true
				} else {
					merged, err := mergeInputFields(obj.InputFields, parsed)
					if err != nil {
						return nil, fmt.Errorf("service %s typ %s: %v", service, typ.Name, err)
					}
					obj.InputFields = merged
				}

			case "OBJECT":
				obj := all[typ.Name].(*graphql.Object)

				for _, field := range typ.Fields {
					typ, err := lookupTypeRef(field.Type, all)
					if err != nil {
						return nil, fmt.Errorf("service %s typ %s field %s has bad typ: %v",
							service, typ, field.Name, err)
					}

					parsed, err := parseInputFields(field.Args, all)
					if err != nil {
						return nil, fmt.Errorf("service %s field %s input: %v", service, field.Name, err)
					}

					f, ok := obj.Fields[field.Name]
					if !ok {
						f = &graphql.Field{
							Args: parsed, // xxx
							Type: typ,    // XXX
						}
						obj.Fields[field.Name] = f
						fieldInfos[f] = &FieldInfo{
							Service:  service,
							Services: map[string]bool{},
						}
					} else {
						merged, err := mergeInputFields(f.Args, parsed)
						if err != nil {
							return nil, fmt.Errorf("service %s field %s input: %v", service, field.Name, err)
						}
						f.Args = merged

						respMerged, err := mergeObjectFieldTypes(f.Type, typ)
						if err != nil {
							return nil, fmt.Errorf("service %s typ %s field %s has bad typ: %v",
								service, typ, field.Name, err)
						}
						f.Type = respMerged
					}

					// XXX check consistent types

					fieldInfos[f].Services[service] = true
				}

			case "UNION":
				union := all[typ.Name].(*graphql.Union)

				for _, other := range typ.PossibleTypes {
					if other.Kind != "OBJECT" {
						return nil, fmt.Errorf("service %s typ %s has possible typ not OBJECT: %v", service, typ.Name, other)
					}
					typ, ok := all[other.Name].(*graphql.Object)
					if !ok {
						return nil, fmt.Errorf("service %s typ %s possible typ %s does not refer to obj", service, typ.Name, other.Name)
					}
					union.Types[typ.Name] = typ
				}
			}
		}
	}

	return &SchemaWithFederationInfo{
		Schema: &graphql.Schema{
			Query:    all["Query"],    // XXX
			Mutation: all["Mutation"], // XXX
		},
		Fields: fieldInfos,
	}, nil
}

// schema.Extend()

// XXX: any types you return you must have the definition for...
//
//   how do we enforce that?? some compile time check that crosses package
//   boundaries and spots Object() (or whatever) calls that are automatic in some
//   package and not in another?
//
//   could not do magic anymore and require an explicit "schema.Object" call for
//   any types returned... maybe with schema.AutoObject("") to handle automatic
//   cases?
//
// XXX: could not allow schemabuilder auto objects outside of packages? seems nice.
// }
