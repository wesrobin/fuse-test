package main

import (
	"errors"
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

func (s *defaultCache) Get(path string) ([]byte, error) {
	flatPath := flattenDirPath(path)

	cachedData, err := os.ReadFile(filepath.Join(s.ssdBasePath, flatPath))
	if os.IsNotExist(err) {
		// An error other than "file not found" occurred when reading from SSD.
		return nil, ErrNotFoundCache
	} else if err != nil {
		return nil, err
	}

	return cachedData, nil
}

func (s *defaultCache) Put(path string, data []byte, mode os.FileMode) error {
	// Write the file to SSD with the same permissions it has in FUSE/NFS.
	flatPath := flattenDirPath(path)
	fileName := filepath.Join(s.ssdBasePath, flatPath)
	if err := os.WriteFile(fileName, data, mode); err != nil {
		return err
	}

	return nil
}
func NewSizeLimitedCache(ssdBasePath string, byteLimit int64) Cache {
	return &sizeLimitedCache{
		ssdBasePath: ssdBasePath,
		byteLimit:   byteLimit,
		cache:       make(map[string]bool),
	}
}

type sizeLimitedCache struct {
	ssdBasePath          string
	byteLimit, byteCount int64

	cacheMu sync.RWMutex
	cache   map[string]bool // Just use a map for easy lookup. We'll be fetching the file from ssd
}

func (s *sizeLimitedCache) Get(path string) ([]byte, error) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	flatPath := flattenDirPath(path)

	if !s.cache[flatPath] {
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

	s.cache[flatPath] = true
	s.byteCount += dataLen

	return nil
}

func NewLRUCache(path string, queueLen int) Cache {
	return &lruCache{
		ssdBasePath: path,
		queueLimit:  queueLen,

		isPresent: make(map[string]bool),
		// Zero value for queue is fine
	}
}

type lruCache struct {
	ssdBasePath string
	queueLimit  int

	cacheMu   sync.RWMutex
	isPresent map[string]bool // Just use a map for easy lookup. We'll be fetching the file from ssd
	queue     []string
}

func (s *lruCache) Get(path string) ([]byte, error) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	flatPath := flattenDirPath(path)

	if !s.isPresent[flatPath] {
		return nil, ErrNotFoundCache
	}

	// TODO(wes): Update queue

	cachedData, err := os.ReadFile(filepath.Join(s.ssdBasePath, flatPath))
	if os.IsNotExist(err) {
		// An error other than "file not found" occurred when reading from SSD.
		return nil, ErrNotFoundCache
	} else if err != nil {
		return nil, err
	}

	return cachedData, nil
}

func (s *lruCache) Put(path string, data []byte, mode os.FileMode) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// Write the file to SSD with the same permissions it has in FUSE/NFS.
	flatPath := flattenDirPath(path)
	fileName := filepath.Join(s.ssdBasePath, flatPath)
	if err := os.WriteFile(fileName, data, mode); err != nil {
		return err
	}

	// TODO(wes): Update queue

	return nil
}
