package main

import (
	"fmt"
	native_fs "io/fs"
	"log"
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FuseFS interface {
	Mount() error
	Serve() error
	Unmount() error
	Mountpoint() string

	fs.FS
	fs.FSInodeGenerator
}

func NewFS(mountpoint, nfsDir string) FuseFS {
	rfs := &fuseFS{mountpoint: mountpoint, lastInode: 1}

	rootNode, err := loadFSTree(nfsDir, rfs)
	if err != nil {
		log.Fatalf("FATAL: Building FS: '%v'", err)
	}

	rfs.rootNFSNode = rootNode

	printTree(rootNode, "")

	return rfs
}

type fuseFS struct {
	mountpoint string
	lastInode  uint64 // TODO(wes): Atomic?
	conn       *fuse.Conn

	rootNFSNode FuseFSNode
	// Add SSD here
}

func (rfs *fuseFS) Mount() error {
	c, err := fuse.Mount(
		rfs.mountpoint,
		fuse.FSName("fusefs"),
		fuse.Subtype("fusefs"),
	)
	if err != nil {
		return err
	}
	rfs.conn = c

	return nil
}

func (rfs *fuseFS) Serve() error {
	server := fs.New(rfs.conn, &fs.Config{Debug: func(msg any) { log.Printf("SERVER_DEBUG: '%v'", msg) }})
	return server.Serve(rfs)
}

func (rfs *fuseFS) Unmount() error {
	err := fuse.Unmount(rfs.mountpoint)
	if err != nil {
		return err
	}

	return rfs.conn.Close()
}

func (rfs *fuseFS) Mountpoint() string {
	return rfs.mountpoint
}

/* fs.FS */
func (rfs *fuseFS) Root() (fs.Node, error) {
	return rfs.rootNFSNode, nil
}

/* fs.FSInodeGenerator */
func (rfs *fuseFS) GenerateInode(parentInode uint64, _ string) uint64 {
	rfs.lastInode++
	return rfs.lastInode
}

func loadFSTree(nfsDirPath string, fs FuseFS) (*fuseFSNode, error) {
	absRootDirPath, err := filepath.Abs(nfsDirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", nfsDirPath, err)
	}

	rootInfo, err := os.Stat(absRootDirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat root directory %s: %w", absRootDirPath, err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("root path %s is not a directory", absRootDirPath)
	}

	rootNFSNode := NewFuseFSNode(
		fs,
		"",
		nfsDir,
		fs.GenerateInode(0, ""),
		os.ModeDir|0o555,
		true,
	)

	// nodesByPath maps a directory's absolute path to its node object
	// This helps in finding the parent node for the current entry.
	nodesByPath := make(map[string]*fuseFSNode)
	nodesByPath[absRootDirPath] = rootNFSNode

	walkErr := filepath.WalkDir(absRootDirPath, func(path string, d native_fs.DirEntry, err error) error {
		if err != nil {
			// This error is from filepath.WalkDir itself, e.g., permission denied to list a directory.
			fmt.Printf("Error accessing path %q: %v. Skipping subtree.\n", path, err)
			if d != nil && d.IsDir() {
				return native_fs.SkipDir // Skip processing this directory further
			}
			return err // Propagate error to stop WalkDir if it's critical or for a file
		}

		// Skip the root directory itself in the callback, as we've already created its node.
		if path == absRootDirPath {
			return nil
		}

		// Determine parent node
		parentPath := filepath.Dir(path)
		parent, ok := nodesByPath[parentPath]
		if !ok {
			// This should ideally not happen if WalkDir processes parents before children.
			return fmt.Errorf("parent node not found for path: %s (parent: %s)", path, parentPath)
		}

		mode := os.ModeDir | 0o555
		if !d.IsDir() {
			mode = 0o555
		}

		currentNode := NewFuseFSNode(
			fs,
			d.Name(),
			parentPath,
			fs.GenerateInode(parent.Inode, d.Name()),
			mode,
			d.IsDir(),
		)

		if d.IsDir() {
			// Add to nodesByPath so its children can find it.
			nodesByPath[path] = currentNode
		}

		// Add current node to its parent's children list
		parent.Children = append(parent.Children, currentNode)

		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("error walking the path %s: %w", absRootDirPath, walkErr)
	}

	return rootNFSNode, nil
}
