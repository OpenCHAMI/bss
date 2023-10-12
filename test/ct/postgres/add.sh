#!/bin/sh
set -e
/usr/bin/hurl --test "$(dirname $0)"/add/*.hurl
