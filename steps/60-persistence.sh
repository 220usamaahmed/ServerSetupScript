#!/usr/bin/env bash
# sss-name: Keep sessions alive after disconnect
# sss-description: logind KillUserProcesses=no, user lingering, sshd keepalives; so zellij survives a dropped SSH connection
# sss-default: on
# sss-needs-root: true

if [ ! -d /run/systemd/system ]; then not_applicable "systemd is not running here, nothing to configure"; fi
me=$(id -un)
changed=0

write_if_different() { # file, content on stdin
  cat > "$tmpdir/new"
  if ! cmp -s "$tmpdir/new" "$1" 2>/dev/null; then
    as_root mkdir -p "$(dirname "$1")"
    as_root install -m 0644 "$tmpdir/new" "$1"
    changed=1
  fi
}

# 1. Do not kill user processes when a login session ends.
write_if_different /etc/systemd/logind.conf.d/sss-persist.conf <<'CONF'
[Login]
KillUserProcesses=no
CONF

# 2. Keep the user manager running with no active login.
if [ "$(loginctl show-user "$me" -p Linger --value 2>/dev/null)" != yes ]; then
  as_root loginctl enable-linger "$me"; changed=1
fi

# 3. Drop dead SSH clients after ~3 minutes instead of holding them forever.
if [ -d /etc/ssh/sshd_config.d ] && grep -Eqs '^[[:space:]]*Include[[:space:]]+/etc/ssh/sshd_config\.d/' /etc/ssh/sshd_config; then
  write_if_different /etc/ssh/sshd_config.d/sss-keepalive.conf <<'CONF'
ClientAliveInterval 30
ClientAliveCountMax 6
CONF
else
  log "note: sshd_config has no Include for sshd_config.d; skipped the keepalive settings"
fi

if [ "$changed" = 1 ]; then
  as_root systemctl kill -s HUP systemd-logind 2>/dev/null || true   # reload logind.conf
  if have sshd || [ -x /usr/sbin/sshd ]; then
    as_root /usr/sbin/sshd -t || die "sshd config test failed; leaving sshd untouched"
    as_root systemctl reload ssh 2>/dev/null || as_root systemctl reload sshd 2>/dev/null || true
  fi
  sleep 1
fi

# Verify the effective state.
linger=$(loginctl show-user "$me" -p Linger --value 2>/dev/null || true)
kill_procs=$(busctl get-property org.freedesktop.login1 /org/freedesktop/login1 org.freedesktop.login1.Manager KillUserProcesses 2>/dev/null | awk '{print $2}' || true)
log "verify: Linger=$linger  KillUserProcesses=${kill_procs:-unknown}"
[ "$linger" = yes ] || die "lingering is not enabled for $me"
[ "$kill_procs" = false ] || die "logind still reports KillUserProcesses=$kill_procs (expected false)"
if [ "$changed" = 0 ]; then already_done "already configured and verified"; fi
log "configured. Reconnect once for it to apply to new sessions."
