#!/bin/sh
# Run all integration tests.

set -e

# Test add/delete/get boot config by XName.
/usr/bin/hurl --test "$(dirname $0)"/tests/xname/*.hurl
