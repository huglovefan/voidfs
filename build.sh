#!/bin/sh

export GOPATH=$PWD

if [ -z "$GOARCH" ]; then
	machine=$(uname -m)
	case $machine in
	x86_64) export GOARCH=386;;
	esac
fi

commit=$(git rev-parse --short HEAD)

rv=0
gofmt -d ./src/* || rv=1
go vet ./src/* || rv=1
go build -ldflags "-X main.commit=$commit" voidfs || rv=1
exit $rv
