package main

import (
	"context"
	"fmt"
	native_fs "io/fs"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"bazil.org/fuse/fuseutil"
)

type FuseFSNode interface {
	fs.Node
	fs.HandleReadDirAller
	fs.NodeStringLookuper

	// TODO(wes): Add some more interfaces?
	// fs.HandleReadAller
	// fs.NodeOpener
	// fs.NodeRemover // Allows rm and rmdir
	// fs.SetAttr // Allows chmod I think?
	// fs.MakeDirer
}

func NewFuseFSNode(fs *fuseFS, name, parentPathRel string, inode uint64, mode os.FileMode, isDir bool) *fuseFSNode {
	return &fuseFSNode{
		FS:            fs,
		Name:          name,
		parentPathRel: parentPathRel,
		Inode:         inode,
		Mode:          mode,
		isDir:         isDir,
	}
}

type fuseFSNode struct {
	FS            *fuseFS
	Name          string
	parentPathRel string // Relative to NFS/SSD base
	Inode         uint64
	Mode          os.FileMode
	isDir         bool

	Children []*fuseFSNode // nil for files
}

func (n *fuseFSNode) relPath() string {
	return filepath.Join(n.parentPathRel, n.Name)
}

func (n *fuseFSNode) nfsPathAbs() string {
	return filepath.Join(n.FS.nfsBaseAbs, n.parentPathRel, n.Name)
}

func (n *fuseFSNode) stat() (native_fs.FileInfo, error) {
	return os.Stat(n.nfsPathAbs()) // NFS is source of truth
}

func (n *fuseFSNode) data() ([]byte, error) {
	fi, err := n.stat()
	if err != nil {
		return nil, err
	} else if fi.IsDir() {
		return nil, syscall.EISDIR
	}

	// 1. Try reading from SSD cache
	cachedData, err := n.FS.ssdCache.Get(n.relPath())
	if err == nil {
		log.Printf("CACHE_HIT: Read %d bytes from SSD for '%s'", len(cachedData), n.relPath())
		return cachedData, nil
	}
	if err != ErrNotFoundCache {
		// An error other than the file not being present in the cache - could be bad but we should continue
		log.Printf("WARNING: Error reading from SSD cache for %s (will try NFS): %v", n.relPath(), err)
	}

	// 2. Try reading from NFS file system
	time.Sleep(nfsFileReadDelay)
	nfsData, err := os.ReadFile(n.nfsPathAbs())
	if err != nil {
		log.Printf("ERROR: Failed to read from NFS path %s: %v", n.nfsPathAbs(), err)
		return nil, syscall.EIO // Return an appropriate FUSE error (I/O error)
	}
	log.Printf("NFS_READ: Read %d bytes for '%s'", len(nfsData), n.relPath())

	// 3. Write the file to the cache with the same permissions it has in FUSE/NFS.
	if err := n.FS.ssdCache.Put(n.relPath(), nfsData, n.Mode); err == ErrWontCache {
		log.Printf("WARNING: Cache refuse to write file: '%v'", err)
	} else if err != nil {
		log.Printf("ERROR: Failed to write to cache %s: %v. Proceeding without caching.", n.relPath(), err)
	} else {
		log.Printf("CACHE_LOADED: Copied '%s' from NFS to cache", n.relPath())
	}

	return nfsData, nil
}

func (n *fuseFSNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = n.Inode
	attr.Mode = n.Mode

	fi, err := n.stat()
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		attr.Size = uint64(fi.Size())
	}

	return nil
}

func (n *fuseFSNode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	// TODO(wes): Lazy load?

	ents := make([]fuse.Dirent, len(n.Children))
	for i, node := range n.Children {
		typ := fuse.DT_File
		if node.Mode.IsDir() {
			typ = fuse.DT_Dir
		}
		ents[i] = fuse.Dirent{Inode: node.Inode, Type: typ, Name: node.Name}
	}
	return ents, nil
}

func (n *fuseFSNode) Lookup(ctx context.Context, name string) (fs.Node, error) {
	for _, n := range n.Children {
		if n.Name == name {
			return n, nil
		} else if n.Mode.IsDir() {
			// TODO: Check if this is needed
			if lookupNode, err := n.Lookup(ctx, name); err == nil {
				return lookupNode, nil
			}
		}
	}
	return nil, syscall.ENOENT
}

func (n *fuseFSNode) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	data, err := n.data()
	if err != nil {
		return err
	}

	fuseutil.HandleRead(req, resp, data)
	return nil
}

// Helper function to print the tree (for verification)
func printTree(n *fuseFSNode, indent string) {
	var contentInfo, nodeType string
	if n.isDir {
		nodeType = "Dir"
		contentInfo = fmt.Sprintf("%d children", len(n.Children))
	} else {
		nodeType = "File"
		if fi, err := n.stat(); err != nil {
			contentInfo = fmt.Sprintf("'%v'", err)
		} else {
			contentInfo = fmt.Sprintf("%d bytes", fi.Size())
		}
	}
	fmt.Printf("%s%s[%d] (%s: %s) -> %s\n", indent, n.Name, n.Inode, nodeType, contentInfo, n.relPath())

	for _, child := range n.Children {
		printTree(child, indent+"  ")
	}
}
