#!/usr/bin/env bash
set -euo pipefail

GITHUB_REPO="jakesmd/hobnob"
INSTALL_DIR="${HOBNOB_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${HOBNOB_VERSION:-latest}" # stamped at release time

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

_resolve_version() {
    if [[ "$VERSION" == "latest" ]]; then
        curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
            | grep '"tag_name"' | sed 's/.*: "//;s/".*//'
    else
        echo "$VERSION"
    fi
}

_download() {
    local os arch tag url tmp
    os="$(_detect_os)"
    arch="$(_detect_arch)"
    if [[ "$os" == "darwin" && "$arch" == "amd64" ]]; then
        echo "error: macOS amd64 is not supported; arm64 only" >&2
        exit 1
    fi
    tag="$(_resolve_version)"
    url="https://github.com/${GITHUB_REPO}/releases/download/${tag}/hobnob_${os}_${arch}.tar.gz"
    tmp="$(mktemp -d)"

    echo "Downloading hobnob ${tag} (${os}/${arch})..."
    curl -fsSL "$url" | tar -xz -C "$tmp"
    install -m755 "$tmp/hobnob" "$INSTALL_DIR/hobnob"
    rm -rf "$tmp"
    echo "Installed: $INSTALL_DIR/hobnob"
}

# Writes body between the hobnob markers in rc. One well-formed block is
# replaced in place, none means append, anything else leaves rc untouched.
# Blocks written by installers before the markers are removed on the way.
_write_hobnob_block() {
    local rc="$1"
    local body="$2"
    local tmp status=0
    touch "$rc"
    tmp="$(mktemp)"
    BODY="$body" awk '
        function bare(s) { sub(/\r$/, "", s); return s }
        function legacy_len(i,   b, n1, n2) {
            b = bare(line[i]); n1 = bare(line[i + 1]); n2 = bare(line[i + 2])
            if (b == "# hobnob: path+completion" && n1 == "export PATH=\"$HOME/.local/bin:$PATH\"" &&
                (n2 == "type compdef &>/dev/null || { autoload -Uz compinit && compinit; }; eval \"$(hobnob completion zsh)\"" ||
                 n2 == "eval \"$(hobnob completion bash)\"")) return 3
            if (b == "# hobnob" && n1 == "fish_add_path \"$HOME/.local/bin\"") return 2
            if (b == "# hobnob completion" && n1 == "hobnob completion fish | source") return 2
            return 0
        }
        {
            line[NR] = $0
            if (bare($0) == "### hobnob ###")     { begins++; first = NR }
            if (bare($0) == "### end hobnob ###") { ends++; last = NR }
        }
        END {
            if (begins > 1 || ends > 1 || begins != ends || first > last) exit 3
            for (i = 1; i <= NR; i++) {
                if (i == first) {
                    out[++n] = "### hobnob ###"; out[++n] = ENVIRON["BODY"]; out[++n] = "### end hobnob ###"
                    i = last
                    continue
                }
                skip = legacy_len(i)
                if (skip) {
                    if (n > 0 && bare(out[n]) == "") n--
                    i += skip - 1
                    continue
                }
                out[++n] = line[i]
            }
            if (!begins) {
                if (n > 0 && bare(out[n]) != "") out[++n] = ""
                out[++n] = "### hobnob ###"; out[++n] = ENVIRON["BODY"]; out[++n] = "### end hobnob ###"
            }
            for (i = 1; i <= n; i++) print out[i]
        }
    ' "$rc" > "$tmp" || status=$?

    if [[ $status -eq 3 ]]; then
        rm -f "$tmp"
        echo "warning: $rc has unpaired '### hobnob ###' markers; left it unchanged. Add these lines yourself:" >&2
        printf '%s\n' "$body" >&2
        return
    elif [[ $status -ne 0 ]]; then
        rm -f "$tmp"
        echo "error: could not read $rc" >&2
        exit 1
    fi

    if cmp -s "$tmp" "$rc"; then
        rm -f "$tmp"
        echo "PATH and completion already configured in $rc"
        return
    fi

    # cat rather than mv so a symlinked rc keeps its link and permissions.
    local backup
    backup="$(mktemp)"
    cp "$rc" "$backup"
    if ! cat "$tmp" > "$rc"; then
        rm -f "$tmp"
        echo "error: could not write $rc; its previous contents are saved in $backup" >&2
        exit 1
    fi
    rm -f "$tmp" "$backup"
    echo "PATH and completion configured in $rc"
}

_setup_shell() {
    local shell="$1"
    # Written as $HOME/... when under home so the rc line survives a renamed home.
    local dir="$INSTALL_DIR"
    case "$dir" in "$HOME"/*) dir="\$HOME${dir#"$HOME"}" ;; esac
    case "$shell" in
        zsh)
            _write_hobnob_block "$HOME/.zshrc" "export PATH=\"$dir:\$PATH\"
type compdef &>/dev/null || { autoload -Uz compinit && compinit; }
eval \"\$(hobnob completion zsh)\""
            echo "Reload: source ~/.zshrc"
            ;;
        bash)
            local rc
            if [[ "$(uname -s)" == "Darwin" ]]; then
                rc="$HOME/.bash_profile"
            else
                rc="$HOME/.bashrc"
            fi
            _write_hobnob_block "$rc" "export PATH=\"$dir:\$PATH\"
eval \"\$(hobnob completion bash)\""
            echo "Reload: source $rc"
            ;;
        fish)
            mkdir -p "$HOME/.config/fish"
            local rc="$HOME/.config/fish/config.fish"
            _write_hobnob_block "$rc" "fish_add_path \"$dir\"
hobnob completion fish | source"
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

    if [[ "${CI:-}" != "true" ]]; then
        local shell
        shell="$(_detect_shell)"
        _setup_shell "$shell"
    fi

    echo ""
    echo "Done. hobnob installed to $INSTALL_DIR/hobnob"
}

# Tests source this file with HOBNOB_INSTALL_SOURCE_ONLY=1 to reach the helpers.
[[ -n "${HOBNOB_INSTALL_SOURCE_ONLY:-}" ]] || main
