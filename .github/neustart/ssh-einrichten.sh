#!/usr/bin/env bash
# SSH-Schluessel ablegen und nur den Host-Schluessel mit dem erwarteten
# Fingerabdruck annehmen (KEY, HOST, FP aus der Umgebung des Workflows).
set -euo pipefail
mkdir -p ~/.ssh && chmod 700 ~/.ssh
printf '%s\n' "$KEY" > ~/.ssh/k && chmod 600 ~/.ssh/k
ssh-keyscan "$HOST" 2>/dev/null > ~/.ssh/alle
: > ~/.ssh/known_hosts
while IFS= read -r z; do
  [ -n "$z" ] || continue
  printf '%s\n' "$z" > ~/.ssh/eine
  if ssh-keygen -lf ~/.ssh/eine 2>/dev/null | awk '{print $2}' | grep -qx "$FP"; then
    printf '%s\n' "$z" >> ~/.ssh/known_hosts
  fi
done < ~/.ssh/alle
[ -s ~/.ssh/known_hosts ] || { echo "::error::Host-Schluessel von $HOST passt nicht zu $FP"; exit 1; }
