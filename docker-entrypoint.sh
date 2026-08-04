#!/bin/sh
set -e

if [ -f /vault-creds/creds.env ]; then
  set -a
  . /vault-creds/creds.env
  set +a
fi

exec /agentplane
