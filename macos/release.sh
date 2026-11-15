#!/bin/sh
# Builds the release of "phoneset Camera.app": archive, sign with a Developer ID certificate, notarize with Apple,
# staple the ticket, and pack it as phoneset-camera-macos.zip (+ .sha256) in the output folder. This is the file
# `phoneset camera install` downloads from a release, so it has to be the signed and notarized one: macOS refuses
# to run a camera extension that is not (Developer ID signed apps are the ones that can ship outside the App Store).
#
# usage: macos/release.sh [--version X.Y.Z] [--out DIR] [--no-notarize] [--upload TAG]
#
# What it needs (only you can create these, see macos/README.md, "Releasing"):
#   - a "Developer ID Application" certificate with its private key in a keychain (Xcode, Settings, Accounts,
#     Manage Certificates, +, Developer ID Application),
#   - notarization credentials, one of:
#       NOTARY_PROFILE                       a profile made once with `xcrun notarytool store-credentials`, or
#       ASC_KEY_PATH, ASC_KEY_ID, ASC_ISSUER_ID   an App Store Connect API key (.p8) with its two ids.
#     With the ASC_* variables xcodebuild also uses the key to fetch the provisioning profile; with NOTARY_PROFILE it
#     is the Apple ID signed in to Xcode that does.
#
# --no-notarize signs only (to check the certificate and the profile work before the first real release).
# --upload TAG puts the zip on the existing GitHub release TAG (needs `gh auth login`) and adds its line to the
#   release's SHA256SUMS, which the program checks the download against. This is the way to release without any
#   certificate or key in GitHub: sign here, on your Mac, after the tag's release exists.
set -eu
cd "$(dirname "$0")"

VERSION="${VERSION:-}"
OUT="release"
NOTARIZE=1
UPLOAD=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --no-notarize) NOTARIZE=0; shift ;;
    --upload) UPLOAD="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

# A release tag looks like v0.2.0; the app wants 0.2.0.
VERSION="${VERSION#v}"
[ -n "$VERSION" ] || VERSION="$(sed -n 's/^ *MARKETING_VERSION: *"\(.*\)"/\1/p' project.yml | head -1)"

if [ -n "$UPLOAD" ]; then
  [ "$NOTARIZE" = 1 ] || { echo "--upload needs the notarized build: drop --no-notarize." >&2; exit 2; }
  command -v gh > /dev/null || { echo "--upload needs the GitHub CLI (gh), signed in: gh auth login" >&2; exit 1; }
  # Fail now, not after the half hour of building, if the release isn't there.
  gh release view "$UPLOAD" > /dev/null || { echo "there is no GitHub release $UPLOAD (push the tag and wait for the release job first)" >&2; exit 1; }
fi

# Credentials for xcodebuild and notarytool, by what is set.
XCODE_AUTH=""
NOTARY_AUTH=""
if [ -n "${ASC_KEY_PATH:-}" ]; then
  : "${ASC_KEY_ID:?ASC_KEY_ID is needed with ASC_KEY_PATH}" "${ASC_ISSUER_ID:?ASC_ISSUER_ID is needed with ASC_KEY_PATH}"
  XCODE_AUTH="-authenticationKeyPath $ASC_KEY_PATH -authenticationKeyID $ASC_KEY_ID -authenticationKeyIssuerID $ASC_ISSUER_ID"
  NOTARY_AUTH="--key $ASC_KEY_PATH --key-id $ASC_KEY_ID --issuer $ASC_ISSUER_ID"
elif [ -n "${NOTARY_PROFILE:-}" ]; then
  NOTARY_AUTH="--keychain-profile $NOTARY_PROFILE"
elif [ "$NOTARIZE" = 1 ]; then
  echo "No notarization credentials: set NOTARY_PROFILE, or ASC_KEY_PATH, ASC_KEY_ID and ASC_ISSUER_ID." >&2
  echo "(--no-notarize signs without notarizing.)" >&2
  exit 1
fi

if ! security find-identity -v -p codesigning | grep -q "Developer ID Application"; then
  echo "No \"Developer ID Application\" certificate in the keychains. Create one in Xcode (Settings, Accounts," >&2
  echo "Manage Certificates), or import it; see macos/README.md, \"Releasing\"." >&2
  exit 1
fi

BUILD="build/release"
rm -rf "$BUILD"
mkdir -p "$BUILD" "$OUT"
ARCHIVE="$BUILD/PhonesetCamera.xcarchive"
EXPORT="$BUILD/export"
APP="$EXPORT/phoneset Camera.app"

