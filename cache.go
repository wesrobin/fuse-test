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

	cacheMu *sync.RWMutex
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
