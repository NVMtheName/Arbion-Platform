#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-lightsail-deploy-test.XXXXXX")"
cleanup() {
  if [[ -d "$test_root" ]]; then
    find "$test_root" -depth -delete
  fi
}
trap cleanup EXIT

stub_bin="$test_root/stub-bin"
mkdir -p "$stub_bin"
scp_log="$test_root/scp.log"
ssh_log="$test_root/ssh.log"
ssh_input="$test_root/ssh-input"
key="$test_root/deploy-key"
printf '%s\n' 'test-only key placeholder' >"$key"
chmod 0600 "$key"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\n" "$*" >>"$ARBION_TEST_SCP_LOG"' \
  >"$stub_bin/scp"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\n" "$*" >>"$ARBION_TEST_SSH_LOG"' \
  'cat >"$ARBION_TEST_SSH_INPUT"' \
  >"$stub_bin/ssh"
chmod 0755 "$stub_bin/scp" "$stub_bin/ssh"

PATH="$stub_bin:$PATH" \
  ARBION_TEST_SCP_LOG="$scp_log" \
  ARBION_TEST_SSH_LOG="$ssh_log" \
  ARBION_TEST_SSH_INPUT="$ssh_input" \
  "$repo_root/scripts/deploy-lightsail-release.sh" HEAD ubuntu@127.0.0.1 "$key" >"$test_root/output"

grep -q "Lightsail deployment completed" "$test_root/output"
grep -q "arbion-release-" "$scp_log"
grep -q -- "sudo -n bash -s" "$ssh_log"
grep -q "release_sha='" "$ssh_input"
grep -q "sha256sum" "$ssh_input"
grep -q "systemctl start arbion-postgres-backup.service" "$ssh_input"
grep -q "PRE_DEPLOY_BACKUP=verified" "$ssh_input"
grep -q "check-production-containers.sh" "$ssh_input"
grep -q "ARBION_DEPLOYMENT_EVIDENCE_JSON=" "$ssh_input"
grep -q 'backup_completed_epoch.*-ge.*deployment_started_epoch' "$ssh_input"
grep -q 'replacement_started_epoch=' "$ssh_input"
grep -q 'replacement_completed_epoch=' "$ssh_input"
grep -q 'rollback_sha256=' "$ssh_input"
bash -n "$ssh_input"

source_umask_line="$(grep -n '^umask 022$' "$ssh_input" | cut -d: -f1)"
extraction_line="$(grep -n '^tar --no-same-owner --no-same-permissions ' "$ssh_input" | cut -d: -f1)"
[[ -n "$source_umask_line" && -n "$extraction_line" && "$source_umask_line" -lt "$extraction_line" ]] || {
  echo "Lightsail deploy does not set canonical source permissions before extraction." >&2
  exit 1
}

backup_line="$(grep -n "systemctl start arbion-postgres-backup.service" "$ssh_input" | cut -d: -f1)"
release_line="$(grep -n "rsync -a --delete" "$ssh_input" | cut -d: -f1)"
[[ "$backup_line" -lt "$release_line" ]] || {
  echo "Lightsail deploy does not require a backup before replacing production code." >&2
  exit 1
}
receipt_line="$(grep -n 'receipt=' "$ssh_input" | cut -d: -f1)"
health_line="$(grep -n '^./scripts/check-production-containers.sh' "$ssh_input" | cut -d: -f1)"
[[ "$receipt_line" -gt "$health_line" ]] || {
  echo "Deployment receipt is emitted before production health is proven." >&2
  exit 1
}

# Execute only the credential-free receipt renderer with synthetic values.
# Never execute the captured remote deployment during a test.
deployment_started_epoch=1788969240 backup_completed_epoch=1788969300
replacement_started_epoch=1788969360 replacement_completed_epoch=1788969420
release_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
current_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
expected_sha=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
rollback_sha256=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
rollback=/opt/arbion/.rollback/release-pre-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-20260909T155505Z.tar.gz
backup_object_key=postgres/daily/arbion-fixture.dump
renderer="$(sed -n '/^receipt=/,/^printf '\''ARBION_DEPLOYMENT_EVIDENCE_JSON=/p' "$ssh_input")"
[[ -n "$renderer" ]] || { echo 'Deployment receipt renderer missing.' >&2; exit 1; }
rendered="$(eval "$renderer")"
receipt_json="${rendered#ARBION_DEPLOYMENT_EVIDENCE_JSON=}"
jq -e '.schema_version == "1.0" and .source == "LIGHTSAIL_DEPLOY_SCRIPT" and .release_sha == "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" and .backup.completed_epoch == 1788969300 and .checks == {readiness:"PASS",public_smoke:"PASS",containers:"PASS"}' <<<"$receipt_json" >/dev/null

if "$repo_root/scripts/deploy-lightsail-release.sh" HEAD 'invalid host' "$key" >"$test_root/invalid-host" 2>&1; then
  echo "Lightsail deploy accepted an invalid host." >&2
  exit 1
fi
grep -q "SSH host must contain" "$test_root/invalid-host"

echo "Lightsail deployment helper tests passed."
