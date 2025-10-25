#!/bin/bash
set -e

# ====== CONFIGURATION ======
APP_NAME="beembridge"
VERSION="0.1.0-alpha.1"
BUILD_DIR="build/bin"
APPDIR="${BUILD_DIR}/${APP_NAME}.AppDir"
APPIMAGE_TOOL="appimagetool-x86_64.AppImage"
ICON_PATH="frontend/public/icon.ico"
# ===========================

echo "🔧 Building ${APP_NAME} for Linux (x86_64)..."
wails build -platform linux/amd64

echo "📁 Preparing AppDir structure in ${APPDIR}..."
rm -rf "${APPDIR}"
mkdir -p "${APPDIR}/usr/bin"

# Copy built binary
cp "${BUILD_DIR}/${APP_NAME}" "${APPDIR}/usr/bin/${APP_NAME}"

# Copy icon
if [ -f "${ICON_PATH}" ]; then
  cp "${ICON_PATH}" "${APPDIR}/${APP_NAME}.png"
else
  echo "⚠️ Warning: Icon not found at ${ICON_PATH}"
fi

# Create AppRun launcher
cat > "${APPDIR}/AppRun" <<EOF
#!/bin/bash
exec "\$APPDIR/usr/bin/${APP_NAME}" "\$@"
EOF
chmod +x "${APPDIR}/AppRun"

# Create .desktop entry
cat > "${APPDIR}/${APP_NAME}.desktop" <<EOF
[Desktop Entry]
Name=${APP_NAME}
Exec=${APP_NAME}
Icon=${APP_NAME}
Type=Application
Categories=Utility;
Comment=A Wails-built application
EOF

# Build the AppImage
echo "📦 Creating AppImage..."
${APPIMAGE_TOOL} "${APPDIR}" "${BUILD_DIR}/${APP_NAME}-${VERSION}-x86_64.AppImage"

echo "✅ Build complete!"
echo "Artifacts created in:"
echo "  - ${BUILD_DIR}/${APP_NAME}-${VERSION}-x86_64.AppImage"
echo "  - ${APPDIR}/ (temporary build directory)"

rm -rf "${APPDIR}" 
