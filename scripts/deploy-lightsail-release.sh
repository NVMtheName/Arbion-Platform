#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "Usage: $0 <git-ref> <ssh-host> <ssh-private-key>" >&2
  exit 2
fi

git_ref="$1"
ssh_host="$2"
ssh_key="$3"
[[ "$git_ref" != -* ]] || {
  echo "Git ref must not begin with '-'." >&2
  exit 2
}
[[ "$ssh_host" =~ ^[A-Za-z0-9._@:-]+$ ]] || {
  echo "SSH host must contain only a user, hostname, IPv4 address, or port separators." >&2
  exit 2
}
[[ -r "$ssh_key" ]] || {
  echo "SSH private key is unavailable: $ssh_key" >&2
  exit 1
}

for command in git mktemp find shasum ssh scp; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir/.." rev-parse --show-toplevel)"
release_sha="$(git -C "$repo_root" rev-parse --verify "$git_ref^{commit}")"
transfer_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
transfer_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-lightsail-deploy.XXXXXX")"
release_archive="$transfer_root/arbion-release-${release_sha}.tar.gz"
cleanup() {
  if [[ -d "$transfer_root" ]]; then
    find "$transfer_root" -depth -delete
  fi
}
trap cleanup EXIT

"$repo_root/scripts/package-release.sh" "$release_sha" "$release_archive"
archive_sha="$(shasum -a 256 "$release_archive" | awk '{print $1}')"
remote_archive="/tmp/arbion-release-${release_sha}-${transfer_id}.tar.gz"

scp -i "$ssh_key" -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new \
  "$release_archive" "$ssh_host:$remote_archive"

ssh -i "$ssh_key" -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new "$ssh_host" 'sudo -n bash -s' <<REMOTE
set -Eeuo pipefail
release_sha='$release_sha'
archive='$remote_archive'
expected_sha='$archive_sha'

[[ -d /opt/arbion && -r /opt/arbion/.env.production ]]
[[ -f "\$archive" ]]
for command in cat curl date docker find grep mktemp mv rsync sha256sum stat tar unlink; do
  command -v "\$command" >/dev/null || {
    echo "Required host command not found: \$command" >&2
    exit 1
  }
done
actual_sha="\$(sha256sum "\$archive" | awk '{print \$1}')"
[[ "\$actual_sha" == "\$expected_sha" ]]
marker="\$(tar -xOf "\$archive" ./.release-sha)"
[[ "\$marker" == "\$release_sha" ]]

stage=''
rollback_tmp=''
cleanup() {
  if [[ -n "\${stage:-}" && -d "\$stage" ]]; then
    find "\$stage" -depth -delete
  fi
  if [[ -n "\${rollback_tmp:-}" && -f "\$rollback_tmp" ]]; then
    unlink "\$rollback_tmp"
  fi
}
trap cleanup EXIT

stage="\$(mktemp -d /opt/arbion/.incoming.XXXXXX)"
tar --no-same-owner --no-same-permissions -xzf "\$archive" -C "\$stage"
[[ "\$(cat "\$stage/.release-sha")" == "\$release_sha" ]]
if find "\$stage" -xdev \( -name '._*' -o -name '__MACOSX' \) -print -quit | grep -q .; then
  echo 'Release metadata hygiene check failed.' >&2
  exit 1
fi

current_sha="\$(cat /opt/arbion/.release-sha)"
timestamp="\$(date -u +%Y%m%dT%H%M%SZ)"
rollback_tmp="\$(mktemp /opt/arbion/.rollback/.pre-release.XXXXXX)"
rollback="/opt/arbion/.rollback/release-pre-\${current_sha}-\${timestamp}.tar.gz"
COPYFILE_DISABLE=1 tar --exclude='./.env.production' --exclude='./.rollback' --exclude='./.incoming.*' --exclude='*.tfstate*' -czf "\$rollback_tmp" -C /opt/arbion .
mv -- "\$rollback_tmp" "\$rollback"
rollback_tmp=''

arbion_owner="\$(stat -c '%U:%G' /opt/arbion)"
rsync -a --delete --exclude='.env.production' --exclude='.rollback/' --exclude='.incoming.*' --chown="\$arbion_owner" "\$stage"/ /opt/arbion/
[[ "\$(cat /opt/arbion/.release-sha)" == "\$release_sha" ]]
[[ -r /opt/arbion/.env.production ]]

cd /opt/arbion
env ARBION_PRODUCTION_ENV_FILE=/opt/arbion/.env.production ./scripts/deploy-production.sh
./scripts/smoke-production.sh
./scripts/check-production-containers.sh
unlink "\$archive"
printf 'DEPLOYED_RELEASE=%s\\n' "\$(cat /opt/arbion/.release-sha)"
REMOTE

echo "Lightsail deployment completed for $release_sha."
