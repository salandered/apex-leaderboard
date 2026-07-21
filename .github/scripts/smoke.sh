#!/usr/bin/env bash
# Smoke test: wait for the composed stack and assert the root endpoint serves 200.
# A 200 on "/" means fully up: the server only starts after Redis connects and the
# main board is seeded (see main.go).

# TODO: make go or python script
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8090}"
attempts="${ATTEMPTS:-60}"

for i in $(seq 1 "$attempts"); do
	code="$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/" || true)"
	if [ "$code" = "200" ]; then
		echo "SMOKE OK: $BASE_URL/ -> 200"
		exit 0
	fi
	echo "waiting for $BASE_URL/ (attempt $i/$attempts, last=$code)"
	sleep 2
done

echo "SMOKE FAIL: $BASE_URL/ did not return 200 after $attempts attempts" >&2
exit 1
