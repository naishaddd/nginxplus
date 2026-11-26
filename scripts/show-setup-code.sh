#!/bin/sh
# Prints the first-run setup code from an NGINX++ container's logs (waits up to ~20s).
C=${1:-nginxpp}
i=0
while [ $i -lt 20 ]; do
  line=$(docker logs "$C" 2>&1 | grep -E "Setup code:|admin account" | tail -1)
  if [ -n "$line" ]; then
    case "$line" in
      *"Setup code:"*) code=${line##*Setup code: }
        printf '\n  Open http://localhost:8000 and create the admin account.\n  Setup code: \033[1;32m%s\033[0m\n\n' "$code" ;;
      *) printf '\n  Admin account already exists. Open http://localhost:8000 and sign in.\n\n' ;;
    esac
    exit 0
  fi
  i=$((i+1)); sleep 1
done
echo "Could not read the setup code yet. Run: docker logs $C | grep 'Setup code'"