echo "==> version $VERSION"
xcodegen generate

# The build number is the time: macOS replaces an installed extension only with a newer build, and a time
# is newer than anything built before, by hand (build.sh) or by a release.
echo "==> archive"
# shellcheck disable=SC2086
xcodebuild -project PhonesetCamera.xcodeproj -scheme PhonesetCamera -configuration Release \
  -archivePath "$ARCHIVE" -allowProvisioningUpdates $XCODE_AUTH \
  MARKETING_VERSION="$VERSION" CURRENT_PROJECT_VERSION="$(date +%s)" archive

# The archive is signed for development; exporting for "developer-id" signs it again for distribution.
echo "==> export (Developer ID)"
# shellcheck disable=SC2086
xcodebuild -exportArchive -archivePath "$ARCHIVE" -exportPath "$EXPORT" \
  -exportOptionsPlist ExportOptions.plist -allowProvisioningUpdates $XCODE_AUTH

echo "==> check the signature"
codesign --verify --deep --strict --verbose=2 "$APP"
SIGNER="$(codesign -dvv "$APP" 2>&1 | sed -n 's/^Authority=//p' | head -1)"
case "$SIGNER" in
  "Developer ID Application"*) echo "signed by: $SIGNER" ;;
  *) echo "signed by \"$SIGNER\", not a Developer ID Application certificate" >&2; exit 1 ;;
esac
# The restricted entitlement is what lets the app install the camera; a missing one fails only on the user's Mac.
codesign -d --entitlements - "$APP" 2>/dev/null | grep -q "com.apple.developer.system-extension.install" || {
  echo "the signed app has no com.apple.developer.system-extension.install entitlement: the provisioning profile" >&2
  echo "has no System Extension capability (developer.apple.com, Identifiers, com.rapp.phoneset.camera)" >&2
  exit 1
}

if [ "$NOTARIZE" = 1 ]; then
  echo "==> notarize (Apple checks it; this takes a few minutes)"
  SUBMIT="$BUILD/submit.zip"
  ditto -c -k --keepParent "$APP" "$SUBMIT"
  # shellcheck disable=SC2086
  RESULT="$(xcrun notarytool submit "$SUBMIT" $NOTARY_AUTH --wait --output-format json)" || true
  echo "$RESULT"
  STATUS="$(echo "$RESULT" | sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p' | head -1)"
  if [ "$STATUS" != "Accepted" ]; then
    ID="$(echo "$RESULT" | sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p' | head -1)"
    echo "notarization says \"$STATUS\"; Apple's log:" >&2
    # shellcheck disable=SC2086
    [ -z "$ID" ] || xcrun notarytool log "$ID" $NOTARY_AUTH >&2 || true
    exit 1
  fi
  echo "==> staple the ticket to the app (so it passes Gatekeeper without a network)"
  xcrun stapler staple "$APP"
  xcrun stapler validate "$APP"
  spctl --assess --type execute --verbose=2 "$APP"
fi

echo "==> pack"
ZIP="$OUT/phoneset-camera-macos.zip"
rm -f "$ZIP" "$ZIP.sha256"
# ditto keeps what a signed app needs (permissions, extended attributes, the signature) that zip may drop.
ditto -c -k --keepParent "$APP" "$ZIP"
(cd "$OUT" && shasum -a 256 phoneset-camera-macos.zip > phoneset-camera-macos.zip.sha256)
echo "done: $ZIP"
cat "$ZIP.sha256"
[ "$NOTARIZE" = 1 ] || echo "NOT notarized (--no-notarize): do not publish this file."

if [ -n "$UPLOAD" ]; then
  echo "==> upload to release $UPLOAD"
  SUMS="$BUILD/sums"
  rm -rf "$SUMS"; mkdir -p "$SUMS"
  gh release upload "$UPLOAD" "$ZIP" --clobber
  # The release's SHA256SUMS gets this file's line (an older line of it goes), the rest stays as it is.
  gh release download "$UPLOAD" --pattern SHA256SUMS --dir "$SUMS" --clobber 2> /dev/null || : > "$SUMS/SHA256SUMS"
  { grep -v "  phoneset-camera-macos.zip$" "$SUMS/SHA256SUMS" || true; cat "$ZIP.sha256"; } > "$SUMS/new"
  mv "$SUMS/new" "$SUMS/SHA256SUMS"
  gh release upload "$UPLOAD" "$SUMS/SHA256SUMS" --clobber
  echo "uploaded; SHA256SUMS now:"
  cat "$SUMS/SHA256SUMS"
fi
