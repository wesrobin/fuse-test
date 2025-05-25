package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	_ "bazil.org/fuse/fs/fstestutil"
)

const (
	mountPoint = "./mnt/all-projects"
	nfsDir     = "./nfs" // Path to our simulated NFS directory
	ssdDir     = "./ssd" // Path to our simulated SSD cache directory
)

func main() {
	ensureDirs(mountPoint, nfsDir, ssdDir)

	log.Printf("Mount point at %s", mountPoint)
	log.Printf("NFS source (relative): %s", nfsDir)
	log.Printf("SSD cache (relative): %s", ssdDir)

	fuseFS := NewFS(mountPoint, nfsDir)
	_ = fuseFS

	if err := fuseFS.Mount(); err != nil {
		log.Fatalf("failed to mount: '%v'", err)
	}

	log.Printf("Mounted file system at '%v'", mountPoint)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("Unmounted filesystem from %s", mountPoint)
		if err := fuseFS.Unmount(); err != nil {
			log.Fatalf("failed to unmount: '%v'", err)
		}
	}()

	if err := fuseFS.Serve(); err != nil {
		log.Fatalf("failed to serve: '%v'", err)
	}

	// c, err := fuse.Mount(
	// 	mountPoint,
	// 	fuse.FSName("cachingfs"),
	// 	fuse.Subtype("cachefs"),
	// 	// fuse.AsyncRead(), // Can add for concurrent reads
	// )
	// if err != nil {
	// 	log.Fatalf("Initialising FUSE connection to dir %s: %v", mountPoint, err)
	// }
	// sigChan := make(chan os.Signal, 1)
	// signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	// go func() {
	// 	<-sigChan
	// 	err := c.Close()
	// 	if err != nil {
	// 		log.Fatalf("Closing FUSE connection to dir %s: %v", mountPoint, err)
	// 	}
	// 	log.Printf("Closed fuse connection to %s", mountPoint)

	// 	if err = fuse.Unmount(mountPoint); err != nil {
	// 		log.Fatalf("failed to unmount: %w", err)
	// 	}
	// 	log.Printf("Unmounted filesystem from %s", mountPoint)
	// }()

	// log.Println("Filesystem mounted. Ctrl+C to unmount and exit.")

	// log.Println("Initial NFS file structure created.")

	// log.Printf("DEBUG main: absNFSPath = '%s', absSSDPath = '%s'", absNFS, absSSD)

	// fuseFS := NewFS(absNFS, absSSD)

	// fsSrv := fs.New(c, &fs.Config{Debug: func(msg interface{}) { log.Printf("SERVER_DEBUG: '%v'", msg) }})

	// err = fsSrv.Serve(fuseFS)
	// if err != nil {
	// 	log.Fatalf("Serve failed: %v", err)
	// }
}

// TODO(wes): Bubble errs up
func ensureDirs(mount, nfs, ssd string) {
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

	_ = os.MkdirAll(filepath.Join(nfs, "/project-1"), 0755)
	_ = os.WriteFile(filepath.Join(nfs, "/project-1/main.py"), []byte("# project-1 main.py\nprint('Hello from project-1 main')"), 0644)
	_ = os.WriteFile(filepath.Join(nfs, "/project-1/common-lib.py"), []byte("# common-lib.py in project-1\nprint('Hello from common-lib in project-1')"), 0644)

	_ = os.MkdirAll(filepath.Join(nfs, "/project-2"), 0755)
	_ = os.WriteFile(filepath.Join(nfs, "/project-2/entrypoint.py"), []byte("# project-2 entrypoint.py\nprint('Hello from project-2 entrypoint')"), 0644)
	_ = os.WriteFile(filepath.Join(nfs, "/project-2/common-lib.py"), []byte("# common-lib.py in project-2\nprint('Hello from common-lib in project-2')"), 0644)
}
