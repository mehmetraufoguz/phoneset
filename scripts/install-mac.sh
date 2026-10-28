#!/bin/sh
# Installs phoneset on macOS: downloads the latest release, puts `phoneset` in
# ~/.local/bin and puts that folder on your PATH.
#
#   curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh
#
# While the repository is private that URL answers 404; the same script runs through the GitHub CLI:
#
#   gh api repos/mehmetraufoguz/phoneset/contents/scripts/install-mac.sh -H "Accept: application/vnd.github.raw" | sh
#
# Options go after `sh -s --`, e.g. `... | sh -s -- --start --autostart`:
#   --start            start phoneset in the background now
#   --autostart        start it in the background at every login
#   --with-blackhole   also install BlackHole 2ch with Homebrew (the virtual microphone)
#   --version vX.Y.Z   install that release instead of the latest
#   --no-modify-path   don't touch ~/.zprofile
#
# Environment: PHONESET_BIN_DIR (default ~/.local/bin), PHONESET_TARBALL (install from a local file).

set -eu

REPO="mehmetraufoguz/phoneset"
BIN_DIR="${PHONESET_BIN_DIR:-$HOME/.local/bin}"
START=0 AUTOSTART=0 BLACKHOLE=0 MODIFY_PATH=1 VERSION=""

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
	--start) START=1 ;;
	--autostart) AUTOSTART=1 ;;
	--with-blackhole) BLACKHOLE=1 ;;
	--no-modify-path) MODIFY_PATH=0 ;;
	--version) shift; [ $# -gt 0 ] || die "--version needs a value"; VERSION="$1" ;;
	-h | --help) sed -n '2,26p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) die "unknown option: $1" ;;
	esac
	shift
done

[ "$(uname -s)" = "Darwin" ] || die "this installer is for macOS. On Windows download phoneset.exe from the releases page."
case "$(uname -m)" in
arm64) ARCH=arm64 ;;
x86_64) ARCH=amd64 ;;
*) die "unsupported processor: $(uname -m)" ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
ASSET="phoneset-macos-$ARCH.tar.gz"

# 1. Get the archive.
if [ -n "${PHONESET_TARBALL:-}" ]; then
	say "Using $PHONESET_TARBALL"
	cp "$PHONESET_TARBALL" "$TMP/$ASSET"
elif command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
	say "Downloading phoneset ${VERSION:-(latest)} for macOS $ARCH with gh..."
	if [ -n "$VERSION" ]; then
		gh release download "$VERSION" --repo "$REPO" --pattern "$ASSET" --dir "$TMP"
	else
		gh release download --repo "$REPO" --pattern "$ASSET" --dir "$TMP"
	fi
else
	say "Downloading phoneset ${VERSION:-(latest)} for macOS $ARCH..."
	if [ -n "$VERSION" ]; then
		URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
	else
		URL="https://github.com/$REPO/releases/latest/download/$ASSET"
	fi
	curl -fsSL "$URL" -o "$TMP/$ASSET" ||
		die "couldn't download $URL. The repository is private: install the GitHub CLI (brew install gh), run 'gh auth login' and try again."
fi

# 2. If it is running, stop it so the file can be replaced, and bring it back afterwards.
WAS_RUNNING=0
if [ -x "$BIN_DIR/phoneset" ] && "$BIN_DIR/phoneset" status --json 2>/dev/null | grep -q '"running": true'; then
	WAS_RUNNING=1
	say "Stopping the running phoneset..."
	"$BIN_DIR/phoneset" stop >/dev/null 2>&1 || true
fi

# 3. Install.
tar -xzf "$TMP/$ASSET" -C "$TMP" phoneset || die "the archive doesn't contain a phoneset binary"
mkdir -p "$BIN_DIR"
install -m 755 "$TMP/phoneset" "$BIN_DIR/phoneset"
xattr -d com.apple.quarantine "$BIN_DIR/phoneset" 2>/dev/null || true
say "Installed $("$BIN_DIR/phoneset" version) to $BIN_DIR/phoneset"

# 4. PATH.
case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*)
	if [ "$MODIFY_PATH" -eq 1 ]; then
		LINE="export PATH=\"$BIN_DIR:\$PATH\"  # phoneset"
		if ! grep -qsF "# phoneset" "$HOME/.zprofile"; then
			printf '\n%s\n' "$LINE" >>"$HOME/.zprofile"
			say "Added $BIN_DIR to your PATH in ~/.zprofile (open a new terminal window to pick it up)."
		fi
	else
		say "Note: $BIN_DIR is not on your PATH."
	fi
	PATH="$BIN_DIR:$PATH"
	;;
esac

# 5. Optional extras.
if [ "$BLACKHOLE" -eq 1 ]; then
	if command -v brew >/dev/null 2>&1; then
		say "Installing BlackHole 2ch (macOS will ask for your password)..."
		brew install blackhole-2ch || say "BlackHole didn't install; see https://existential.audio/blackhole/"
	else
		say "Homebrew isn't installed. Get BlackHole 2ch from https://existential.audio/blackhole/"
	fi
fi
[ "$AUTOSTART" -eq 1 ] && "$BIN_DIR/phoneset" autostart enable
if [ "$START" -eq 1 ] || [ "$WAS_RUNNING" -eq 1 ]; then
	"$BIN_DIR/phoneset" daemon
fi

say ""
say "Next:"
[ "$START" -eq 1 ] || [ "$WAS_RUNNING" -eq 1 ] || say "  phoneset daemon              run it in the background (tray icon, pairing page)"
say "  phoneset                     or run it in this terminal"
say "  phoneset autostart enable    start it at every login"
if ! command -v brew >/dev/null 2>&1 || [ "$BLACKHOLE" -eq 0 ]; then
	say "  The phone's microphone needs the BlackHole 2ch virtual device: brew install blackhole-2ch"
fi
