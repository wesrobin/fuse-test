package main

import (
	"context"
	"fmt"
	native_fs "io/fs"
	"os"
	"path/filepath"
	"syscall"

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

func NewFuseFSNode(FS FuseFS, name, parentPath string, inode uint64, mode os.FileMode, isDir bool) *fuseFSNode {
	return &fuseFSNode{
		FS:         FS,
		Name:       name,
		parentPath: parentPath,
		Inode:      inode,
		Mode:       mode,
		isDir:      isDir,
	}
}

type fuseFSNode struct {
	FS         FuseFS
	Name       string
	parentPath string
	Inode      uint64
	Mode       os.FileMode
	isDir      bool

	Children []*fuseFSNode // nil for files
}

func (n *fuseFSNode) path() string {
	return filepath.Join(n.parentPath, n.Name)
}

func (n *fuseFSNode) stat() (native_fs.FileInfo, error) {
	return os.Stat(n.path())
}

func (n *fuseFSNode) data() ([]byte, error) {
	fi, err := n.stat()
	if err != nil {
		return nil, err
	} else if fi.IsDir() {
		// TODO(wes): Return err
		return nil, nil
	}

	// TODO(wes): Return actual data
	/*
	   // It's a file, read its content.
	   fileData, readErr := os.ReadFile(path)
	   if readErr != nil {
	       log.Printf("Failed to read file %s: '%v'. Skipping content.\n", path, readErr)
	       currentNode.Data = nil
	   } else {
	       currentNode.Data = fileData
	   }
	*/
	return make([]byte, fi.Size()), nil
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
	fmt.Printf("%s%s[%d] (%s: %s) -> %s\n", indent, n.Name, n.Inode, nodeType, contentInfo, n.path())

	for _, child := range n.Children {
		printTree(child, indent+"  ")
	}
}
