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

func NewFS(mountpoint, nfsDir, ssdDir string) FuseFS {
	absNFSDir, err := filepath.Abs(nfsDir)
	if err != nil {
		log.Fatalf("FATAL: Invalid NFS relative path '%s'", nfsDir)
	} else if _, err := os.Stat(absNFSDir); err != nil {
		log.Fatalf("FATAL: Could not find NFS path '%s'", absNFSDir)
	}
	absSSDDir, err := filepath.Abs(ssdDir)
	if err != nil {
		log.Fatalf("FATAL: Invalid SSD relative path '%s'", ssdDir)
	} else if _, err := os.Stat(absSSDDir); err != nil {
		log.Fatalf("FATAL: Could not find SSD path '%s'", absSSDDir)
	}

	rfs := &fuseFS{
		mountpoint: mountpoint,
		lastInode:  1,
		nfsBaseAbs: absNFSDir,
		ssdBaseAbs: absSSDDir,
	}

	rootNode, err := loadFSTree(rfs)
	if err != nil {
		log.Fatalf("FATAL: Building FS: '%v'", err)
	}

	rfs.rootNode = rootNode

	printTree(rootNode, "")

	return rfs
}

type fuseFS struct {
	mountpoint string
	lastInode  uint64 // TODO(wes): Atomic?
	conn       *fuse.Conn
	nfsBaseAbs string
	ssdBaseAbs string

	rootNode FuseFSNode // TODO(wes): Should this rather be a map[path]node?
	// Add SSD here
}

func (rfs *fuseFS) Mount() error {
	c, err := fuse.Mount(
		rfs.mountpoint,
		fuse.FSName("fusefs"),
		fuse.Subtype("fusefs"),
		fuse.ReadOnly(),
	)
	if err != nil {
		return err
	}
	rfs.conn = c

	return nil
}

func (rfs *fuseFS) Serve() error {
	server := fs.New(
		rfs.conn,
		&fs.Config{
			Debug: func(msg any) {
				log.Printf("S_DEBUG: '%v'", msg)
			},
		})
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

func (rfs *fuseFS) Root() (fs.Node, error) {
	return rfs.rootNode, nil
}

// GenerateInode keeps a global fs counter and just increments it for simplicity
func (rfs *fuseFS) GenerateInode(_ uint64, _ string) uint64 {
	rfs.lastInode++
	return rfs.lastInode
}

func loadFSTree(fs *fuseFS) (*fuseFSNode, error) {
	rootNFSNode := NewFuseFSNode(
		fs,
		"",
		"", // Relative to base NFS/SSD
		fs.GenerateInode(0, ""),
		os.ModeDir|perm_READ,
		true,
	)

	// nodesByRelPath maps a directory's relative path to its node object
	// This helps in finding the parent node for the current entry.
	nodesByRelPath := make(map[string]*fuseFSNode)
	nodesByRelPath[""] = rootNFSNode

	walkErr := filepath.WalkDir("", func(currentAbsNFSPath string, d native_fs.DirEntry, err error) error {
		if err != nil {
			// This error is from filepath.WalkDir itself, e.g., permission denied to list a directory.
			fmt.Printf("Error accessing path %q: %v. Skipping subtree.\n", currentAbsNFSPath, err)
			if d != nil && d.IsDir() {
				return native_fs.SkipDir // Skip processing this directory further
			}
			return err // Propagate error to stop WalkDir if it's critical or for a file
		}

		// Skip the root directory itself in the callback, as we've already created its node.
		if currentAbsNFSPath == fs.nfsBaseAbs {
			return nil
		}

		parentAbsNFSPath := filepath.Dir(currentAbsNFSPath)
		parentRelPath, _ := filepath.Rel(fs.nfsBaseAbs, parentAbsNFSPath)

		// Determine parent node
		parent, ok := nodesByRelPath[parentRelPath]
		if !ok {
			// This should ideally not happen if WalkDir processes parents before children.
			return fmt.Errorf("parent node not found for path: %s (parent: %s)", currentAbsNFSPath, parentRelPath)
		}

		mode := os.ModeDir | perm_READEXECUTE
		if !d.IsDir() {
			mode = perm_READEXECUTE
		}

		currentNode := NewFuseFSNode(
			fs,
			d.Name(),
			parentRelPath,
			fs.GenerateInode(parent.Inode, d.Name()),
			mode,
			d.IsDir(),
		)

		if d.IsDir() {
			// Add to nodesByPath so its children can find it.
			nodesByRelPath[parentRelPath] = currentNode
		}

		// Add current node to its parent's children list
		parent.Children = append(parent.Children, currentNode)

		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("error walking from root: %w", walkErr)
	}

	return rootNFSNode, nil
}
