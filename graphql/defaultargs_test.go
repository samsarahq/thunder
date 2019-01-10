package graphql_test

import (
	"testing"

	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal/testgraphql"
)

func TestDefaultArgs(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type Inner struct {
		OptionalValue string
		RequiredValue string
	}

	query := schema.Query()
	query.FieldFunc("inner", func(input struct {
		OptionalInput string `graphql:",optional"`
		RequiredInput string
	}) Inner {
		return Inner{
			OptionalValue: input.OptionalInput,
			RequiredValue: input.RequiredInput,
		}
	})

	_ = schema.Mutation()

	builtSchema := schema.MustBuild()

	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("happy path all provided", `{
		inner(
			optionalInput: "teeeeeeeest", 
			requiredInput: "requiredInput!", 
		) { 
			optionalValue
			requiredValue
		}
	}`)

	snap.SnapshotQuery("missing required parameter", `{
		inner(
			optionalInput: "teeeeeeeest", 
		) { 
			optionalValue
			requiredValue
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("missing optional parameter does not error", `{
		inner(
			requiredInput: "teeeeeeeest", 
		) { 
			optionalValue
			requiredValue
		}
	}`)
}
