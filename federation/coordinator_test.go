package federation

import (
	"testing"
//
	// "github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	// "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)



func setupPlanner(t *testing.T) (*Planner, error) {
	schemas := map[string]map[string]*schemabuilder.Schema{
		"schema1": {
			"schema1": buildTestSchema1(),
		},
		"schema2": {
			"schema2": buildTestSchema2(),
		},
	}

	builtSchemas := make(serviceSchemas)
	for service, versions := range schemas {
		builtSchemas[service] = make(map[string]*introspectionQueryResult)
		for version, schema := range versions {
			builtSchemas[service][version] = extractSchema(t, schema.MustBuild())
		}
	}
	merged, err := convertVersionedSchemas(builtSchemas)
	require.NoError(t, err)

	f, err := newFlattener(merged.Schema)
	return &Planner{
		flattener: f,
		schema:    merged,
	}, nil
}



func TestPlanner(t *testing.T) {
	e, err := setupPlanner(t)
	require.NoError(t, err)
	schema := buildTestSchema1()
	srv, err := NewServer(schema.MustBuild())
	// schema2 := buildTestSchema2()


}
