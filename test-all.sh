#!/bin/bash
set -x

pushd editor
go test ./...
popd

pushd extras
go test ./...
popd
