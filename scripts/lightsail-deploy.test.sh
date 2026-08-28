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
grep -q "check-production-containers.sh" "$ssh_input"

if "$repo_root/scripts/deploy-lightsail-release.sh" HEAD 'invalid host' "$key" >"$test_root/invalid-host" 2>&1; then
  echo "Lightsail deploy accepted an invalid host." >&2
  exit 1
fi
grep -q "SSH host must contain" "$test_root/invalid-host"

echo "Lightsail deployment helper tests passed."
