#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 5 ]]; then
  echo "usage: deploy-production.sh RELEASE_ID VERSION BUCKET OBJECT_KEY SHA256" >&2
  exit 2
fi

release_id=$1
version=$2
bucket=$3
object_key=$4
expected_sha=$5

[[ $release_id =~ ^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}$ ]]
[[ $version =~ ^shejane-beta-[0-9]{8}\.[0-9]+$ ]]
[[ $bucket =~ ^[a-z0-9][a-z0-9.-]+[a-z0-9]$ ]]
[[ $object_key == "releases/$release_id/new-api" ]]
[[ $expected_sha =~ ^[0-9a-f]{64}$ ]]

deploy_root=/srv/shejane-cloud
release_dir="$deploy_root/releases/$release_id"
current_link="$deploy_root/current"
environment_file="$deploy_root/shared/shejane-cloud.env"
backup_root="/srv/backups/shejane-cloud/$release_id"
main_container=shejane-cloud-canary
canary_container=shejane-cloud-next
runtime_image=shejane-cloud-runtime:bookworm-ca-20260729

exec 9>/var/lock/shejane-cloud-deploy.lock
flock -n 9 || { echo "another SheJane Cloud deployment is running" >&2; exit 1; }

previous_release=$(readlink -f "$current_link")
[[ -x "$previous_release/new-api" ]]
[[ -f $environment_file ]]
docker inspect "$main_container" >/dev/null
docker network inspect shejane_default >/dev/null

run_container() {
  local name=$1
  local port=$2
  local restart=$3
  local binary=$4
  docker run -d \
    --name "$name" \
    --restart "$restart" \
    --network shejane_default \
    --env-file "$environment_file" \
    -p "127.0.0.1:$port:3000" \
    -v "$binary:/opt/shejane-cloud/new-api:ro" \
    -v "$deploy_root/data:/data" \
    -v "$deploy_root/logs:/app/logs" \
    --entrypoint /opt/shejane-cloud/new-api \
    "$runtime_image" \
    --log-dir /app/logs >/dev/null
}

wait_for_version() {
  local port=$1
  local expected=$2
  local actual
  for _ in {1..30}; do
    actual=$(curl -fsS --max-time 2 "http://127.0.0.1:$port/api/status" 2>/dev/null \
      | jq -r '.data.version // empty' || true)
    [[ $actual == "$expected" ]] && return 0
    sleep 1
  done
  return 1
}

wait_for_health() {
  local port=$1
  for _ in {1..30}; do
    curl -fsS --max-time 2 "http://127.0.0.1:$port/api/status" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

rollback_main() {
  docker rm -f "$main_container" >/dev/null 2>&1 || true
  run_container "$main_container" 3001 unless-stopped "$previous_release/new-api"
  wait_for_health 3001
  ln -sfn "$previous_release" "$current_link"
}

cleanup_canary() {
  docker rm -f "$canary_container" >/dev/null 2>&1 || true
}
trap cleanup_canary EXIT

/usr/local/sbin/shejane-cloud-backup
install -d -m 0700 "$backup_root"
install -m 0600 "$environment_file" "$backup_root/shejane-cloud.env"

if [[ -e $release_dir ]]; then
  [[ $(sha256sum "$release_dir/new-api" | cut -d' ' -f1) == "$expected_sha" ]]
else
  install -d -m 0755 "$release_dir"
  aws s3 cp "s3://$bucket/$object_key" "$release_dir/new-api" \
    --region us-east-1 \
    --only-show-errors
  chmod 0755 "$release_dir/new-api"
fi
[[ $(sha256sum "$release_dir/new-api" | cut -d' ' -f1) == "$expected_sha" ]]

cleanup_canary
run_container "$canary_container" 3002 no "$release_dir/new-api"
wait_for_version 3002 "$version"

if ! {
  docker stop -t 30 "$main_container" >/dev/null
  docker rm "$main_container" >/dev/null
  run_container "$main_container" 3001 unless-stopped "$release_dir/new-api"
  wait_for_version 3001 "$version"
  ln -sfn "$release_dir" "$current_link"
}; then
  echo "promotion failed; restoring $previous_release" >&2
  rollback_main
  exit 1
fi

cleanup_canary
trap - EXIT
echo "deployed $version from $release_dir"
