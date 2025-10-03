#!/bin/bash
CURDIR=$(cd $(dirname $0); pwd)
BinaryName=content
echo "$CURDIR/bin/${BinaryName}"
exec $CURDIR/bin/${BinaryName}