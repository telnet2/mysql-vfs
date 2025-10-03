#!/bin/bash
CURDIR=$(cd $(dirname $0); pwd)
BinaryName=metadata
echo "$CURDIR/bin/${BinaryName}"
exec $CURDIR/bin/${BinaryName}