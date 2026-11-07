#!/usr/bin/env bash
# sss-name: Install zellij
# sss-description: Terminal multiplexer, prebuilt binary into ~/.local/bin (pin: SSS_ZELLIJ_VERSION)
# sss-default: on
# sss-requires: base-packages

need_cmds curl tar
ensure_bin_dir
tag=$(resolve_tag ZELLIJ zellij-org/zellij)
want="${tag#v}"
if have zellij && [ "$(zellij --version | awk '{print $2}')" = "$want" ]; then
  already_done "zellij $want already installed"
fi
download "https://github.com/zellij-org/zellij/releases/download/$tag/zellij-$SSS_ARCH-unknown-linux-musl.tar.gz" "$tmpdir/z.tgz"
tar -xzf "$tmpdir/z.tgz" -C "$tmpdir" zellij
install -m 0755 "$tmpdir/zellij" "$SSS_BIN/zellij"
log "installed $("$SSS_BIN/zellij" --version)"
