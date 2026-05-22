# Changelog

## 1.0.0

* First release.
* Save the current clipboard text as an encrypted secret with a chosen name (`-s`, `--save`).
* Reveal a saved secret to the clipboard via `fzf` (no argument) or by exact name (`himitsu-bako name`).
* Remove a saved secret via `fzf` (`-r`, `--remove`).
* Encrypt secrets with `filippo.io/age` using a locally generated X25519 identity stored at `identity.txt`.
* Store secrets under the platform's user config directory by default; override with `SECRET_STORE_DIR`.
* Cross-platform clipboard support: `pbcopy`/`pbpaste` on macOS, `wl-copy`/`wl-paste` on Linux Wayland, `xclip` or `xsel` on Linux X11, PowerShell or `clip.exe` on Windows.
* Auto-clear the clipboard 30 seconds after a reveal; configurable with `--timeout=N`, disable with `--timeout=0`. The cleaner only clears if the clipboard still contains the revealed secret.
* Sort the `fzf` picker alphabetically by secret name.
* Print the version with `--version` (`-v`).
