#!/usr/bin/env bash
# Baut S-Check Desktop für Windows (x64) unter Linux oder WSL:
#   build/s-check.exe          Programm
#   build/s-check-setup.exe    Installation für den Benutzer (NSIS)
#   build/s-check.msi          Installation für alle Benutzer, Softwareverteilung (wixl)
#
# Benötigt: Go 1.24, Node 20, makensis (nsis), wixl (msitools), x86_64-w64-mingw32-windres
# (binutils-mingw-w64). Optional signieren: SIGN_PFX=<Datei.pfx> SIGN_PASS=<Kennwort> (osslsigncode).
set -euo pipefail
cd "$(dirname "$0")"
DESKTOP="$(pwd)"
VERSION="${VERSION:-$(cat VERSION)}"
IFS=. read -r V1 V2 V3 <<<"$VERSION"
OUT="$DESKTOP/build"
rm -rf "$OUT" && mkdir -p "$OUT"

echo "== Oberfläche"
(cd .. && { [ -d node_modules ] || npm ci --no-audit --no-fund; } && npm run build >/dev/null)
rm -rf web && cp -r ../dist web

echo "== Programmsymbol und Versionsangaben"
if command -v x86_64-w64-mingw32-windres >/dev/null; then
  sed -e "s/@V1@/$V1/g; s/@V2@/$V2/g; s/@V3@/$V3/g; s/@VERSION@/$VERSION/g" winres/s-check.rc.in > winres/s-check.rc
  (cd winres && x86_64-w64-mingw32-windres --preprocessor=cat -O coff -i s-check.rc -o ../rsrc_windows_amd64.syso)
  rm winres/s-check.rc
else
  echo "   windres fehlt: vorhandene rsrc_windows_amd64.syso wird verwendet"
fi

echo "== Programm"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H windowsgui -X main.version=$VERSION" -o "$OUT/s-check.exe" .
cp winres/s-check.ico "$OUT/"

sign() {
  [ -n "${SIGN_PFX:-}" ] || return 0
  osslsigncode sign -pkcs12 "$SIGN_PFX" -pass "${SIGN_PASS:-}" -n "S-Check" -h sha256 \
    -t http://timestamp.digicert.com -in "$1" -out "$1.signed" && mv "$1.signed" "$1"
}
sign "$OUT/s-check.exe"

echo "== Installation (NSIS)"
makensis -V2 -DVERSION="$VERSION" -DSRC="$OUT" -DOUT="$OUT/s-check-setup.exe" installer/s-check.nsi
sign "$OUT/s-check-setup.exe"

echo "== Installation (MSI)"
wixl -a x64 -D Version="$VERSION" -D Src="$OUT" -o "$OUT/s-check.msi" installer/s-check.wxs
# wixl ignores Secure="yes": without this SERVERURL from the command line is dropped in managed installs
msibuild "$OUT/s-check.msi" -q "UPDATE \`Property\` SET \`Value\` = 'OLDERFOUND;NEWERFOUND;SERVERURL' WHERE \`Property\` = 'SecureCustomProperties'"
sign "$OUT/s-check.msi"

rm "$OUT/s-check.ico"
(cd "$OUT" && sha256sum s-check.exe s-check-setup.exe s-check.msi > SHA256SUMS.txt)
ls -l "$OUT"
