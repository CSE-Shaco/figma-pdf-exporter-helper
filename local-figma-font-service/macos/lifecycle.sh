#!/bin/bash
# Sourced only by this package's double-click entry points. Uses macOS tools.
set -euo pipefail
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
font_label=dev.figmapdf.local-font-service
font_domain="gui/$(id -u)"
font_dir="$HOME/Library/Application Support/FigmaPDFExporter/FontService"
font_bin="$font_dir/figma-local-font-service"
font_owner="$font_dir/owner.sha256"
font_plist="$HOME/Library/LaunchAgents/$font_label.plist"
font_package=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
fail() { printf '%s\n' "$*" >&2; return 1; }
hash_file() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
check_parents() {
  local item
  for item in "$HOME/Library" "$HOME/Library/Application Support" "$HOME/Library/Application Support/FigmaPDFExporter" "$font_dir" "$HOME/Library/LaunchAgents"; do
    [ ! -L "$item" ] || fail "Refusing symbolic link: $item"
    if [ -e "$item" ]; then [ -d "$item" ] && [ -O "$item" ] || fail "Directory is not owned by this user: $item"; fi
  done
}
check_owned() {
  if [ -e "$font_dir/service.stderr.log" ] || [ -L "$font_dir/service.stderr.log" ]; then
    [ -f "$font_dir/service.stderr.log" ] && [ ! -L "$font_dir/service.stderr.log" ] && [ -O "$font_dir/service.stderr.log" ] || fail 'Unsafe existing service log.'
  fi
  [ -f "$font_bin" ] && [ ! -L "$font_bin" ] && [ -O "$font_bin" ] || fail 'Installed binary ownership check failed.'
  [ -f "$font_owner" ] && [ ! -L "$font_owner" ] && [ -O "$font_owner" ] || fail 'Installation marker is missing or unsafe.'
  [ "$(cat "$font_owner")" = "$(hash_file "$font_bin")" ] || fail 'Installed binary does not match its ownership marker.'
  [ -f "$font_plist" ] && [ ! -L "$font_plist" ] && [ -O "$font_plist" ] || fail 'Installed launch agent ownership check failed.'
  [ "$(/usr/libexec/PlistBuddy -c 'Print :Label' "$font_plist")" = "$font_label" ] || fail 'Unexpected launch agent label.'
  [ "$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:0' "$font_plist")" = "$font_bin" ] || fail 'Unexpected launch agent executable.'
  ! /usr/libexec/PlistBuddy -c 'Print :ProgramArguments:1' "$font_plist" >/dev/null 2>&1 || fail 'Unexpected launch agent arguments.'
}
job_loaded() { launchctl print "$font_domain/$font_label" >/dev/null 2>&1; }
check_loaded_owner() {
  # launchctl print's program path is a separate complete line, not a substring match.
  launchctl print "$font_domain/$font_label" | /usr/bin/awk -v expected="$font_bin" '
    /^[[:space:]]*program = / { sub(/^[[:space:]]*program = /, ""); if ($0 == expected) ok=1 }
    END { exit !ok }' || fail 'A different development/legacy job uses this label. Remove that job explicitly before installing.'
}
font_install() (
  check_parents
  font_existed=0; font_loaded=0; font_locked=0; font_transaction=0; font_stage=''
  font_old_bin=0; font_old_marker=0; font_old_plist=0
  font_new_bin=0; font_new_marker=0; font_new_plist=0
  font_lock="$HOME/Library/Application Support/FigmaPDFExporter/.font-install.lock"
  font_finish() {
    font_status=$?
    trap - EXIT
    set +e
    font_restore_failed=0
    if [ "$font_transaction" = 1 ]; then
      if job_loaded; then
        if check_loaded_owner; then launchctl bootout "$font_domain/$font_label" || font_restore_failed=1
        else font_restore_failed=1; fi
      fi
      if [ "$font_new_bin" = 1 ]; then rm -f "$font_bin" || font_restore_failed=1; fi
      if [ "$font_new_marker" = 1 ]; then rm -f "$font_owner" || font_restore_failed=1; fi
      if [ "$font_new_plist" = 1 ]; then rm -f "$font_plist" || font_restore_failed=1; fi
      # Restore individually: a failed move must never trigger deletion of its backup.
      if [ "$font_old_bin" = 1 ]; then mv "$font_backup/figma-local-font-service" "$font_bin" || font_restore_failed=1; fi
      if [ "$font_old_marker" = 1 ]; then mv "$font_backup/owner.sha256" "$font_owner" || font_restore_failed=1; fi
      if [ "$font_old_plist" = 1 ]; then mv "$font_backup/agent.plist" "$font_plist" || font_restore_failed=1; fi
      if [ "$font_restore_failed" = 0 ] && [ "$font_loaded" = 1 ] && ! job_loaded; then
        launchctl bootstrap "$font_domain" "$font_plist" || font_restore_failed=1
      fi
      if [ "$font_restore_failed" = 0 ]; then
        printf '%s\n' 'Installation failed; prior files and registration state were restored.' >&2
        if [ "$font_existed" = 0 ]; then rmdir "$font_dir" 2>/dev/null || true; fi
      else
        printf 'Installation failed and rollback was incomplete. Preserved backup directory: %s\n' "$font_backup" >&2
      fi
    fi
    if [ -n "$font_stage" ]; then
      # Known staging files only. Never recursively remove a backup directory.
      rm -f "$font_stage/figma-local-font-service" "$font_stage/owner.sha256" "$font_stage/new.plist"
      rmdir "$font_stage/previous" 2>/dev/null || true
      rmdir "$font_stage" 2>/dev/null || true
    fi
    if [ "$font_locked" = 1 ]; then rmdir "$font_lock" || true; fi
    if [ "$font_restore_failed" != 0 ]; then font_status=1; fi
    exit "$font_status"
  }
  trap font_finish EXIT
  mkdir -p "$HOME/Library/Application Support/FigmaPDFExporter" "$HOME/Library/LaunchAgents"
  mkdir "$font_lock" || fail 'Another install/uninstall is in progress (or a prior interrupted lock remains).'
  font_locked=1
  if [ -e "$font_dir" ] || [ -e "$font_plist" ]; then check_owned; font_existed=1; fi
  if job_loaded; then check_loaded_owner; font_loaded=1; fi
  [ -f "$font_package/figma-local-font-service" ] && [ ! -L "$font_package/figma-local-font-service" ] || fail 'Package executable is missing.'
  (cd "$font_package" && shasum -a 256 -c SHA256SUMS.txt) || fail 'Package checksum verification failed.'
  font_stage=$(mktemp -d "$HOME/Library/Application Support/FigmaPDFExporter/.font-stage.XXXXXX")
  font_backup="$font_stage/previous"
  mkdir "$font_backup"
  cp "$font_package/figma-local-font-service" "$font_stage/figma-local-font-service"
  chmod 700 "$font_stage/figma-local-font-service"
  hash_file "$font_stage/figma-local-font-service" > "$font_stage/owner.sha256"
  cp "$font_package/job.plist" "$font_stage/new.plist"
  plutil -replace ProgramArguments -array "$font_stage/new.plist"
  plutil -insert ProgramArguments -string "$font_bin" -append "$font_stage/new.plist"
  plutil -replace StandardErrorPath -string "$font_dir/service.stderr.log" "$font_stage/new.plist"
  chmod 600 "$font_stage/new.plist" "$font_stage/owner.sha256"
  plutil -lint "$font_stage/new.plist"
  mkdir -p "$font_dir"
  chmod 700 "$font_dir"
  font_transaction=1
  if [ "$font_loaded" = 1 ]; then launchctl bootout "$font_domain/$font_label"; fi
  if [ "$font_existed" = 1 ]; then
    mv "$font_bin" "$font_backup/figma-local-font-service"; font_old_bin=1
    mv "$font_owner" "$font_backup/owner.sha256"; font_old_marker=1
    mv "$font_plist" "$font_backup/agent.plist"; font_old_plist=1
  fi
  mv "$font_stage/figma-local-font-service" "$font_bin"; font_new_bin=1
  mv "$font_stage/owner.sha256" "$font_owner"; font_new_marker=1
  mv "$font_stage/new.plist" "$font_plist"; font_new_plist=1
  launchctl bootstrap "$font_domain" "$font_plist"
  font_transaction=0
  rm -f "$font_backup/figma-local-font-service" "$font_backup/owner.sha256" "$font_backup/agent.plist"
  printf '%s\n' 'Installed for this user. Figma requests start the service automatically; login registration persists after restart.'
)
font_uninstall() (
  font_lock="$HOME/Library/Application Support/FigmaPDFExporter/.font-install.lock"
  font_locked=0
  trap 'if [ "$font_locked" = 1 ]; then rmdir "$font_lock"; fi' EXIT
  check_parents
  if [ ! -e "$font_dir" ] && [ ! -e "$font_plist" ]; then printf '%s\n' 'Nothing is installed.'; return; fi
  mkdir "$font_lock" || fail 'Another install/uninstall is in progress (or a prior interrupted lock remains).'
  font_locked=1
  check_owned
  if job_loaded; then check_loaded_owner; launchctl bootout "$font_domain/$font_label"; fi
  rm -f "$font_bin" "$font_owner" "$font_plist"
  # Remove only our own regular log. Never follow a symbolic link or delete unknown files.
  if [ -f "$font_dir/service.stderr.log" ] && [ ! -L "$font_dir/service.stderr.log" ] && [ -O "$font_dir/service.stderr.log" ]; then rm -f "$font_dir/service.stderr.log"; fi
  rmdir "$font_dir" 2>/dev/null || true
  rmdir "$HOME/Library/Application Support/FigmaPDFExporter" 2>/dev/null || true
  printf '%s\n' 'Uninstalled. Unrelated files were preserved.'
)
