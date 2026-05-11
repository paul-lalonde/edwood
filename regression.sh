#!/bin/bash
# regression.sh — mirror the CI gate locally.
#
# Runs the same checks as .github/workflows/edwood.yml so a green
# regression.sh predicts a green CI. staticcheck and misspell are
# invoked via `go run @version` instead of `go install` so the runner
# does not require $GOPATH/bin entries.
#
# Exit status: 0 if every stage passes; non-zero on the first failure.

set -e
set -o pipefail

cd "$(dirname "$0")"

STATICCHECK_VERSION=v0.6.1
MISSPELL_VERSION=v0.3.4

step() {
	printf '\n== %s ==\n' "$1"
}

step "gofmt -d -s ."
diff -u <(echo -n) <(gofmt -d -s .)

step "go vet ."
go vet .

step "staticcheck"
go run "honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}" \
	-checks inherit,-U1000,-SA4003 ./...

step "misspell -error ."
go run "github.com/client9/misspell/cmd/misspell@${MISSPELL_VERSION}" -error .

step "go test -race ./..."
go test -race ./...

printf '\nregression.sh: all checks passed\n'
