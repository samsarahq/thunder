package federation

import (
	"encoding/json"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/stretchr/testify/assert"
)

func TestAddIntrospectionQueryToSchemaVersions(t *testing.T) {
	schema1 := buildTestSchema1()
	schema1Bytes, err := introspection.ComputeSchemaJSON(*schema1)
	assert.Nil(t, err)
	var iq1 IntrospectionQueryResult
	assert.Nil(t, json.Unmarshal(schema1Bytes, &iq1))

	schema2 := buildTestSchema2()
	schema2Bytes, err := introspection.ComputeSchemaJSON(*schema2)
	assert.Nil(t, err)
	var iq2 IntrospectionQueryResult
	assert.Nil(t, json.Unmarshal(schema2Bytes, &iq2))

	schemaVersions := map[string]map[string]*IntrospectionQueryResult{
		"service1": {
			"": &iq1,
		},
		"service2": {
			"": &iq2,
		},
	}

	schemaVersionsWithIntrospection, err := AddIntrospectionQueryToSchemaVersions(schemaVersions)
	assert.Nil(t, err)

	assert.Equal(t, schemaVersions["service1"], schemaVersionsWithIntrospection["service1"])
	assert.Equal(t, schemaVersions["service2"], schemaVersionsWithIntrospection["service2"])

	introspectionSchema := schemaVersionsWithIntrospection[IntrospectionClientName][""]
	assert.NotNil(t, introspectionSchema)

	var containsSchemaField bool
	for _, typ := range introspectionSchema.Schema.Types {
		if typ.Name == "__Schema" {
			containsSchemaField = true
			break
		}
	}
	assert.True(t, containsSchemaField)

	snapshotter := snapshotter.New(t)
	defer snapshotter.Verify()

	snapshotter.Snapshot("schema with introspection query", introspectionSchema.Schema)
}
