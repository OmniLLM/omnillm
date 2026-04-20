#!/bin/sh
if [ "$1" = "--auth" ]; then
  exec /app/omnillm auth
else
  exec /app/omnillm start --port 5002 "$@"
fi
