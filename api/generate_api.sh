#!/bin/zsh

mkdir -p ../monitor/apiserver
mkdir -p ../healthcheck/apiclient
oapi-codegen -generate types,server,spec -package apiserver ./longhorn-monitor_openapi.yaml > ../monitor/apiserver/apiserver.gen.go
oapi-codegen -generate types,client -package apiclient ./longhorn-monitor_openapi.yaml > ../healthcheck/apiclient/apiclient.gen.go