#!/bin/sh
# Delete a node and a boot config from BSS.
set -e
/usr/bin/hurl --test "$(dirname $0)"/delete/*.hurl
