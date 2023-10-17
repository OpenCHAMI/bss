#!/bin/sh
# Run all integration tests.

set -e

# Test add/delete/get boot config by XName.
echo '============================='
echo 'EXECUTING xname UNIT TESTS...'
echo '============================='
/usr/bin/hurl --test "$(dirname $0)"/tests/xname/*.hurl

# Test add/delete/get boot config by MAC address.
echo '============================='
echo 'EXECUTING mac UNIT TESTS...'
echo '============================='
/usr/bin/hurl --test "$(dirname $0)"/tests/mac/*.hurl

echo '============================='
echo 'ALL UNIT TESTS COMPLETED'
echo '============================='
