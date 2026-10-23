#!/usr/bin/env bash
set -euo pipefail
image=${1:?usage: dependency-image-smoke.sh IMAGE}
: "${EVIDENCE:=$PWD/.forge-evidence}"
run="forge091-matrix-$(date +%s)-$RANDOM"
mkdir -p "$EVIDENCE/$run"
fixture_dir="$EVIDENCE/$run"
chmod 755 "$fixture_dir"
network= containers=()
# Register intended names before creating resources; cleanup uses returned IDs.
printf '{"run":"%s","network":"%s","containers":["%s-rabbit","%s-matrix","%s-adapter","%s-client"],"host_ports":[],"storage":"fresh anonymous broker volumes"}\n' "$run" "$run" "$run" "$run" "$run" "$run" > "$fixture_dir/resources.json"
cleanup() {
  status=$?
  trap - EXIT
  for id in "${containers[@]}"; do
    docker logs "$id" > "$fixture_dir/$id.log" 2>&1 || :
    docker rm -fv "$id" >> "$fixture_dir/cleanup.log" 2>&1 || status=1
  done
  if [[ -n $network ]]; then docker network rm "$network" >> "$fixture_dir/cleanup.log" 2>&1 || status=1; fi
  printf '%s\n' "$status" > "$fixture_dir/exit-status"
  exit "$status"
}
trap cleanup EXIT
for name in "$run-rabbit" "$run-matrix" "$run-adapter" "$run-client"; do
  if docker container inspect "$name" >/dev/null 2>&1; then echo "Fixture name already exists" >&2; exit 1; fi
done
if docker network inspect "$run" >/dev/null 2>&1; then exit 1; fi
CGO_ENABLED=0 go build -o "$fixture_dir/smoke" .scripts/dependency-image-smoke.go
chmod 755 "$fixture_dir/smoke"
network=$(docker network create --internal "$run")
printf '%s\n' "$network" > "$fixture_dir/network.id"
rabbit=$(docker create --network "$network" --network-alias rabbit --name "$run-rabbit" -e RABBITMQ_DEFAULT_USER=fixture -e RABBITMQ_DEFAULT_PASS=fixture -e RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS='+S 2:2' rabbitmq@sha256:d7af1c87c5f1eda13fcfca06db452bf3aeab6619fc3358b68535c0c02c4e52bc)
containers+=("$rabbit"); printf '%s\n' "$rabbit" >> "$fixture_dir/containers.ids"
docker start "$rabbit" >/dev/null
ready=false
for _ in $(seq 1 60); do
  if docker exec "$rabbit" rabbitmq-diagnostics -q ping >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
[[ $ready == true ]]
fixture=$(docker create --network "$network" --network-alias matrix --name "$run-matrix" --mount "type=bind,source=$fixture_dir,target=/fixture,readonly" --entrypoint /fixture/smoke "$image" serve)
containers+=("$fixture"); printf '%s\n' "$fixture" >> "$fixture_dir/containers.ids"
docker start "$fixture" >/dev/null
adapter=$(docker create --network "$network" --network-alias adapter --name "$run-adapter" -e CONFIG_PATH=/absent-fixture-config -e SYNAPSE_SERVER_URL=http://matrix:8081 -e SYNAPSE_HOMESERVER_NAME=fixture.invalid -e MATRIX_AS_TOKEN=fixture-as-token -e MATRIX_HS_TOKEN=fixture-hs-token -e RABBITMQ_URL=amqp://fixture:fixture@rabbit:5672/ "$image")
containers+=("$adapter"); printf '%s\n' "$adapter" >> "$fixture_dir/containers.ids"
docker cp "$adapter:/app/matrix-adapter" "$fixture_dir/matrix-adapter"
go version -m "$fixture_dir/matrix-adapter" > "$fixture_dir/binary-modules.txt"
docker start "$adapter" >/dev/null
client=$(docker create --network "$network" --name "$run-client" --mount "type=bind,source=$fixture_dir,target=/fixture,readonly" --entrypoint /fixture/smoke "$image" verify)
containers+=("$client"); printf '%s\n' "$client" >> "$fixture_dir/containers.ids"
docker start -a "$client"
[[ $(docker inspect --format '{{.State.ExitCode}}' "$client") == 0 ]]
echo "Final image broker-to-Matrix RPC success, forbidden error, correlation and acknowledgement checks passed"
