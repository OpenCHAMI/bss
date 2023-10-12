#!/bin/sh
set -e
/usr/bin/hurl --test "$(dirname $0)"/delete/*.hurl
