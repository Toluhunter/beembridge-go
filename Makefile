# Makefile for Beembridge

# ====== CONFIGURATION ======
APP_NAME := beembridge
# Note: This version is hardcoded from build_appimage.sh.
VERSION := 0.1.0-alpha.1
BUILD_DIR := build/bin
APPDIR := $(BUILD_DIR)/$(APP_NAME).AppDir
# Assumes appimagetool is in the current directory or in PATH
APPIMAGE_TOOL := appimagetool-x86_64.AppImage
# Using the existing png icon in the build directory
ICON_PATH := build/appicon.png
FINAL_APPIMAGE := $(BUILD_DIR)/$(APP_NAME)-$(VERSION)-x86_64.AppImage

.PHONY: all run build clean

# Default target
all: build

# Run the application in development mode
run:
	@echo "🚀 Starting development server..."
	wails dev -tags webkit2_41

# Build the application and create an AppImage
build:
	@echo "🔧 Building $(APP_NAME) for Linux (x86_64)..."
	wails build -platform linux/amd64

	@echo "📁 Preparing AppDir structure in $(APPDIR)..."
	rm -rf $(APPDIR)
	mkdir -p $(APPDIR)/usr/bin

	@echo "    -> Copying binary..."
	cp $(BUILD_DIR)/$(APP_NAME) $(APPDIR)/usr/bin/$(APP_NAME)

	@echo "    -> Copying icon..."
	@if [ -f "$(ICON_PATH)" ]; then \
		cp $(ICON_PATH) $(APPDIR)/$(APP_NAME).png; \
	else \
		echo "⚠️ Warning: Icon not found at $(ICON_PATH)"; \
	fi

	@echo "    -> Creating AppRun launcher..."
	@echo '#!/bin/bash' > $(APPDIR)/AppRun
	@echo 'exec "$$APPDIR/usr/bin/$(APP_NAME)" "$$@"' >> $(APPDIR)/AppRun
	chmod +x $(APPDIR)/AppRun

	@echo "    -> Creating .desktop entry..."
	@echo '[Desktop Entry]' > $(APPDIR)/$(APP_NAME).desktop
	@echo 'Name=$(APP_NAME)' >> $(APPDIR)/$(APP_NAME).desktop
	@echo 'Exec=$(APP_NAME)' >> $(APPDIR)/$(APP_NAME).desktop
	@echo 'Icon=$(APP_NAME)' >> $(APPDIR)/$(APP_NAME).desktop
	@echo 'Type=Application' >> $(APPDIR)/$(APP_NAME).desktop
	@echo 'Categories=Utility;' >> $(APPDIR)/$(APP_NAME).desktop
	@echo 'Comment=A Wails-built application' >> $(APPDIR)/$(APP_NAME).desktop

	@echo "📦 Creating AppImage..."
	$(APPIMAGE_TOOL) $(APPDIR) $(FINAL_APPIMAGE)

	@echo "🧹 Cleaning up temporary AppDir..."
	rm -rf $(APPDIR)

	@echo "✅ Build complete!"
	@echo "Artifact created at: $(FINAL_APPIMAGE)"

# Clean up build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f $(BUILD_DIR)/$(APP_NAME)
	rm -f $(FINAL_APPIMAGE)
	rm -rf $(APPDIR)
	@echo "Done."
