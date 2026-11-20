#!/usr/bin/env bash
set -euo pipefail

container_name="gramsrv-main_deltament_1"

runtime=
if command -v podman >/dev/null 2>&1; then
  runtime=podman
elif command -v docker >/dev/null 2>&1; then
  runtime=docker
else
  printf 'stop-deltament: neither podman nor docker is available\n' >&2
  exit 1
fi

container_id=$("$runtime" ps -q --filter "name=^/${container_name}$" | head -n 1)
if [[ -z "$container_id" ]]; then
  printf 'stop-deltament: %s is already stopped or missing\n' "$container_name"
  exit 0
fi

"$runtime" stop -t 30 "$container_id"
