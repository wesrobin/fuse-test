package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrNotFoundCache = errors.New("not found in cache")
	ErrWontCache     = errors.New("cache refused the file")
)

type Cache interface {
	// Get fetches a file at the given path from the cache.
	// Returns ErrNotFoundCache if the file does not exist.
	Get(path string) ([]byte, error)

	// Put a new file in the cache.
	// Returns ErrWontCache if for whatever reason the cache refused the file.
	// Returns nil error if file is successfully cached.
	Put(path string, data []byte, mode os.FileMode) error
}

func NewDefaultCache(ssdBasePath string) Cache {
	return &defaultCache{
		ssdBasePath: ssdBasePath,
	}
}

type defaultCache struct {
	ssdBasePath string
}

func (d *defaultCache) Get(path string) ([]byte, error) {
	flatPath := flattenDirPath(path)

	cachedData, err := os.ReadFile(filepath.Join(d.ssdBasePath, flatPath))
	if os.IsNotExist(err) {
		// An error other than "file not found" occurred when reading from SSD.
		return nil, ErrNotFoundCache
	} else if err != nil {
		return nil, err
	}

	return cachedData, nil
}

func (d *defaultCache) Put(path string, data []byte, mode os.FileMode) error {
	// Write the file to SSD with the same permissions it has in FUSE/NFS.
	flatPath := flattenDirPath(path)
	fileName := filepath.Join(d.ssdBasePath, flatPath)
	if err := os.WriteFile(fileName, data, mode); err != nil {
		return err
	}

	return nil
}
func NewSizeLimitedCache(ssdBasePath string, byteLimit int64) Cache {
	return &sizeLimitedCache{
		ssdBasePath: ssdBasePath,
		byteLimit:   byteLimit,
		isPresent:   make(map[string]bool),
	}
}

type sizeLimitedCache struct {
	ssdBasePath          string
	byteLimit, byteCount int64

	cacheMu   sync.RWMutex
	isPresent map[string]bool // Just use a map for easy lookup. We'll be fetching the file from ssd
}

func (s *sizeLimitedCache) Get(path string) ([]byte, error) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	flatPath := flattenDirPath(path)

	if !s.isPresent[flatPath] {
		return nil, ErrNotFoundCache
	}

	cachedData, err := os.ReadFile(filepath.Join(s.ssdBasePath, flatPath))
	if os.IsNotExist(err) {
		// An error other than "file not found" occurred when reading from SSD.
		return nil, ErrNotFoundCache
	} else if err != nil {
		return nil, err
	}

	return cachedData, nil
}

// Put will overwrite any existing data. Not great for huge files, but it (currently) isn't called
// before first running a Get.
func (s *sizeLimitedCache) Put(path string, data []byte, mode os.FileMode) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	dataLen := int64(len(data))
	if s.byteCount+dataLen > s.byteLimit {
		return ErrWontCache
	}

	// Write the file to SSD with the same permissions it has in FUSE/NFS.
	flatPath := flattenDirPath(path)
	fileName := filepath.Join(s.ssdBasePath, flatPath)
	if err := os.WriteFile(fileName, data, mode); err != nil {
		return err
	}

	s.isPresent[flatPath] = true
	s.byteCount += dataLen

	return nil
}

func NewLRUCache(path string, capacity int) Cache {
	if capacity == 0 {
		log.Fatalf("FATAL: LRU cache initialised with 0 capacity")
	}
	return &lruCache{
		ssdBasePath: path,
		capacity:    capacity,

		isPresent: make(map[string]bool),
	}
}

type lruCache struct {
	ssdBasePath string
	capacity    int

	cacheMu   sync.RWMutex
	isPresent map[string]bool // Just use a map for easy lookup. We'll be fetching the file from ssd
	queueMu   sync.Mutex
	queue     []string // A doubly-linked list has better performance for write operations, but Go doesn't have good support for one
}

func (lru *lruCache) Get(path string) ([]byte, error) {
	lru.cacheMu.RLock()
	defer lru.cacheMu.RUnlock()

	flatPath := flattenDirPath(path)

	if !lru.isPresent[flatPath] {
		return nil, ErrNotFoundCache
	}

	_ = lru.promote(flatPath) // Not putting anything new in, ignore evicted

	cachedData, err := os.ReadFile(filepath.Join(lru.ssdBasePath, flatPath))
	if os.IsNotExist(err) {
		// An error other than "file not found" occurred when reading from SSD.
		return nil, ErrNotFoundCache
	} else if err != nil {
		return nil, err
	}

	return cachedData, nil
}

func (lru *lruCache) Put(path string, data []byte, mode os.FileMode) error {
	lru.cacheMu.Lock()
	defer lru.cacheMu.Unlock()

	// Write the file to SSD with the same permissions it has in FUSE/NFS.
	flatPath := flattenDirPath(path)
	fileName := filepath.Join(lru.ssdBasePath, flatPath)
	if err := os.WriteFile(fileName, data, mode); err != nil {
		return err
	}

	// Promote or add the new path to the back of the lru
	evicted := lru.promote(flatPath)
	if evicted != nil {
		delete(lru.isPresent, *evicted)
	} else {
		lru.isPresent[flatPath] = true
	}

	return nil
}

// promote updates the key in the queue
// If the key is present in the queue, it will move it to the back (most recently used position).
// If the key is not present in the queue, it will add it to the back.
// If a new key is added and the queue length >= capacity, the front key (least recently used) will
// evicted and returned.
// The returned key will be nil if no key was evicted.
func (lru *lruCache) promote(key string) *string {
	lru.queueMu.Lock()
	defer lru.queueMu.Unlock()

	foundIdx := -1
	for i, k := range lru.queue {
		if k == key {
			foundIdx = i
			break
		}
	}

	if foundIdx != -1 {
		// Remove the key from its current position
		// This operation is O(N) where N is current cache size
		lru.queue = append(lru.queue[:foundIdx], lru.queue[foundIdx+1:]...)
	}
	// Add the key to the back (MRU position)
	lru.queue = append(lru.queue, key)

	var evicted *string
	if len(lru.queue) > lru.capacity {
		// Need to evict
		evictee := lru.queue[0]
		lru.queue = lru.queue[1:]
		evicted = &evictee
	}
	return evicted // nil if none evicted
}
