#!/usr/bin/env bash
# Build a distributable .dmg from a Wails .app bundle.
#
#   build/darwin/make-dmg.sh <path-to.app> <output.dmg> [volume-name]
#
# Uses `create-dmg` if available (brew install create-dmg) for a styled image
# with an /Applications drop target; falls back to plain `hdiutil` otherwise.
#
# Code signing / notarisation: if APPLE_DEVELOPER_ID is set, the bundle is
# signed before packaging. Notarisation (xcrun notarytool) is left to CI when
# APPLE_NOTARY_PROFILE is set, since it requires stored credentials.
set -euo pipefail

APP="${1:?usage: make-dmg.sh <app> <dmg> [volname]}"
DMG="${2:?usage: make-dmg.sh <app> <dmg> [volname]}"
VOLNAME="${3:-Personal AI Agent}"

if [ ! -d "$APP" ]; then
  echo "ERROR: app bundle not found: $APP" >&2
  exit 1
fi

# Optional code signing with a Developer ID Application certificate.
if [ -n "${APPLE_DEVELOPER_ID:-}" ]; then
  echo "→ Signing $APP with: $APPLE_DEVELOPER_ID"
  codesign --deep --force --options runtime \
    --sign "$APPLE_DEVELOPER_ID" "$APP"
fi

rm -f "$DMG"

if command -v create-dmg >/dev/null 2>&1; then
  echo "→ Building styled DMG with create-dmg"
  create-dmg \
    --volname "$VOLNAME" \
    --window-size 540 360 \
    --icon-size 100 \
    --icon "$(basename "$APP")" 140 180 \
    --app-drop-link 400 180 \
    --no-internet-enable \
    "$DMG" "$APP" || {
      # create-dmg exits non-zero if it cannot detach; verify the file exists.
      [ -f "$DMG" ] || { echo "create-dmg failed"; exit 1; }
    }
else
  echo "→ create-dmg not found; building plain DMG with hdiutil"
  staging="$(mktemp -d)"
  cp -R "$APP" "$staging/"
  ln -s /Applications "$staging/Applications"
  hdiutil create -volname "$VOLNAME" -srcfolder "$staging" \
    -ov -format UDZO "$DMG"
  rm -rf "$staging"
fi

# Optional notarisation when a stored notarytool profile is provided.
if [ -n "${APPLE_NOTARY_PROFILE:-}" ]; then
  echo "→ Notarising $DMG"
  xcrun notarytool submit "$DMG" \
    --keychain-profile "$APPLE_NOTARY_PROFILE" --wait
  xcrun stapler staple "$DMG"
fi

echo "✓ DMG → $DMG"
