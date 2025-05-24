package main

import (
	"context"
	"log"
	"os"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// FS implements the hello world file system.
type FS struct{}

func (FS) Root() (fs.Node, error) {
	return Dir{}, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct{}

func (Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Println("Dir.Attr called")
	a.Inode = 1
	a.Mode = os.ModeDir | 0555 // Read-only
	return nil
}

func (Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Printf("Dir.Lookup called for: %s\n", name)
	return nil, syscall.ENOENT
}

func (Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("Dir.ReadDirAll called for root")
	var entries []fuse.Dirent

	// Read the nfsPath directory
	dirEntries, err := os.ReadDir(nfsDir) // Using os.ReadDir is generally preferred
	if err != nil {
		log.Printf("Error reading nfsPath %s: %v", nfsDir, err)
		return nil, err // Or a FUSE-specific error like syscall.EIO
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
	log.Printf("Dir.ReadDirAll returning: %+v", entries)
	return entries, nil
}
