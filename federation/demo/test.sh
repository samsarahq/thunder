#!/bin/sh

go run ./userservice &
go run ./pictureservice &
sleep 1
go run ./gateway

killall userservice pictureservice
