package schemabuilder

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/northvolt/thunder/graphql"
)

// IfaceStrategy determines the strategy that should be used for what fields
// to expose in GraphQL for a specific type.
type IfaceStrategy func(*schemaBuilder, reflect.Method) (string, *graphql.Field, error)

// IfaceGetterStrategy only exposes fields for methods that starts with 'Get'.
// Example:
// func GetID() string => id: String!
// func getID() string => nothing
// func ID() string => nothing
func IfaceGetterStrategy(sb *schemaBuilder, method reflect.Method) (string, *graphql.Field, error) {
	// Ignore methods without output
	if method.Type.NumOut() == 0 {
		return "", nil, nil
	}

	fieldName := strings.Replace(method.Name, "Get", "", 1)
	// Ignore methods that don't start with 'Get'
	if fieldName == method.Name {
		return "", nil, nil
	}

	retType, err := sb.getType(method.Type.Out(0))
	if err != nil {
		return "", nil, err
	}

	return makeGraphql(fieldName), &graphql.Field{
		Resolve:        createIfaceResolver(method.Name),
		Type:           retType,
		ParseArguments: nilParseArguments,
	}, nil
}

func (sb *schemaBuilder) buildIface(typ reflect.Type) error {
	if sb.types[typ] != nil {
		return nil
	}

	var name string
	var description string
	methods := Methods{}
	// var objectKey string
	possibleTypes := make(map[string]*graphql.Object)
	if object, ok := sb.ifaces[typ]; ok {
		name = object.Name
		description = object.Description
		methods = object.Methods
		// objectKey = object.key
		for _, obj := range object.PossibleTypes {
			sb.buildStruct(obj)
			obj := sb.types[obj].(*graphql.Object)
			possibleTypes[obj.Name] = obj
		}
	}

	if name == "" {
		log.Printf("%#v", typ)
		name = typ.Name()
		if name == "" {
			return fmt.Errorf("bad type %s: should have a name", typ)
		}
		if originalType, ok := sb.typeNames[name]; ok {
			return fmt.Errorf("duplicate name %s: seen both %v and %v", name, originalType, typ)
		}
	}

	object := &graphql.Object{
		Name:          name,
		Description:   description,
		Fields:        make(map[string]*graphql.Field),
		IsInterface:   true,
		PossibleTypes: possibleTypes,
		Type:          typ,
	}
	sb.types[typ] = object
	sb.typeNames[name] = typ

	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fieldName, field, err := sb.ifaceStrategy(sb, m)
		if err != nil {
			return fmt.Errorf("method: %s.%s: %w", typ.Name(), m.Name, err)
		}
		if field == nil || fieldName == "" {
			continue
		}
		object.Fields[fieldName] = field
	}

	for name, method := range methods {
		fn, err := sb.buildFunction(typ, method)
		if err != nil {
			return fmt.Errorf("build method %s on %s: %w", name, object.Name, err)
		}
		object.Fields[name] = fn
	}

	return nil
}

// buildField generates a graphQL field for a struct's field.  This field can be
// used to "resolve" a response for a graphql request.
func (sb *schemaBuilder) buildIfaceField(method reflect.Method) (*graphql.Field, error) {
	if method.Type.NumOut() == 0 {
		return nil, nil
	}
	retType, err := sb.getType(method.Type.Out(0))
	if err != nil {
		return nil, err
	}

	return &graphql.Field{
		Resolve:        createIfaceResolver(method.Name),
		Type:           retType,
		ParseArguments: nilParseArguments,
	}, nil
}

func createIfaceResolver(methodName string) graphql.Resolver {
	return func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
		// Find the method on the specific type we received.
		m, ok := findMethodOnType(source, methodName)
		if !ok {
			return nil, fmt.Errorf("method %s not found on %T", methodName, source)
		}
		// Call it without any input arguments.
		for _, v := range m.Call(nil) {
			// We always return the first result.
			return v.Interface(), nil
		}
		return nil, fmt.Errorf("no result")
	}
}

func findMethodOnType(source interface{}, name string) (reflect.Value, bool) {
	t := reflect.TypeOf(source)
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.Name == name {
			return reflect.ValueOf(source).Method(i), true
		}
	}
	return reflect.Value{}, false
}

func (sb *schemaBuilder) validateInterfaces() error {
	for t, iface := range sb.ifaces {
		for _, pt := range iface.PossibleTypes {
			impl, ok := sb.types[pt]
			if !ok {
				return fmt.Errorf("found missing possible type: %s for %s", pt, iface.Name)
			}
			obj, ok := impl.(*graphql.Object)
			if !ok {
				return fmt.Errorf("not object %s for %s", impl.String(), iface.Name)
			}
			for name, f := range sb.types[t].(*graphql.Object).Fields {
				ff, ok := obj.Fields[name]
				if !ok {
					return fmt.Errorf("missing field %s on %s for %s", name, obj.Name, iface.Name)
				}
				if f.Type.String() != ff.Type.String() {
					return fmt.Errorf("non-matching types %s and %s on field %s for %s and %s", f.Type.String(), ff.Type.String(), name, iface.Name, obj.Name)
				}
			}
		}
	}
	return nil
}
