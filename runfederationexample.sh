#!/bin/sh

go run ./federationexample/devicegqlserver &
go run ./federationexample/safetyservice &
go run ./federationexample/gqlgateway &
sleep 1
go run ./federationexample/server

killall devicegqlserver safetyservice gqlgateway server
