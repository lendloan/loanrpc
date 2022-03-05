#!/bin/bash

protoc --proto_path=$GOPATH/src/heegproto:. --go_out=. *.proto