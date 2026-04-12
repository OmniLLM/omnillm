#!/bin/sh
if [ "$1" = "--auth" ]; then
  exec /app/omnimodel auth
else
  exec /app/omnimodel start --port 5002 "$@"
fi
