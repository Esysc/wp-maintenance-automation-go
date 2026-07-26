#!/bin/sh
if [ -n "$RESTIC_PASSWORD" ] && [ -n "$RESTIC_PASSWORD_FILE" ]; then
  mkdir -p "$(dirname "$RESTIC_PASSWORD_FILE")"
  printf '%s' "$RESTIC_PASSWORD" > "$RESTIC_PASSWORD_FILE"
fi
exec "$@"
