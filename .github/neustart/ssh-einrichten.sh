#!/usr/bin/env bash
# SSH-Schluessel ablegen und nur den Host-Schluessel mit dem erwarteten
# Fingerabdruck annehmen (KEY, HOST, FP aus der Umgebung des Workflows).
set -euo pipefail
mkdir -p ~/.ssh && chmod 700 ~/.ssh
printf '%s' "$KEY" | perl -0777 -pe 's/\r//g; s/(?:-|\xE2\x80[\x93\x94])+ *(BEGIN|END) OPENSSH PRIVATE KEY *(?:-|\xE2\x80[\x93\x94])+/-----$1 OPENSSH PRIVATE KEY-----/g; unless (/-----BEGIN OPENSSH PRIVATE KEY-----/) { (my $z = $_) =~ s/\s//g; if ($z =~ m|^[A-Za-z0-9+/]+=*$|) { require MIME::Base64; my $d = MIME::Base64::decode_base64($z); if ($d =~ /-----BEGIN OPENSSH PRIVATE KEY-----/) { $_ = $d; } elsif ($d =~ /\Aopenssh-key-v1\x00/) { $_ = qq{-----BEGIN OPENSSH PRIVATE KEY-----\n$z\n-----END OPENSSH PRIVATE KEY-----\n}; } } } s/\s*(-----BEGIN OPENSSH PRIVATE KEY-----)\s*(.*?)\s*(-----END OPENSSH PRIVATE KEY-----)\s*/my ($k, $b, $f) = ($1, $2, $3); $b =~ s|\s||g; $b =~ s|(.{70})|$1\n|g; $b =~ s|\n\z||; "$k\n$b\n$f\n"/se' > ~/.ssh/k && chmod 600 ~/.ssh/k
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
