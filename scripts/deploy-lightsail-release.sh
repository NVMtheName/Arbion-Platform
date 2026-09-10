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

# Extract tracked application code with canonical read/execute permissions even
# if the remote administrative shell uses a private umask. Existing secret files
# are excluded from replacement; mktemp still protects temporary artifacts.
umask 022

[[ -d /opt/arbion && -r /opt/arbion/.env.production ]]
[[ -f "\$archive" ]]
for command in cat curl date docker find grep hostname jq mktemp mv rsync sha256sum stat systemctl tar unlink; do
  command -v "\$command" >/dev/null || {
    echo "Required host command not found: \$command" >&2
    exit 1
  }
done
deployment_started_epoch="\$(date -u +%s)"
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

# A release may not replace production code until the current production
# database has completed a new encrypted off-host backup. The root-owned unit
# owns its credentials and failure notification; this deploy path never reads
# or prints them.
systemctl start arbion-postgres-backup.service
[[ "\$(systemctl show arbion-postgres-backup.service --property=Result --value)" == 'success' ]]
/opt/arbion/scripts/check-postgres-backup-freshness.sh
[[ -f /var/lib/arbion-backups/last-success && ! -L /var/lib/arbion-backups/last-success ]]
read -r backup_completed_epoch backup_object_key backup_extra </var/lib/arbion-backups/last-success
[[ "\$backup_completed_epoch" =~ ^[0-9]{1,12}$ && "\$backup_object_key" =~ ^postgres/daily/[A-Za-z0-9._-]+$ && -z "\${backup_extra:-}" ]]
[[ "\$backup_completed_epoch" -ge "\$deployment_started_epoch" && "\$backup_completed_epoch" -le "\$(date -u +%s)" ]]
printf 'PRE_DEPLOY_BACKUP=verified\n'

current_sha="\$(cat /opt/arbion/.release-sha)"
[[ "\$current_sha" =~ ^[0-9a-f]{40}$ ]]
timestamp="\$(date -u +%Y%m%dT%H%M%SZ)"
rollback_tmp="\$(mktemp /opt/arbion/.rollback/.pre-release.XXXXXX)"
rollback="/opt/arbion/.rollback/release-pre-\${current_sha}-\${timestamp}.tar.gz"
COPYFILE_DISABLE=1 tar --exclude='./.env.production' --exclude='./.rollback' --exclude='./.incoming.*' --exclude='*.tfstate*' -czf "\$rollback_tmp" -C /opt/arbion .
mv -- "\$rollback_tmp" "\$rollback"
rollback_tmp=''
rollback_sha256="\$(sha256sum "\$rollback" | awk '{print \$1}')"

arbion_owner="\$(stat -c '%U:%G' /opt/arbion)"
replacement_started_epoch="\$(date -u +%s)"
rsync -a --delete --exclude='.env.production' --exclude='.rollback/' --exclude='.incoming.*' --chown="\$arbion_owner" "\$stage"/ /opt/arbion/
replacement_completed_epoch="\$(date -u +%s)"
[[ "\$(cat /opt/arbion/.release-sha)" == "\$release_sha" ]]
[[ -r /opt/arbion/.env.production ]]

cd /opt/arbion
env ARBION_PRODUCTION_ENV_FILE=/opt/arbion/.env.production ./scripts/deploy-production.sh
./scripts/smoke-production.sh
./scripts/check-production-containers.sh
unlink "\$archive"
printf 'DEPLOYED_RELEASE=%s\\n' "\$(cat /opt/arbion/.release-sha)"
printf 'PREVIOUS_RELEASE=%s\\n' "\$current_sha"
printf 'ROLLBACK_ARCHIVE=%s\\n' "\$rollback"
# Forward-only, credential-free receipt. This is an observed deployment record,
# not an approval, signature, restore proof, or independent audit conclusion.
receipt="\$(jq -cn --arg host "\$(hostname)" --arg release "\$release_sha" --arg previous "\$current_sha" \
  --arg archive "\$expected_sha" --arg rollback "\$rollback" --arg rollback_sha "\$rollback_sha256" \
  --arg key "\$backup_object_key" --argjson started "\$deployment_started_epoch" \
  --argjson backup "\$backup_completed_epoch" --argjson replace_start "\$replacement_started_epoch" \
  --argjson replace_end "\$replacement_completed_epoch" --argjson completed "\$(date -u +%s)" \
  '{schema_version:"1.0",source:"LIGHTSAIL_DEPLOY_SCRIPT",host:\$host,release_sha:\$release,previous_release_sha:\$previous,archive_sha256:\$archive,started_epoch:\$started,backup:{completed_epoch:\$backup,object_key:\$key},replacement_started_epoch:\$replace_start,replacement_completed_epoch:\$replace_end,completed_epoch:\$completed,rollback:{path:\$rollback,sha256:\$rollback_sha},checks:{readiness:"PASS",public_smoke:"PASS",containers:"PASS"}}')"
printf 'ARBION_DEPLOYMENT_EVIDENCE_JSON=%s\\n' "\$receipt"
REMOTE

echo "Lightsail deployment completed for $release_sha."
