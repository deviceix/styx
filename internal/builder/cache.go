package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CacheEntry represents a cached build artifact
type CacheEntry struct {
	Path            string        `json:"path"`
	Hash            string        `json:"hash"`
	Timestamp       int64         `json:"timestamp"`
	Dependencies    []string      `json:"dependencies"`
	CommandHash     string        `json:"command_hash"`
	ObjectFile      string        `json:"object_file"`
	CompilationTime time.Duration `json:"compilation_time"`
}

// BuildCache represents the cache of build artifacts
type BuildCache struct {
	Version       string                 `json:"version"`
	Entries       map[string]*CacheEntry `json:"entries"`
	LastBuildTime time.Time              `json:"last_build_time"`
}

// Cache provides methods to manage the build cache
type Cache struct {
	Path          string
	BuildCache    *BuildCache
	HashAlgorithm string
}

// NewCache creates a new Cache instance
func NewCache(cachePath string) *Cache {
	return &Cache{
		Path:          cachePath,
		HashAlgorithm: "sha256",
	}
}

// Load loads the cache from disk
func (c *Cache) Load() error {
	cacheDir := filepath.Dir(c.Path)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := os.ReadFile(c.Path)
	if err != nil {
		// new cache
		if os.IsNotExist(err) {
			c.BuildCache = &BuildCache{
				Version:       "1.0",
				Entries:       make(map[string]*CacheEntry),
				LastBuildTime: time.Now(),
			}
			return nil
		}
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	// parse JSON
	c.BuildCache = &BuildCache{}
	if err := json.Unmarshal(data, c.BuildCache); err != nil {
		return fmt.Errorf("failed to parse cache file: %w", err)
	}

	return nil
}

// Save serializes & saves the cache to disk
func (c *Cache) Save() error {
	// update last build time
	c.BuildCache.LastBuildTime = time.Now()
	data, err := json.MarshalIndent(c.BuildCache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache: %w", err)
	}

	cacheDir := filepath.Dir(c.Path)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := os.WriteFile(c.Path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// GetEntry retrieves a cache entry by path
func (c *Cache) GetEntry(path string) (*CacheEntry, bool) {
	if c.BuildCache == nil {
		return nil, false
	}

	entry, exists := c.BuildCache.Entries[path]
	return entry, exists
}

// PutEntry adds or updates a cache entry
func (c *Cache) PutEntry(entry *CacheEntry) {
	if c.BuildCache == nil {
		c.BuildCache = &BuildCache{
			Version:       "1.0",
			Entries:       make(map[string]*CacheEntry),
			LastBuildTime: time.Now(),
		}
	}

	c.BuildCache.Entries[entry.Path] = entry
}

// RemoveEntry removes a cache entry
func (c *Cache) RemoveEntry(path string) {
	if c.BuildCache == nil {
		return
	}

	delete(c.BuildCache.Entries, path)
}

// CalculateFileHash computes a hash of the file content
func (c *Cache) CalculateFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil { /* TODO: error handling */
		}
	}(file)

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// CalculateCommandHash computes a hash of the build command
func (c *Cache) CalculateCommandHash(command string, args []string) string {
	hasher := sha256.New()
	hasher.Write([]byte(command))

	for _, arg := range args {
		hasher.Write([]byte(arg))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// NeedsRebuild checks if a file needs to be rebuilt
func (c *Cache) NeedsRebuild(path string, dependencies []string, commandHash string) (bool, error) {
	// Get cache entry
	entry, exists := c.GetEntry(path)
	if !exists {
		return true, nil
	}

	// Check if file exists
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, fmt.Errorf("failed to stat file: %w", err)
	}

	// time-based check
	if fileInfo.ModTime().Unix() != entry.Timestamp {
		return true, nil
	}

	// file hash
	currentHash, err := c.CalculateFileHash(path)
	if err != nil {
		return true, fmt.Errorf("failed to calculate hash: %w", err)
	}

	if currentHash != entry.Hash {
		return true, nil
	}

	if commandHash != entry.CommandHash {
		return true, nil
	}

	// last fallback; note: use something else because this is slow
	for _, depPath := range dependencies {
		depInfo, err := os.Stat(depPath)
		if err != nil {
			return true, nil
		}

		rebuild, err := c.NeedsRebuild(depPath, []string{}, "")
		if err != nil || rebuild {
			return true, err
		}

		// if this dep is newer than the target
		if depInfo.ModTime().Unix() > entry.Timestamp {
			return true, nil
		}
	}

	return false, nil
}

// UpdateEntry updates a cache entry after a successful build
func (c *Cache) UpdateEntry(path string, dependencies []string, commandHash string, objectFile string, compilationTime time.Duration) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	hash, err := c.CalculateFileHash(path)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	entry := &CacheEntry{
		Path:            path,
		Hash:            hash,
		Timestamp:       fileInfo.ModTime().Unix(),
		Dependencies:    dependencies,
		CommandHash:     commandHash,
		ObjectFile:      objectFile,
		CompilationTime: compilationTime,
	}

	c.PutEntry(entry)
	return nil
}

// Clean removes entries for files that no longer exist
func (c *Cache) Clean() {
	if c.BuildCache == nil {
		return
	}

	for path := range c.BuildCache.Entries {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			c.RemoveEntry(path)
		}
	}
}
