#!/bin/sh
# jump-cd installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/Violetylove/jump-cd/main/scripts/install.sh | sh
#
# What it does:
#   1. works out your OS and architecture
#   2. downloads the matching release archive and verifies its checksum
#   3. installs the binary into ~/.local/bin (override with JCD_INSTALL_DIR)
#   4. makes sure that directory is on PATH, editing your rc file if needed
#   5. appends the shell integration line to your rc file, exactly once
#
# Overridable through the environment:
#   JCD_VERSION=<x.y.z>     install a specific version instead of the latest
#   JCD_INSTALL_DIR=<dir>   install somewhere other than ~/.local/bin
#   JCD_BASE_URL=<url>      download from a mirror instead of GitHub Releases
#   JCD_NO_SHELL_SETUP=1    download and install only, do not touch any rc file
#
# Written in POSIX sh and printing plain ASCII on purpose: it runs in whatever
# shell and whatever locale the user happens to have, including macOS's
# ancient bash 3.2 and minimal containers.
set -eu

REPO="Violetylove/jump-cd"
APP="jump-cd"
BIN="jcd"

step() { printf '\n%s\n' "$*"; }
info() { printf '  %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --------------------------------------------------------------- preflight

step "Checking the environment"

command -v tar > /dev/null 2>&1 || die "tar is required but was not found"

if command -v curl > /dev/null 2>&1; then
    HAVE_CURL=1
elif command -v wget > /dev/null 2>&1; then
    HAVE_CURL=0
else
    die "either curl or wget is required"
fi

http_get() {
    if [ "$HAVE_CURL" = 1 ]; then
        curl -fsSL "$1"
    else
        wget -qO- "$1"
    fi
}

http_download() {
    # $1 url, $2 destination
    if [ "$HAVE_CURL" = 1 ]; then
        curl -fsSL -o "$2" "$1"
    else
        wget -qO "$2" "$1"
    fi
}

# --------------------------------------------------------------- platform

step "Detecting the platform"

case "$(uname -s)" in
    Linux)  goos=linux ;;
    Darwin) goos=darwin ;;
    *)      die "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
    x86_64|amd64)  goarch=amd64 ;;
    arm64|aarch64) goarch=arm64 ;;
    *)             die "unsupported architecture: $(uname -m)" ;;
esac

info "os:   $goos"
info "arch: $goarch"

# --------------------------------------------------------------- version

version=${JCD_VERSION:-}
if [ -z "$version" ]; then
    if [ "$HAVE_CURL" = 1 ]; then
        # Let GitHub redirect /releases/latest to the tagged URL and read the
        # final address. No API call, so no rate limit.
        latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
            "https://github.com/$REPO/releases/latest")
        version=$(printf '%s' "${latest_url##*/}" | sed 's/^v//')
    else
        version=$(http_get "https://api.github.com/repos/$REPO/releases/latest" \
            | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)
    fi
fi
[ -n "$version" ] || die "could not work out the latest version; set JCD_VERSION"
info "version: $version"

base=${JCD_BASE_URL:-https://github.com/$REPO/releases/download/v$version}
archive="${APP}_${version}_${goos}_${goarch}.tar.gz"

# --------------------------------------------------------------- download

step "Downloading"
info "$base/$archive"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

http_download "$base/$archive" "$tmp/$archive" \
    || die "download failed: $base/$archive"

# Verify against the checksums.txt the release ships. A missing checksum file
# is a warning rather than an error, so that mirrors can drop it if they must.
if http_download "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
    want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt" | head -1)
    if [ -z "$want" ]; then
        warn "no checksum listed for $archive, skipping verification"
    elif command -v sha256sum > /dev/null 2>&1; then
        got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
    elif command -v shasum > /dev/null 2>&1; then
        got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
    else
        got=""
        warn "no sha256 tool found, skipping checksum verification"
    fi
    if [ -n "$want" ] && [ -n "$got" ]; then
        [ "$got" = "$want" ] || die "checksum mismatch for $archive"
        info "checksum verified"
    fi
else
    warn "checksums.txt not available, skipping verification"
fi

tar -xzf "$tmp/$archive" -C "$tmp" || die "could not extract $archive"
[ -f "$tmp/$BIN" ] || die "the archive did not contain $BIN"

# --------------------------------------------------------------- install

step "Installing"

dir=${JCD_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dir" || die "could not create $dir"
[ -w "$dir" ] || die "$dir is not writable; set JCD_INSTALL_DIR to a directory you own"

# Write next to the target and rename, so replacing a running binary works.
cp "$tmp/$BIN" "$dir/.$BIN.new"
chmod 755 "$dir/.$BIN.new"
mv -f "$dir/.$BIN.new" "$dir/$BIN"
info "$dir/$BIN"

# --------------------------------------------------------------- PATH

step "Checking PATH"

need_path=0
case ":$PATH:" in
    *":$dir:"*) info "$dir is already on PATH" ;;
    *)
        need_path=1
        info "$dir is not on PATH yet"
        ;;
esac

# --------------------------------------------------------------- shell

line=""
rc=""
if [ "${JCD_NO_SHELL_SETUP:-}" = "1" ]; then
    step "Skipping shell setup (JCD_NO_SHELL_SETUP=1)"
else
    step "Setting up your shell"

    case "$(basename "${SHELL:-sh}")" in
        zsh)
            rc="$HOME/.zshrc"
            line='eval "$(jcd init zsh)"'
            path_line="export PATH=\"$dir:\$PATH\""
            ;;
        bash)
            rc="$HOME/.bashrc"
            line='eval "$(jcd init bash)"'
            path_line="export PATH=\"$dir:\$PATH\""
            ;;
        fish)
            rc="$HOME/.config/fish/config.fish"
            line='jcd init fish | source'
            path_line="fish_add_path $dir"
            ;;
        *)
            rc=""
            ;;
    esac

    if [ -z "$rc" ]; then
        warn "could not recognise your shell from SHELL=${SHELL:-unset}"
        warn "add this line to your shell's startup file yourself:"
        warn "    eval \"\$(jcd init <shell>)\""
    elif grep -q 'jcd init' "$rc" 2>/dev/null; then
        info "$rc already contains the integration, leaving it alone"
    else
        mkdir -p "$(dirname "$rc")"
        {
            printf '\n# jump-cd\n'
            if [ "$need_path" = 1 ]; then
                printf '%s\n' "$path_line"
            fi
            printf '%s\n' "$line"
        } >> "$rc"
        info "appended to $rc"
    fi
fi

# --------------------------------------------------------------- done

step "Done"
info "jcd $version is installed"

if [ -n "$line" ] && [ -n "$rc" ]; then
    printf '\nOpen a new shell, or run this in the current one:\n\n    %s\n' "$line"
    [ "$need_path" = 1 ] && printf '\nYou will also need PATH to include %s.\n' "$dir"
fi

printf '\nThen check everything with:\n\n    jcd doctor\n\n'
