package schemabuilder

import (
	"context"
	"fmt"
	"log"
	"reflect"

	"github.com/northvolt/thunder/graphql"
)

func (sb *schemaBuilder) buildIface(typ reflect.Type) error {
	if sb.types[typ] != nil {
		return nil
	}

	var name string
	var description string
	// var methods Methods
	// var objectKey string
	if object, ok := sb.objects[typ]; ok {
		name = object.Name
		description = object.Description
		// methods = object.Methods
		// objectKey = object.key
	}

	if name == "" {
		name = typ.Name()
		if name == "" {
			return fmt.Errorf("bad type %s: should have a name", typ)
		}
		if originalType, ok := sb.typeNames[name]; ok {
			return fmt.Errorf("duplicate name %s: seen both %v and %v", name, originalType, typ)
		}
	}

	object := &graphql.Object{
		Name:        name,
		Description: description,
		Fields:      make(map[string]*graphql.Field),
		IsInterface: true,
	}
	sb.types[typ] = object
	sb.typeNames[name] = typ

	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		field, err := sb.buildIfaceField(m)
		if err != nil {
			return fmt.Errorf("method: %s.%s: %w", typ.Name(), m.Name, err)
		}
		if field == nil {
			continue
		}
		object.Fields[makeGraphql(m.Name)] = field
	}

	return nil
}

// buildField generates a graphQL field for a struct's field.  This field can be
// used to "resolve" a response for a graphql request.
func (sb *schemaBuilder) buildIfaceField(method reflect.Method) (*graphql.Field, error) {
	log.Printf("BUILD IFACE FIELD: %s", method.Name)
	if method.Type.NumOut() == 0 {
		return nil, nil
	}
	retType, err := sb.getType(method.Type.Out(0))
	if err != nil {
		return nil, err
	}

	return &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			log.Printf("source: %T", source)
			log.Printf("method: %s", method.Name)

			t := reflect.TypeOf(source)

			midx := -1
			for i := 0; i < t.NumMethod(); i++ {
				mm := t.Method(i)
				if mm.Name == method.Name {
					midx = i
					break
				}
			}

			if midx < 0 {
				return nil, fmt.Errorf("unable to execute")
			}

			value := reflect.ValueOf(source)
			res := value.Method(midx).Call(nil)
			for _, v := range res {
				return v.Interface(), nil
			}
			return nil, fmt.Errorf("no result")
		},
		Type:           retType,
		ParseArguments: nilParseArguments,
	}, nil
}
