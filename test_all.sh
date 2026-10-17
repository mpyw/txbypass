#!/usr/bin/env bash
# Runs every check CI runs. Use it before committing.

set -o pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

declare -a failed_tests=()

run_test() {
    local name="$1"
    shift

    echo "=== $name ==="
    if "$@"; then
        echo -e "${GREEN}[$name] OK${NC}"
    else
        echo -e "${RED}[$name] FAILED${NC}"
        failed_tests+=("$name")
    fi
}

run_test "test" go test ./...
run_test "lint" golangci-lint run ./...
# shrink goes first: a declaration it would unexport becomes private to its
# file, and the analyzer then judges that narrower scope.
run_test "shrink" declscope shrink ./...
run_test "declscope" declscope ./...
# The engine is held near full coverage. What is left are guards that no
# input reaches, listed in design/implementation.md.
run_test "coverage" ./coverage.sh

echo ""
echo "===== Summary ====="
if [ ${#failed_tests[@]} -eq 0 ]; then
    echo -e "${GREEN}All checks passed!${NC}"
    exit 0
fi
echo -e "${RED}Failed:${NC}"
for t in "${failed_tests[@]}"; do
    echo -e "  ${RED}- $t${NC}"
done
exit 1
