package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the TOML configuration for a Styx project
type Config struct {
	Project      ProjectConfig                `toml:"project"`
	Build        BuildConfig                  `toml:"build"`
	Toolchain    ToolchainConfig              `toml:"toolchain"`
	Targets      map[string]TargetConfig      `toml:"targets"`
	Dependencies map[string]DependencyConfig  `toml:"dependencies"`
	Environment  map[string]EnvironmentConfig `toml:"environment"`
}

// ProjectConfig contains project metadata
type ProjectConfig struct {
	Name     string `toml:"name"`
	Version  string `toml:"version"`
	Language string `toml:"language"`
	Standard string `toml:"standard"`
}

// BuildConfig contains build settings
type BuildConfig struct {
	OutputType    string   `toml:"output_type"`
	OutputName    string   `toml:"output_name"`
	Sources       []string `toml:"sources"`
	IncludeDirs   []string `toml:"include_dirs"`
	Exclude       []string `toml:"exclude"`
	PreBuildCmds  []string `toml:"pre_build_cmds"`
	PostBuildCmds []string `toml:"post_build_cmds"`
}

// ToolchainConfig contains compiler settings
type ToolchainConfig struct {
	Compiler      string   `toml:"compiler"`
	CFlags        []string `toml:"c_flags"`
	CXXFlags      []string `toml:"cxx_flags"`
	LinkerFlags   []string `toml:"linker_flags"`
	ArchiverFlags []string `toml:"archiver_flags"`
}

// TargetConfig contains target-specific build settings
type TargetConfig struct {
	CFlags      []string          `toml:"c_flags"`
	CXXFlags    []string          `toml:"cxx_flags"`
	LinkerFlags []string          `toml:"linker_flags"`
	Env         map[string]string `toml:"env"`
}

// DependencyConfig contains dependency information
type DependencyConfig struct {
	Version string `toml:"version"`
	URL     string `toml:"url"`
	Local   string `toml:"local"`
}

// EnvironmentConfig contains environment-specific settings
type EnvironmentConfig struct {
	Toolchain     string            `toml:"toolchain"`
	OutputDir     string            `toml:"output_dir"`
	BuildFlags    []string          `toml:"build_flags"`
	Env           map[string]string `toml:"env"`
	PreBuildCmds  []string          `toml:"pre_build_cmds"`
	PostBuildCmds []string          `toml:"post_build_cmds"`
}

// ParseFile parses a TOML configuration file
func ParseFile(path string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", path)
	}

	var config Config

	// Parse TOML
	_, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validateConfig checks if the configuration is valid
func validateConfig(config *Config) error {
	// Check required fields
	if config.Project.Name == "" {
		return errors.New("project name is required")
	}

	if config.Build.OutputType == "" {
		return errors.New("build output type is required")
	}

	// Validate output type
	validOutputTypes := map[string]bool{
		"executable": true,
		"static_lib": true,
		"shared_lib": true,
	}

	if !validOutputTypes[config.Build.OutputType] {
		return fmt.Errorf("invalid output type: %s (must be executable, static_lib, or shared_lib)", config.Build.OutputType)
	}

	// If no output name specified, use project name
	if config.Build.OutputName == "" {
		config.Build.OutputName = config.Project.Name
	}

	return nil
}

// LoadConfig attempts to load a configuration file from the given directory
// or from known default locations
func LoadConfig(dir string) (*Config, error) {
	// check specific path first if provided
	if dir != "" {
		if filepath.Ext(dir) == ".toml" {
			return ParseFile(dir)
		}

		// check for styx.toml in the given directory
		candidates := []string{
			filepath.Join(dir, "styx.toml"),
			filepath.Join(dir, "Styx.toml"),
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return ParseFile(candidate)
			}
		}
	}

	// check in current directory
	candidates := []string{
		"styx.toml",
		"Styx.toml",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return ParseFile(candidate)
		}
	}

	return nil, errors.New("no configuration file found")
}
