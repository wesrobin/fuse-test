package main

import (
	"log"
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
)

const (
	mountPoint = "./mnt/all-projects"
	nfsDir     = "./nfs" // Path to our simulated NFS directory
	ssdDir     = "./ssd" // Path to our simulated SSD cache directory
)

func main() {
	absNFS, absSSD := ensureDirs(mountPoint, nfsDir, ssdDir)

	log.Printf("Mounting filesystem at %s", mountPoint)
	log.Printf("NFS source (relative): %s", nfsDir)
	log.Printf("SSD cache (relative): %s", ssdDir)

	// absNFS, absSSD = nfsDir, ssdDir

	c, err := fuse.Mount(
		mountPoint,
		fuse.FSName("cachingfs"),
		fuse.Subtype("cachefs"),
		// fuse.AsyncRead(), // Can add for concurrent reads
	)
	if err != nil {
		log.Fatalf("Initialising FUSE connection to dir %s: %v", mountPoint, err)
	}
	defer func(c *fuse.Conn) {
		err := c.Close()
		if err != nil {
			log.Fatalf("Closing FUSE connection to dir %s: %v", mountPoint, err)
		}
	}(c)
	defer func() {
		err := fuse.Unmount(mountPoint)
		if err != nil {
			log.Fatalf("Unmounting FUSE connection to dir %s: %v", mountPoint, err)
		}
	}()

	log.Println("Filesystem mounted. Ctrl+C to unmount and exit.")

	_ = os.MkdirAll(filepath.Join(absNFS, "/project-1"), 0755)
	_ = os.WriteFile(filepath.Join(absNFS, "/project-1/main.py"), []byte("# project-1 main.py\nprint('Hello from project-1 main')"), 0644)
	_ = os.WriteFile(filepath.Join(absNFS, "/project-1/common-lib.py"), []byte("# common-lib.py in project-1\nprint('Hello from common-lib in project-1')"), 0644)

	_ = os.MkdirAll(filepath.Join(absNFS, "/project-2"), 0755)
	_ = os.WriteFile(filepath.Join(absNFS, "/project-2/entrypoint.py"), []byte("# project-2 entrypoint.py\nprint('Hello from project-2 entrypoint')"), 0644)
	_ = os.WriteFile(filepath.Join(absNFS, "/project-2/common-lib.py"), []byte("# common-lib.py in project-2\nprint('Hello from common-lib in project-2')"), 0644)

	log.Println("Initial NFS file structure created.")

	log.Printf("DEBUG main: absNFSPath = '%s', absSSDPath = '%s'", absNFS, absSSD)

	fuseFS := NewFS(absNFS, absSSD)

	err = fs.Serve(c, fuseFS)
	if err != nil {
		log.Fatalf("Serve failed: %v", err)
	}
}

// TODO(wes): Bubble errs up
func ensureDirs(mount, nfs, ssd string) (string, string) {
	if err := os.MkdirAll(mount, 0755); err != nil {
		log.Fatalf("Creating mount point %s: %v", mount, err)
	}
	log.Printf("Mount created: %s", mount)

	// Create nfsPath and ssdPath if they don't exist for initial setup
	if err := os.MkdirAll(nfs, 0755); err != nil {
		log.Fatalf("Creating NFS path %s: %v", nfs, err)
	}
	log.Printf("NFS source: %s", nfsDir)

	if err := os.MkdirAll(ssd, 0755); err != nil {
		log.Fatalf("Creating SSD path %s: %v", ssd, err)
	}
	log.Printf("SSD cache: %s", ssdDir)

	absNFSPath, err := filepath.Abs(nfs)
	if err != nil {
		log.Fatalf("Failed to get absolute path for NFS: %v", err)
	}
	absSSDPath, err := filepath.Abs(ssd)
	if err != nil {
		log.Fatalf("Failed to get absolute path for SSD: %v", err)
	}

	return absNFSPath, absSSDPath
}
