#!/bin/sh

go run ./service1 &
go run ./service2 &
sleep 1
go run ./gateway

killall service1 service2
