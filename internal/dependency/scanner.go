package dependency

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DependencyScanner scans source files for dependencies
type DependencyScanner struct {
	includeDirs     []string
	visitedFiles    map[string]bool
	systemIncludeRe *regexp.Regexp
	localIncludeRe  *regexp.Regexp
}

// NewDependencyScanner creates a new DependencyScanner with the given include directories
func NewDependencyScanner(includeDirs []string) *DependencyScanner {
	return &DependencyScanner{
		includeDirs:     includeDirs,
		visitedFiles:    make(map[string]bool),
		systemIncludeRe: regexp.MustCompile(`#include\s*<([^>]+)>`),
		localIncludeRe:  regexp.MustCompile(`#include\s*"([^"]+)"`),
	}
}

// Scan scans a source file for dependencies
func (s *DependencyScanner) Scan(sourceFile string) ([]string, error) {
	s.visitedFiles = make(map[string]bool)

	dependencies := make(map[string]bool)
	if err := s.scanRecursive(sourceFile, dependencies); err != nil {
		return nil, err
	}

	var result []string
	for dep := range dependencies {
		result = append(result, dep)
	}

	return result, nil
}

// scanRecursive recursively scans for dependencies with visitor pattern
func (s *DependencyScanner) scanRecursive(sourceFile string, dependencies map[string]bool) error {
	if s.visitedFiles[sourceFile] {
		return nil
	}

	s.visitedFiles[sourceFile] = true
	file, err := os.Open(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil { /* TODO */
		}
	}(file)

	// get dir of current file for resolving relative includes
	sourceDir := filepath.Dir(sourceFile)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// get local includes first
		if matches := s.localIncludeRe.FindStringSubmatch(line); len(matches) > 1 {
			includePath := matches[1]
			// relative path check
			resolvedPath := filepath.Join(sourceDir, includePath)
			if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
				found := false
				for _, dir := range s.includeDirs {
					tryPath := filepath.Join(dir, includePath)
					if _, err := os.Stat(tryPath); err == nil {
						resolvedPath = tryPath
						found = true
						break
					}
				}

				if !found {
					// just store the raw include path and move on
					continue
				}
			}

			// add as deps
			dependencies[resolvedPath] = true
			if err := s.scanRecursive(resolvedPath, dependencies); err != nil {
				return err
			}
		}

		// system includes (#include <file.h>)
		if matches := s.systemIncludeRe.FindStringSubmatch(line); len(matches) > 1 {
			includePath := matches[1]
			found := false
			for _, dir := range s.includeDirs {
				tryPath := filepath.Join(dir, includePath)
				if _, err := os.Stat(tryPath); err == nil {
					dependencies[tryPath] = true
					if err := s.scanRecursive(tryPath, dependencies); err != nil {
						return err
					}

					found = true
					break
				}
			}

			// if the system header isn't found, that's usually okay
			// it's probably a standard library header or root
			if !found {
				continue
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning file: %w", err)
	}

	return nil
}

// FindSourceFiles finds all source files matching the given patterns
func FindSourceFiles(patterns []string, excludePatterns []string) ([]string, error) {
	var sourceFiles []string

	// Process each pattern
	for _, pattern := range patterns {
		// absolute path case
		if filepath.IsAbs(pattern) {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("failed to expand glob pattern %s: %w", pattern, err)
			}
			sourceFiles = append(sourceFiles, matches...)
			continue
		}

		if strings.Contains(pattern, string(os.PathSeparator)) {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("failed to expand glob pattern %s: %w", pattern, err)
			}
			sourceFiles = append(sourceFiles, matches...)
			continue
		}

		if strings.Contains(pattern, "**") {
			parts := strings.Split(pattern, "**")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid recursive pattern: %s", pattern)
			}

			baseDir := parts[0]
			suffix := parts[1]
			err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() && strings.HasSuffix(path, suffix) {
					sourceFiles = append(sourceFiles, path)
				}

				return nil
			})

			if err != nil {
				return nil, fmt.Errorf("failed to walk directory tree: %w", err)
			}

			continue
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to expand glob pattern %s: %w", pattern, err)
		}
		sourceFiles = append(sourceFiles, matches...)
	}

	// exclude
	if len(excludePatterns) > 0 {
		var filteredFiles []string

		for _, file := range sourceFiles {
			excluded := false

			for _, excludePattern := range excludePatterns {
				matched, err := filepath.Match(excludePattern, filepath.Base(file))
				if err != nil {
					return nil, fmt.Errorf("invalid exclude pattern %s: %w", excludePattern, err)
				}

				if matched || strings.Contains(file, excludePattern) {
					excluded = true
					break
				}
			}

			if !excluded {
				filteredFiles = append(filteredFiles, file)
			}
		}

		sourceFiles = filteredFiles
	}

	// delete duped files from pattern matching
	seen := make(map[string]bool)
	var result []string
	for _, file := range sourceFiles {
		if !seen[file] {
			seen[file] = true
			result = append(result, file)
		}
	}

	return result, nil
}
