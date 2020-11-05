package model

import (
	"log"
	"testing"

	"github.com/northvolt/thunder/graphql/introspection"
	"github.com/northvolt/thunder/graphql/schemabuilder"
)

func Test(t *testing.T) {
	s := schemabuilder.NewSchema()
	s.Object("EdgeGateway", EdgeGateway{})
	s.Interface("NorthvoltIdentity", new(NorthvoltIdentity))

	data, err := introspection.ComputeSchemaJSON(*s)
	if err != nil {
		t.Fatal(err)
	}

	log.Println(string(data))
}
