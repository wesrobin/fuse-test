#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
# The name of your Go application's output binary
APP_NAME="fuse-test"

# The directory to copy files to
DESTINATION_COPY_FOLDER="nfs"
DIRECTORIES_TO_CREATE=("nfs" "ssd" "mnt/all-projects")

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
# 2. Check for fuse3 (fusermount3) and install if necessary
log "Checking for fuse3 (fusermount3)..."
if command -v fusermount3 &> /dev/null; then
  log "fusermount3 (from fuse3 package) is already installed."
else
  log "fusermount3 not found. Attempting to install fuse3 package."
  log "Updating package list (sudo required)..."
  if ! sudo apt update; then
    error_exit "apt update failed. Please check your internet connection and permissions."
  fi

  log "Installing fuse3 (sudo required)..."
  if ! sudo apt install -y fuse3; then
    error_exit "fuse3 installation failed. Please check the output above for errors."
  fi
  log "fuse3 installed successfully."
fi

# 3. Build the Go application
# Assumes your Go files are in the current directory ('.')
# and the main package is also in the current directory.
log "Building Go application '$APP_NAME'..."
if ! go build -o "$APP_NAME" .; then
  error_exit "Go build failed."
fi
log "Go application built successfully: $APP_NAME"

# 4. Create required directories in the working directory
# 4. Check and remove existing directories, then recreate them
log "Preparing directories..."
for dir_name in "${DIRECTORIES_TO_CREATE[@]}"; do
  if [ -d "$dir_name" ]; then
    log "Directory '$dir_name' exists. Removing it..."
    if ! rm -rf "$dir_name"; then
      error_exit "Failed to remove directory '$dir_name'. Check permissions."
    fi
    log "Directory '$dir_name' removed."
  elif [ -f "$dir_name" ]; then
    log "A file named '$dir_name' exists and is not a directory. Removing it..."
    if ! rm -f "$dir_name"; then
      error_exit "Failed to remove file '$dir_name'. Check permissions."
    fi
    log "File '$dir_name' removed."
  fi

  log "Creating directory '$dir_name'..."
  if ! mkdir -p "$dir_name"; then
    error_exit "Failed to create directory '$dir_name'."
  fi
  log "Directory '$dir_name' created."
done
log "All specified directories are ready."log "Directories created."

# 5. Copy files from user-specified source to the destination folder
log "Copying files from '$SOURCE_FILES_LOCATION' to '$DESTINATION_COPY_FOLDER/'..."
# Using rsync for better feedback and handling of directories.
# The trailing slash on the source ensures the *contents* of the directory are copied.
if ! rsync -av --progress "$SOURCE_FILES_LOCATION/" "$DESTINATION_COPY_FOLDER/"; then
  error_exit "Failed to copy files from '$SOURCE_FILES_LOCATION'."
fi
log "Files copied successfully."

log "Build script completed successfully!"
echo ""
echo "--------------------------------------------------"
echo " SUMMARY:"
echo "--------------------------------------------------"
echo " > Application '$APP_NAME' built."
echo " > fuse3 installation checked/performed."
echo " > Directories created/recreated: ${DIRECTORIES_TO_CREATE[*]}"
echo " > Files from '$SOURCE_FILES_LOCATION' copied to '$DESTINATION_COPY_FOLDER'."
echo "--------------------------------------------------"
echo ""
echo "Your application '$APP_NAME' is ready in the current directory. Run it with ./$APP_NAME"

exit 0
