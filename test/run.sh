#!/usr/bin/env bash
# Build the test container and run sss against it.
#   test/run.sh                      # arm64, user "tester" (sudo), plain output
#   PLATFORM=linux/amd64 test/run.sh # x86_64 under emulation
#   SSS_USER=plain test/run.sh       # no root: needs-root steps must be skipped
#   SSS_USER=pwuser SSS_SUDO_PASSWORD=testpw test/run.sh --yes   # sudo with a password
#   BASE=debian:12 test/run.sh       # Debian instead of Ubuntu
#   KEEP=1 test/run.sh               # leave the container running (ssh -p 2222 -i test/keys/id_ed25519 tester@localhost)
# Extra arguments are passed to sss.
set -euo pipefail
cd "$(dirname "$0")/.."
PLATFORM=${PLATFORM:-linux/arm64}; SSS_USER=${SSS_USER:-tester}; BASE=${BASE:-ubuntu:24.04}
NAME=sss-test-target; PORT=${PORT:-2222}
mkdir -p test/keys
[ -f test/keys/id_ed25519 ] || ssh-keygen -q -t ed25519 -N '' -f test/keys/id_ed25519
go build -o sss .
docker build -q --platform "$PLATFORM" --build-arg BASE="$BASE" -t sss-test-target -f test/Dockerfile test >/dev/null
docker rm -f $NAME >/dev/null 2>&1 || true
docker run -d --rm --platform "$PLATFORM" --name $NAME -p $PORT:22 sss-test-target >/dev/null
[ -n "${KEEP:-}" ] || trap 'docker rm -f $NAME >/dev/null 2>&1 || true' EXIT
for _ in $(seq 1 30); do
  ssh -q -p $PORT -i test/keys/id_ed25519 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes \
      $SSS_USER@localhost true 2>/dev/null && break; sleep 1
done
./sss -p $PORT -i test/keys/id_ed25519 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
  "$@" $SSS_USER@localhost
