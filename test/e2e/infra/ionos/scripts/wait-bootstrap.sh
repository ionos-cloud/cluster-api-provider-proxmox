#!/usr/bin/env bash
# Waits until every node in $HOSTS reports a finished bootstrap; fails as soon as any node reports an error.
set -euo pipefail

deadline=$((SECONDS + ${BOOTSTRAP_TIMEOUT:-3600}))
pending=$HOSTS
last_error=""
errfile=$(mktemp)
trap 'rm -f "$errfile"' EXIT

ssh_node() {
  local host=$1
  local command=$2
  ssh -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=yes \
    -o BatchMode=yes -o ConnectTimeout=10 "root@$host" "$command"
  return $?
}

while [[ -n "$pending" ]]; do
  still_pending=""
  for host in $pending; do
    if ! state=$(ssh_node "$host" 'if [ -f /root/pve-bootstrap/failed ]; then echo failed; elif [ -f /root/pve-bootstrap/done ]; then echo done; fi' 2>"$errfile"); then
      last_error="$host: $(tail -n 1 "$errfile")"
      still_pending="$still_pending $host"
      continue
    fi
    case "$state" in
    done) echo "$host bootstrapped" ;;
    failed)
      echo "bootstrap failed on $host:" >&2
      ssh_node "$host" 'tail -n 50 /var/log/pve-bootstrap.log' >&2 || true
      exit 1
      ;;
    *) still_pending="$still_pending $host" ;;
    esac
  done
  pending=$still_pending
  [[ -n "$pending" ]] || break
  if [[ "$SECONDS" -ge "$deadline" ]]; then
    echo "timed out waiting for:$pending" >&2
    [[ -z "$last_error" ]] || echo "last ssh error: $last_error" >&2
    exit 1
  fi
  sleep 20
done
