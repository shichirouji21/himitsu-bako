# Himitsu Bako

[![Version](https://img.shields.io/badge/version-1.1.0-blue)](https://github.com/shichirouji21/himitsu-bako/releases/tag/v1.1.0)
[![AUR](https://img.shields.io/badge/AUR-himitsu--bako--bin-1793d1)](https://aur.archlinux.org/packages/himitsu-bako-bin)
[![License](https://img.shields.io/badge/license-BSD--2--Clause-green)](LICENSE)

Encrypted clipboard-backed secret storage using [`age`](https://age-encryption.org/).

`himitsu-bako` is a small terminal helper for saving and retrieving text secrets quickly. It reads text from your clipboard, stores it encrypted on disk, and copies revealed secrets back to your clipboard.

## Install

### Arch Linux

```bash
paru -S himitsu-bako-bin
```

or with another AUR helper:

```bash
yay -S himitsu-bako-bin
```

To build from source instead of using the prebuilt release binary:

```bash
paru -S himitsu-bako
```

### Nix (any supported system)

```bash
nix run github:shichirouji21/himitsu-bako
nix profile install github:shichirouji21/himitsu-bako
```

The Nix package wraps `himitsu-bako` so `fzf`, `wl-clipboard`, and `xclip` are available without separate installation.

### Go

```bash
go install github.com/shichirouji21/himitsu-bako@latest
```

`himitsu-bako` will be placed in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`). Make sure that directory is on your `PATH`.

### From source

Clone the repository and build the binary:

```bash
git clone https://github.com/shichirouji21/himitsu-bako.git
cd himitsu-bako
go build -o himitsu-bako .
./himitsu-bako --help
```

Or add an alias:

```bash
alias hm="$HOME/repos/himitsu-bako/himitsu-bako"
```

For regular use, put the alias in `~/.zshrc` or `~/.bashrc`.

On Windows, build `himitsu-bako.exe` and run it directly from PowerShell:

```powershell
go build -o himitsu-bako.exe .
.\himitsu-bako.exe --help
```

## Dependencies

| Tool | Purpose |
| --- | --- |
| Go | Builds the `himitsu-bako` binary. |
| `fzf` | Interactive secret selection for reveal, delete, and rename. Exact-name reveal does not need `fzf`. |
| Clipboard tool | Reads and writes clipboard text. |

`himitsu-bako` uses the Go age library internally. The `age` command-line tool is not required at runtime.

Supported clipboard tools:

| Platform/session | Tools |
| --- | --- |
| macOS | `pbcopy` and `pbpaste`, already included with macOS. |
| Linux Wayland | `wl-copy` and `wl-paste` from `wl-clipboard`. |
| Linux X11 | `xclip` or `xsel`. |
| Windows | PowerShell clipboard APIs, with `clip.exe` as write fallback. |

Example installs:

```bash
brew install go fzf
sudo apt install golang fzf wl-clipboard
sudo apt install golang fzf xclip
winget install GoLang.Go junegunn.fzf
```

## Usage

```bash
./himitsu-bako                  # reveal a secret with fzf and copy it to the clipboard
./himitsu-bako azure-token      # reveal exact secret name without fzf
./himitsu-bako -s               # save current clipboard as an encrypted secret
./himitsu-bako -d               # delete a secret with fzf
./himitsu-bako -r               # rename a secret with fzf
./himitsu-bako --timeout=0 foo  # reveal without auto-clearing the clipboard
```

After a reveal, `himitsu-bako` schedules a background process that clears the clipboard 30 seconds later. The cleaner only clears if the clipboard still contains the revealed secret, so anything you copy in the meantime is preserved. Override the delay with `--timeout=N` (seconds), or pass `--timeout=0` to disable auto-clear.

Save flow:

```bash
./himitsu-bako -s
```

The tool asks for a secret name, reads the current clipboard text, encrypts it, and saves it. Saving with an existing name overwrites that secret.

Reveal flow:

```bash
./himitsu-bako
```

The tool opens `fzf`, lets you pick a secret by name, and copies the decrypted value to your clipboard. If you already know the name, pass it directly:

```bash
./himitsu-bako azure-token
```

Delete flow:

```bash
./himitsu-bako -d
```

The tool opens `fzf`, lets you pick a secret by name, and deletes the encrypted file.

Rename flow:

```bash
./himitsu-bako -r
```

The tool opens `fzf`, lets you pick a secret by name, asks for the new name, and rewrites the encrypted file with the same secret value under the new name.

## Storage

By default the tool stores data in:

| Platform | Directory |
| --- | --- |
| Linux | `~/.config/himitsu-bako` |
| macOS | `~/Library/Application Support/himitsu-bako` |
| Windows | `%AppData%\himitsu-bako` |
| Any platform with `XDG_CONFIG_HOME` set | `$XDG_CONFIG_HOME/himitsu-bako` |

Override the location with `SECRET_STORE_DIR`:

```bash
SECRET_STORE_DIR=/path/to/himitsu-bako ./himitsu-bako
```

## Encryption Files

The store contains:

| Path | Purpose |
| --- | --- |
| `identity.txt` | Local private age identity. Required to decrypt secrets. |
| `recipient.txt` | Public recipient derived from `identity.txt`. Used to encrypt new or updated secrets. |
| `secrets/*.age` | Encrypted secret files. Each file contains the secret name and value after decryption. |

The first time you save a secret, `himitsu-bako` creates a local age identity. New secrets are encrypted to the recipient derived from that identity.

## Security And Backups

Keep `identity.txt` private. Anyone with `identity.txt` and your encrypted `secrets/*.age` files can decrypt your secrets.

Back up `identity.txt`. If you lose it, your saved secrets cannot be decrypted or recovered.

`recipient.txt` is not needed for decryption. `himitsu-bako` recreates it from `identity.txt` when needed for saving new or updated secrets.

## Limitations

`himitsu-bako` is text-only. It reads and writes clipboard text, not images or other binary clipboard content.

It is designed for local personal use. It does not sync secrets between machines or manage remote backups.

## Versioning

Print the current version with:

```bash
./himitsu-bako --version
```

Release notes are tracked in [CHANGELOG.md](CHANGELOG.md).

## License

`himitsu-bako` is released under the BSD 2-Clause License. See [LICENSE](LICENSE) for the full text.
