package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// FS implements the hello world file system.
type FS struct {
	nfsRoot string // Abs path to nfs root
	ssdRoot string // Abs path to ssd root
}

func NewFS(nfsPath string, ssdPath string) *FS {
    // Add this log to see what's coming in:
    log.Printf("DEBUG NewFS: Constructor called with nfsArg='%s', ssdArg='%s'", nfsPath, ssdPath)
    fsInstance := &FS{
        nfsRoot: nfsPath, // Are you sure this line exists and is correct?
        ssdRoot: ssdPath,
    }
    // Add this log to see what's being set:
    log.Printf("DEBUG NewFS: FS instance created with fsInstance.nfsRoot='%s'", fsInstance.nfsRoot)
    return fsInstance
}

func (fs *FS) Root() (fs.Node, error) {
	return &Dir{
		fs:   fs,
		path: "",
	}, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	fs   *FS
	path string
}

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Printf("Dir.Attr called for: %s\n", d.path)

	// Get NFS directory attrs
	realNFS, err := os.Stat(d.fullNFSPath())
	if err != nil {
		log.Printf("Dir.Attr: os.Stat failed for %s: %v", d.fullNFSPath(), err)
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return err
	}

	a.Mode = os.ModeDir | 0555 // Read-only
	a.Valid = 0                // TODO(wes): Should this be increased? Caches attributes for n seconds
	a.Inode = realNFS.Sys().(*syscall.Stat_t).Ino
	a.Gid = realNFS.Sys().(*syscall.Stat_t).Gid
	a.Uid = realNFS.Sys().(*syscall.Stat_t).Uid
	a.Mtime = realNFS.ModTime()
	a.Ctime = realNFS.ModTime() // For simplicity, use Mtime for Ctime as well
	a.Size = uint64(realNFS.Size())

	return nil
}

func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	requestedNFSPath := filepath.Join(d.fullNFSPath(), name)
	log.Printf("Dir.Lookup called in dir '%s' for name '%s'. Full NFS path: %s\n", d.path, name, requestedNFSPath)

	fi, err := os.Lstat(requestedNFSPath) // Use Lstat to not follow symlinks if any
	if err != nil {
		log.Printf("Dir.Lookup: os.Lstat failed for %s: %v", requestedNFSPath, err)
		if os.IsNotExist(err) {
			return nil, syscall.ENOENT
		}
		return nil, err // Or map to a syscall error
	}

	var n fs.Node
	if fi.IsDir() {
		log.Printf("Dir.Lookup: '%s' is a directory. Returning Dir node.", name)
		n = &Dir{fs: d.fs, path: requestedNFSPath} // TODO(wes): I think recursive folders should take their parent's path + name
	} else {
		// It's a file (or symlink, etc., but we'll treat as file for now)
		log.Printf("Dir.Lookup: '%s' is a file. Returning File node.", name)
		n = &File{fs: d.fs, path: requestedNFSPath} // TODO(wes): I think this should take its parent's path + name
	}
	return n, nil
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Printf("Dir.ReadDirAll called for: %s\n", d.fullNFSPath())

	var entries []fuse.Dirent

	// Read the nfsPath directory
	dirEntries, err := os.ReadDir(d.fullNFSPath())
	if err != nil {
		log.Printf("Error reading nfsPath %s: %v", d.fullNFSPath(), err)
		if os.IsNotExist(err) {
			return nil, syscall.ENOENT
		}
		if os.IsPermission(err) {
			return nil, syscall.EACCES
		}
		return nil, syscall.EIO
	}

	for _, entry := range dirEntries {
		var d fuse.Dirent
		d.Name = entry.Name()
		// Determine if it's a directory or file for d.Type
		if entry.IsDir() {
			d.Type = fuse.DT_Dir
		} else {
			d.Type = fuse.DT_File
		}
		entries = append(entries, d)
	}
	log.Printf("Dir.ReadDirAll for %s returning: %+v\n", d.fullNFSPath(), entries)
	return entries, nil
}

func (d *Dir) fullNFSPath() string {
	// TODO(wes): I don't think this works yet for recursive dirs - might need to fix on Dir
	// return filepath.Join(d.fs.nfsRoot, d.path)
	return d.path
}

// File represents a file in our filesystem.
type File struct {
	fs   *FS
	path string
}

// Attr implements fs.Node.
func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Printf("File.Attr called for: %s", f.path)

	// Attributes will first be checked from SSD (cache), then fallback to NFS
	// For now, let's just use NFS for attributes. Caching logic will come later.
	nfsFullPath := f.fullNFSPath()
	log.Printf("File.Attr: Checking attributes from NFS path: %s", nfsFullPath)

	fi, err := os.Lstat(nfsFullPath) // Lstat to handle symlinks properly if they were to exist
	if err != nil {
		log.Printf("File.Attr: os.Lstat failed for %s: %v", nfsFullPath, err)
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return err // Or map to syscall error
	}

	a.Mode = 0444 // Read-only file
	a.Size = uint64(fi.Size())
	a.Mtime = fi.ModTime()
	a.Ctime = fi.ModTime() // As with Dir, using Mtime for Ctime for simplicity
	a.Inode = fi.Sys().(*syscall.Stat_t).Ino
	a.Gid = fi.Sys().(*syscall.Stat_t).Gid
	a.Uid = fi.Sys().(*syscall.Stat_t).Uid
	a.Valid = 0 // Cache attributes for 1 second, or 0 for re-validation

	log.Printf("File.Attr for %s: Mode=%v, Size=%d", f.path, a.Mode, a.Size)
	return nil
}

// Helper to get the full NFS path for this file
func (f *File) fullNFSPath() string {
	// return f.fs.nfsRoot + "/" + f.path
	return f.path
}

// Helper to get the full SSD cache path for this file
func (f *File) fullSSDPath() string {
	// All projects share the same cache, so we'll store files directly in ssdRoot
	// using their full relative path from nfsRoot as their name in the cache to avoid collisions.
	// e.g., nfs/project-1/common-lib.py becomes ssd/project-1/common-lib.py
	// e.g., nfs/project-2/common-lib.py becomes ssd/project-2/common-lib.py
	// This structure makes it easy to locate cached files and mirrors the NFS structure.
	return f.fs.ssdRoot + "/" + f.path
}
