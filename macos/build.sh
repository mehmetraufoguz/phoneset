#!/bin/sh
# Generates the Xcode project, builds phoneset Camera.app (signed with the team in project.yml, Xcode's automatic
# signing) and puts it in /Applications, where macOS installs a camera extension from. The build number is the time:
# macOS replaces an installed extension only with a newer one.
# usage: macos/build.sh [--no-install]
set -e
cd "$(dirname "$0")"
xcodegen generate
xcodebuild -project PhonesetCamera.xcodeproj -scheme PhonesetCamera -configuration Debug \
  -derivedDataPath build -allowProvisioningUpdates CURRENT_PROJECT_VERSION="$(date +%s)" build
APP="build/Build/Products/Debug/phoneset Camera.app"
codesign --verify --deep --strict "$APP" && echo "signature ok"
if [ "$1" != "--no-install" ]; then
  # An old copy is replaced as a whole: a half-updated bundle fails its signature.
  rm -rf "/Applications/phoneset Camera.app"
  ditto "$APP" "/Applications/phoneset Camera.app"
  echo "installed: /Applications/phoneset Camera.app"
fi
