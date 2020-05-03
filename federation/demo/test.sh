#!/bin/sh

go run ./userservice &
go run ./pictureservice &
go run ./gatewayExecutor &
sleep 1
go run ./gateway

killall userservice pictureservice
