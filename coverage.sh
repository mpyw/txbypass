#!/usr/bin/env bash
# Fails when a statement is not covered, unless the code says why no input
# can reach it.
#
# Every package is measured against every test, so a branch the analyzer's
# fixtures reach counts for the package that holds it. A block may stay
# uncovered only when the comment right above its first statement starts
# with "Unreachable:" and says why. Anything else needs a test, or is
# deleted.
set -o pipefail

profile=$(mktemp)
trap 'rm -f "$profile"' EXIT

# -count=1: analysistest reads the fixtures through go list, which the test
# cache does not see, so a cached result can miss a fixture change.
go test -count=1 -coverpkg=./... -coverprofile="$profile" ./... > /dev/null || exit 1
module=$(go list -m)

# The profile lists a block once for each test binary. A block is covered
# when any of them ran it.
uncovered=$(awk 'NR > 1 {
	if (!($1 in count) || $3 > count[$1]) count[$1] = $3
} END {
	for (b in count) if (count[b] == 0) print b
}' "$profile" | sort)

failed=0
while read -r block; do
	[ -z "$block" ] && continue
	file=${block%%:*}
	file=${file#"$module"/}
	line=${block#*:}
	line=${line%%.*}
	# The comment above the block's first statement, read upward to its
	# first line.
	reason=$(awk -v l="$line" 'NR < l { lines[NR] = $0 } END {
		for (i = l - 1; i > 0 && lines[i] ~ /^[ \t]*\/\//; i--) first = lines[i]
		print first
	}' "$file")
	if [[ "$reason" =~ //[[:space:]]Unreachable: ]]; then
		echo "unreachable  $file:$line"
	else
		echo "UNCOVERED    $file:$line"
		failed=1
	fi
done <<< "$uncovered"

total=$(go tool cover -func="$profile" | awk '/^total:/ {print $3}')
echo "statement coverage $total"
exit $failed
