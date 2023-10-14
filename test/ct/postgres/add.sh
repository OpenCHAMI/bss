#!/bin/sh
# Add node information and boot configs to BSS and SMD, showing
# the result.
set -e
/usr/bin/hurl --test "$(dirname $0)"/add/*.hurl
