#!/bin/bash
CURDIR=$(cd $(dirname $0); pwd)
BinaryName=scheduler
echo "$CURDIR/bin/${BinaryName}"
exec $CURDIR/bin/${BinaryName}