#!/usr/bin/env bash
set -euo pipefail

GITHUB_REPO="jakesmd/hobnob"
INSTALL_DIR="${HOBNOB_INSTALL_DIR:-$HOME/.local/bin}"

_detect_os() {
    case "$(uname -s)" in
        Darwin) echo "darwin" ;;
        Linux)  echo "linux"  ;;
        *) echo "error: unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
}

_detect_arch() {
    case "$(uname -m)" in
        x86_64)         echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *) echo "error: unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac
}

_detect_shell() {
    basename "${SHELL:-bash}"
}

_in_source_repo() {
    local dir="$PWD"
    while [[ "$dir" != "/" ]]; do
        if grep -q '^module hobnob$' "$dir/go.mod" 2>/dev/null && [[ -f "$dir/cmd/hobnob/main.go" ]]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    return 1
}

_build_from_source() {
    local root="$1"
    if ! command -v go &>/dev/null; then
        echo "error: go not found on PATH — cannot build from source" >&2
        exit 1
    fi
    echo "Building hobnob from source ($(go version | awk '{print $3}'))..."
    go build -o "$INSTALL_DIR/hobnob" "$root/cmd/hobnob"
    echo "Installed: $INSTALL_DIR/hobnob"
}

_download() {
    local os arch url tmp
    os="$(_detect_os)"
    arch="$(_detect_arch)"
    if [[ "$os" == "darwin" && "$arch" == "amd64" ]]; then
        echo "error: macOS amd64 is not supported; arm64 only" >&2
        exit 1
    fi
    url="https://github.com/${GITHUB_REPO}/releases/latest/download/hobnob_${os}_${arch}.tar.gz"
    tmp="$(mktemp -d)"

    echo "Downloading hobnob (${os}/${arch})..."
    curl -fsSL "$url" | tar -xz -C "$tmp"
    install -m755 "$tmp/hobnob" "$INSTALL_DIR/hobnob"
    rm -rf "$tmp"
    echo "Installed: $INSTALL_DIR/hobnob"
}

_ensure_hobnob_block_in_rc() {
    local rc="$1"
    local comp_line="$2"
    # Stable marker ensures PATH always precedes completion and re-runs are idempotent.
    local marker="# hobnob: path+completion"
    if grep -qF "$marker" "$rc" 2>/dev/null; then
        return
    fi
    printf '\n%s\nexport PATH="$HOME/.local/bin:$PATH"\n%s\n' "$marker" "$comp_line" >> "$rc"
    echo "PATH and completion configured in $rc"
}

_setup_shell() {
    local shell="$1"
    case "$shell" in
        zsh)
            _ensure_hobnob_block_in_rc "$HOME/.zshrc" 'type compdef &>/dev/null || { autoload -Uz compinit && compinit; }; eval "$(hobnob completion zsh)"'
            echo "Reload: source ~/.zshrc"
            ;;
        bash)
            local rc
            if [[ "$(uname -s)" == "Darwin" ]]; then
                rc="$HOME/.bash_profile"
            else
                rc="$HOME/.bashrc"
            fi
            _ensure_hobnob_block_in_rc "$rc" 'eval "$(hobnob completion bash)"'
            echo "Reload: source $rc"
            ;;
        fish)
            mkdir -p "$HOME/.config/fish"
            local rc="$HOME/.config/fish/config.fish"
            if ! grep -qF "fish_add_path" "$rc" 2>/dev/null || ! grep -qF "local/bin" "$rc" 2>/dev/null; then
                printf '\n# hobnob\nfish_add_path "$HOME/.local/bin"\n' >> "$rc"
                echo "Added ~/.local/bin to fish PATH in $rc"
            fi
            if ! grep -qF "hobnob completion fish" "$rc" 2>/dev/null; then
                printf '\n# hobnob completion\nhobnob completion fish | source\n' >> "$rc"
                echo "Completion configured in $rc"
            fi
            echo "Reload: source $rc"
            ;;
        *)
            echo "Unknown shell: $shell. Add $INSTALL_DIR to PATH and run 'hobnob completion <shell>' manually."
            ;;
    esac
}

main() {
    mkdir -p "$INSTALL_DIR"

    local src_root
    if src_root="$(_in_source_repo)"; then
        _build_from_source "$src_root"
    else
        if ! command -v curl &>/dev/null; then
            echo "error: curl not found on PATH" >&2
            exit 1
        fi
        _download
    fi

    echo ""
    echo "$("$INSTALL_DIR/hobnob" --version) installed."

    local shell
    shell="$(_detect_shell)"
    _setup_shell "$shell"

    echo ""
    echo "Done. hobnob installed to $INSTALL_DIR/hobnob"
}

main
