

package main

import (
	"context"
	"log"
	"net"
	
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"

)

func schema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchemaWithName("device")


	type Location struct {
		Latitude float64
		Longitude float64
	}
	location := schema.Object("Location", Location{}, 
		schemabuilder.FetchObjectFromKeys(func(ctx context.Context, args struct{ Keys []*Location }) ([]*Location) {
			return args.Keys
		}),
	)

	location.FieldFunc("street", func(ctx context.Context, location *Location) (string , error) {
		return "My street address", nil
	})



	type Device struct {
		Id int64
		Name string
	}
	type DeviceKey struct {
		Id int64
	}
		device := schema.Object("Device", Device{},
		schemabuilder.FetchObjectFromKeys(func(ctx context.Context, args struct{ Keys []*DeviceKey }) ([]*Device, error) {
			devices := make([]*Device, 0, len(args.Keys))
			for _, key := range args.Keys {
				// Fetch the full device from db, but for the example we're setting name to testDevice
				devices = append(devices, &Device{Id: key.Id, Name: "testDevice"})
			}

			return devices, nil
		}),
	)

	device.Key("id")


	device.FieldFunc("location", func(ctx context.Context, device *Device) (*Location , error) {
		return &Location{Latitude: 123.123, Longitude: 456.456}, nil
	})


	schema.Query().FieldFunc("device", func(ctx context.Context) (*Device , error) {
		return &Device{Id: 1, Name: "testDevice"}, nil
	})

	return schema
} 



func main() {
	schema := schema().MustBuild()
	server, err := federation.NewServer(schema)
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	thunderpb.RegisterExecutorServer(grpcServer, server)

	listener, err := net.Listen("tcp", ":1234")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}
