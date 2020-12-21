

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
	schema := schemabuilder.NewSchemaWithName("safety")

	type Location struct {
		Latitude float64
		Longitude float64
	}
	schema.Object("Location", Location{}, 
		schemabuilder.FetchObjectFromKeys(func(ctx context.Context, args struct{ Keys []*Location }) ([]*Location) {
			// In this case the keys are a full location object, so we can just return the keys
			return args.Keys
		}),
	)

	type Camera struct {
		Id int64
		IsOn bool
	}
	camera := schema.Object("Camera", Camera{})
	camera.FieldFunc("isMultiCam", func(ctx context.Context, camera *Camera) (bool , error) {
		return false, nil
	})
	camera.FieldFunc("cameraLocation", func(ctx context.Context, camera *Camera) (*Location , error) {
		return &Location{Latitude: 246.245, Longitude: 135.135}, nil
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
				// Fecth the full device from db, but for the example we're setting name to testDevice
				devices = append(devices, &Device{Id: key.Id, Name: "testDevice"})
			}
			return devices, nil
		}),
	)
	device.Key("id")
	device.FieldFunc("camera", func(ctx context.Context, device *Device) ([]*Camera , error) {
		return []*Camera{&Camera{Id: 1, IsOn: true}, &Camera{Id: 2, IsOn: true}}, nil
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

	listener, err := net.Listen("tcp", ":1235")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}
