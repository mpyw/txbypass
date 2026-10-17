#!/usr/bin/env bash
# Fails when statement coverage of the module falls below the floor.
#
# Every package is measured against every test, so a branch the analyzer's
# fixtures reach counts for the package that holds it. What is not covered
# should be a guard no input reaches: design/implementation.md lists them.
set -o pipefail

floor=99.5
profile=$(mktemp)
trap 'rm -f "$profile"' EXIT

go test -coverpkg=./... -coverprofile="$profile" ./... > /dev/null || exit 1
total=$(go tool cover -func="$profile" | awk '/^total:/ {sub("%", "", $3); print $3}')
echo "statement coverage ${total}% (floor ${floor}%)"
awk -v t="$total" -v f="$floor" 'BEGIN { exit !(t >= f) }'
