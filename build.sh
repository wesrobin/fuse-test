#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
# The name of your Go application's output binary
APP_NAME="fuse-test"

# The directory to copy files to
DESTINATION_COPY_FOLDER="nfs"

# --- Script Functions ---

# Function to print messages
log() {
  echo "INFO: $1"
}

# Function to print error messages and exit
error_exit() {
  echo "ERROR: $1" >&2
  exit 1
}

# --- Main Script ---

# 1. Ask for the source location of files to copy
read -p "Enter the full path to the source directory of files to copy. Leave empty for default ('testdata'): " SOURCE_FILES_LOCATION

if [ -z "$SOURCE_FILES_LOCATION" ]; then
  SOURCE_FILES_LOCATION="testdata"
fi

if [ ! -d "$SOURCE_FILES_LOCATION" ]; then
  error_exit "Source directory '$SOURCE_FILES_LOCATION' does not exist."
fi

log "Starting build process..."

# 2. Update package list and install fuse3
sudo apt update
sudo apt install -y fuse3

# 3. Build the Go application
# Assumes your Go files are in the current directory ('.')
# and the main package is also in the current directory.
log "Building Go application '$APP_NAME'..."
if ! go build -o "$APP_NAME" .; then
  error_exit "Go build failed."
fi
log "Go application built successfully: $APP_NAME"

# 4. Create required directories in the working directory
log "Creating directories: nfs, ssd, mnt/all-projects, $DESTINATION_COPY_FOLDER"
mkdir -p nfs
mkdir -p ssd
mkdir -p mnt/all-projects # -p creates parent directories if they don't exist
log "Directories created."

# 5. Copy files from user-specified source to the destination folder
log "Copying files from '$SOURCE_FILES_LOCATION' to '$DESTINATION_COPY_FOLDER/'..."
# Using rsync for better feedback and handling of directories.
# The trailing slash on the source ensures the *contents* of the directory are copied.
if ! rsync -av --progress "$SOURCE_FILES_LOCATION/" "$DESTINATION_COPY_FOLDER/"; then
  error_exit "Failed to copy files from '$SOURCE_FILES_LOCATION'."
fi
log "Files copied successfully."

log "Build script completed successfully!"
echo "Your application '$APP_NAME' is ready."
echo "Files from '$SOURCE_FILES_LOCATION' are copied to '$DESTINATION_COPY_FOLDER'."
echo "Directories 'nfs', 'ssd', and 'mnt/all-projects' are created."

exit 0
