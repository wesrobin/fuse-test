package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "bazil.org/fuse/fs/fstestutil"
)

const (
	mountPoint = "./mnt/all-projects"
	nfsDir     = "./nfs" // Path to our simulated NFS directory
	ssdDir     = "./ssd" // Path to our simulated SSD cache directory

	nfsFileReadDelay = time.Second

	perm_READWRITEEXECUTE = 0o700
	perm_READEXECUTE      = 0o500
	perm_READ             = 0o400
)

var (
	// ** Cache specific **
	cache       = flag.String("cache", "default", "Define which cache to use (size, lru). If not specified, default cache is used.\n EXAMPLE: --cache=lru")
	lruCapacity = flag.Int("lrucap", 2, "Define the capacity of the LRU cache. Only used when --cache=lru is set.")
	lruDebug    = flag.Bool("lrudebug", false, "When specified, enable cache debugging (only available with LRU cache).")
	sizeLimit   = flag.Int64("sizelim", 128, "Define the capacity of the Size Limited cache. Only used when --cache=size is set.")

	// ** FUSE debugging **
	debugServer = flag.Bool("sdebug", false, "When specified, log FUSE server messages.")
)

func usage() {
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	ensureDirs(mountPoint, nfsDir, ssdDir)

	log.Printf("Mount point at %s", mountPoint)
	log.Printf("NFS source (relative): %s", nfsDir)
	log.Printf("SSD cache (relative): %s", ssdDir)

	absSSDDir, err := filepath.Abs(ssdDir)
	if err != nil {
		log.Fatalf("FATAL: Invalid SSD relative path '%s'", ssdDir)
	} else if _, err := os.Stat(absSSDDir); err != nil {
		log.Fatalf("FATAL: Could not find SSD path '%s'", absSSDDir)
	}

	fuseFS := NewFS(mountPoint, nfsDir, ssdDir, initCache(absSSDDir))

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

	if err := fuseFS.Serve(*debugServer); err != nil {
		log.Fatalf("failed to serve: '%v'", err)
	}
}

func initCache(ssdDir string) Cache {
	var c Cache
	switch *cache {
	case "lru":
		c = NewLRUCache(ssdDir, *lruCapacity, *lruDebug)
	case "size":
		c = NewSizeLimitedCache(ssdDir, *sizeLimit)
	default:
		c = NewDefaultCache(ssdDir)
	}
	return c
}

// TODO(wes): Bubble errs up
func ensureDirs(mount, nfs, ssd string) {
	if err := os.RemoveAll(mount); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing mountpoint %s: %v", mount, err)
	}
	if err := os.MkdirAll(mount, perm_READWRITEEXECUTE); err != nil {
		log.Fatalf("Creating mount point %s: %v", mount, err)
	}
	log.Printf("Mount created: %s", mount)

	// Create nfsPath and ssdPath if they don't exist for initial setup
	if err := os.RemoveAll(nfs); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing mountpoint %s: %v", mount, err)
	}
	if err := os.MkdirAll(nfs, perm_READWRITEEXECUTE); err != nil {
		log.Fatalf("Creating NFS path %s: %v", nfs, err)
	}
	log.Printf("NFS source: %s", nfsDir)

	if err := os.RemoveAll(ssd); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing mountpoint %s: %v", mount, err)
	}
	if err := os.MkdirAll(ssd, perm_READWRITEEXECUTE); err != nil {
		log.Fatalf("Creating SSD path %s: %v", ssd, err)
	}
	log.Printf("SSD cache: %s", ssdDir)

	_ = os.MkdirAll(
		filepath.Join(nfs, "/project-1"),
		perm_READWRITEEXECUTE)
	_ = os.WriteFile(
		filepath.Join(nfs, "/project-1/main.py"),
		[]byte("# project-1 main.py\nprint('Hello from project-1 main')"),
		perm_READWRITEEXECUTE)
	_ = os.WriteFile(
		filepath.Join(nfs, "/project-1/common-lib.py"),
		[]byte("# common-lib.py in project-1\nprint('Hello from common-lib in project-1')"),
		perm_READWRITEEXECUTE)

	_ = os.MkdirAll(
		filepath.Join(nfs, "/project-2"), perm_READWRITEEXECUTE)
	_ = os.WriteFile(
		filepath.Join(nfs, "/project-2/entrypoint.py"),
		[]byte("# project-2 entrypoint.py\nprint('Hello from project-2 entrypoint')"),
		perm_READWRITEEXECUTE)
	_ = os.WriteFile(
		filepath.Join(nfs, "/project-2/common-lib.py"),
		[]byte("# common-lib.py in project-2\nprint('Hello from common-lib in project-2')"),
		perm_READWRITEEXECUTE)
}
