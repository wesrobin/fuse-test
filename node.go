package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"bazil.org/fuse/fuseutil"
)

type FuseFSNode interface {
	fs.Node
	fs.HandleReadDirAller
	fs.HandleReadAller
	fs.NodeOpener
	fs.NodeStringLookuper

	// TODO(wes): Add some more interfaces?
	// fs.NodeRemover // Allows rm and rmdir
	// fs.SetAttr // Allows chmod I think?
	// fs.MakeDirer
}

func NewFuseFSNode() FuseFSNode {
	return &fuseFSNode{}
}

type fuseFSNode struct {
	FS    FuseFS
	Name  string
	Inode uint64
	Mode  os.FileMode
	isDir bool
	Data  []byte // nil for dirs

	Children []*fuseFSNode // nil for files
}

// Helper function to print the tree (for verification)
func printTree(n *fuseFSNode, indent string) {
	nodeType := "File"
	contentInfo := fmt.Sprintf("%d bytes", len(n.Data))
	if n.isDir {
		nodeType = "Dir"
		contentInfo = fmt.Sprintf("%d children", len(n.Children))
	}
	fmt.Printf("%s%s[%d] (%s: %s)\n", indent, n.Name, n.Inode, nodeType, contentInfo)

	for _, child := range n.Children {
		printTree(child, indent+"  ")
	}
}

func (n *fuseFSNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = n.Inode
	attr.Mode = n.Mode

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

func (n *fuseFSNode) ReadAll(ctx context.Context) ([]byte, error) {
	if n.isDir {
		return nil, fuse.Errno(syscall.EISDIR)
	}
	return n.Data, nil
}

// TODO(wes): The reading doesn't seem to be working..

func (n *fuseFSNode) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	if !req.Flags.IsReadOnly() {
		return nil, fuse.Errno(syscall.EACCES)
	}
	resp.Flags |= fuse.OpenKeepCache
	return n, nil
}

func (n *fuseFSNode) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	fuseutil.HandleRead(req, resp, n.Data)
	return nil
}
