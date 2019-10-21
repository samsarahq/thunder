package main

import "github.com/samsarahq/thunder/graphql"

type Executor struct {
	RemoteSchemas *graphql.Schema
}

type TypeName string

type Type interface{}

type Scalar struct{}

type Field struct {
	// XXX: service(s)?
	Service string
	Args    map[string]TypeName
}

type InputObject struct {
}

type Object struct {
	Fields map[string]Field
}

type Batch []interface{}

func (e *Executor) execute(input Batch, typ *Object, selectionSet *graphql.SelectionSet) {
	// map from service to selection set
	requests := make(map[string]*graphql.RawSelectionSet)

	// XXX: raw or not?
	selections := graphql.Flatten(*graphql.SelectionSet)

	for _, selection := range selections {
		field := typ.Fields[selection.Name]
		field.Service
	}
}

type SubPlan struct {
	Path []string
	*Plan
}

type Plan struct {
	Service      string
	SelectionSet *graphql.SelectionSet
	After        []SubPlan
}

func (e *Executor) plan(typ *Object, selectionSet *graphql.SelectionSet) {
	// map from service to selection set
	requests := make(map[string]*graphql.RawSelectionSet)

	// XXX: raw or not?
	selections := graphql.Flatten(*graphql.SelectionSet)

	for _, selection := range selections {
		field := typ.Fields[selection.Name]
		field.Service
	}
}
