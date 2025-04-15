package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ScriptParser parses a custom DSL script configuration
type ScriptParser struct {
	content      string
	config       *Config
	currentBlock string
}

// ParseScript parses a configuration script file
func ParseScript(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("script file not found: %s", path)
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}

	parser := &ScriptParser{
		content: string(content),
		config: &Config{
			Project:      ProjectConfig{},
			Build:        BuildConfig{},
			Toolchain:    ToolchainConfig{},
			Targets:      make(map[string]TargetConfig),
			Dependencies: make(map[string]DependencyConfig),
			Environment:  make(map[string]EnvironmentConfig),
		},
	}

	if err := parser.parse(); err != nil {
		return nil, err
	}

	if err := validateConfig(parser.config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return parser.config, nil
}

// parse processes the script content
func (p *ScriptParser) parse() error {
	lines := strings.Split(p.content, "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if err := p.parseLine(line, lineNum+1); err != nil {
			return fmt.Errorf("line %d: %w", lineNum+1, err)
		}
	}

	return nil
}

// parseLine handles a single line of script
func (p *ScriptParser) parseLine(line string, lineNum int) error {
	// Project declaration
	if match := regexp.MustCompile(`Project\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*\)`).FindStringSubmatch(line); match != nil {
		p.config.Project.Name = match[1]
		p.config.Project.Version = match[2]
		return nil
	}

	if match := regexp.MustCompile(`Language\s*\(\s*"([^"]+)"(?:\s*,\s*"([^"]+)")?\s*\)`).FindStringSubmatch(line); match != nil {
		p.config.Project.Language = match[1]
		if len(match) > 2 && match[2] != "" {
			p.config.Project.Standard = match[2]
		}
		return nil
	}

	for _, outputType := range []string{"Executable", "StaticLib", "SharedLib"} {
		pattern := fmt.Sprintf(`%s\s*\(\s*"([^"]+)"\s*,\s*\[\s*(.*?)\s*\]\s*\)`, outputType)
		if match := regexp.MustCompile(pattern).FindStringSubmatch(line); match != nil {
			p.config.Build.OutputName = match[1]

			switch outputType {
			case "Executable":
				p.config.Build.OutputType = "executable"
			case "StaticLib":
				p.config.Build.OutputType = "static_lib"
			case "SharedLib":
				p.config.Build.OutputType = "shared_lib"
			}

			if err := p.parseBuildBlock(match[2]); err != nil {
				return err
			}

			return nil
		}
	}
	// toolchains
	// compiler
	if match := regexp.MustCompile(`Compiler\s*\(\s*"([^"]+)"\s*\)`).FindStringSubmatch(line); match != nil {
		p.config.Toolchain.Compiler = match[1]
		return nil
	}

	// c-flags
	if match := regexp.MustCompile(`Flags\s*\(\s*(.*?)\s*\)`).FindStringSubmatch(line); match != nil {
		flags, err := parseStringList(match[1])
		if err != nil {
			return err
		}
		// add to global
		p.config.Toolchain.CFlags = append(p.config.Toolchain.CFlags, flags...)
		p.config.Toolchain.CXXFlags = append(p.config.Toolchain.CXXFlags, flags...)
		return nil
	}

	if match := regexp.MustCompile(`Target\s*\(\s*"([^"]+)"\s*,\s*\[\s*(.*?)\s*\]\s*\)`).FindStringSubmatch(line); match != nil {
		targetName := match[1]
		target := TargetConfig{
			Env: make(map[string]string),
		}

		if err := p.parseTargetBlock(match[2], &target); err != nil {
			return err
		}

		p.config.Targets[targetName] = target
		return nil
	}

	return fmt.Errorf("unrecognized statement: %s", line)
}

// parseBuildBlock processes the contents of a build block
func (p *ScriptParser) parseBuildBlock(content string) error {
	items := extractBlockItems(content)

	for _, item := range items {
		if match := regexp.MustCompile(`Sources\s*\(\s*(.*?)\s*\)`).FindStringSubmatch(item); match != nil {
			sources, err := parseStringList(match[1])
			if err != nil {
				return err
			}
			p.config.Build.Sources = append(p.config.Build.Sources, sources...)
			continue
		}

		if match := regexp.MustCompile(`Exclude\s*\(\s*(.*?)\s*\)`).FindStringSubmatch(item); match != nil {
			exclude, err := parseStringList(match[1])
			if err != nil {
				return err
			}
			p.config.Build.Exclude = append(p.config.Build.Exclude, exclude...)
			continue
		}

		if match := regexp.MustCompile(`IncludeDirs\s*\(\s*(.*?)\s*\)`).FindStringSubmatch(item); match != nil {
			includeDirs, err := parseStringList(match[1])
			if err != nil {
				return err
			}
			p.config.Build.IncludeDirs = append(p.config.Build.IncludeDirs, includeDirs...)
			continue
		}

		return fmt.Errorf("unrecognized build item: %s", item)
	}

	return nil
}

// parseTargetBlock processes the contents of a target block
func (p *ScriptParser) parseTargetBlock(content string, target *TargetConfig) error {
	items := extractBlockItems(content)
	for _, item := range items {
		if match := regexp.MustCompile(`Flags\s*\(\s*(.*?)\s*\)`).FindStringSubmatch(item); match != nil {
			flags, err := parseStringList(match[1])
			if err != nil {
				return err
			}
			target.CFlags = append(target.CFlags, flags...)
			target.CXXFlags = append(target.CXXFlags, flags...)
			continue
		}

		return fmt.Errorf("unrecognized target item: %s", item)
	}

	return nil
}

// parseStringList converts a list of quoted strings to a string slice
func parseStringList(content string) ([]string, error) {
	var result []string

	pattern := `"([^"]+)"`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			result = append(result, match[1])
		}
	}

	return result, nil
}

// extractBlockItems splits a block content into individual items
// note: this handles comma-separated items
func extractBlockItems(content string) []string {
	var items []string

	// split by commas, but respect nested blocks
	var currentItem strings.Builder
	depth := 0
	for _, char := range content {
		switch char {
		case '[', '(':
			depth++
			currentItem.WriteRune(char)
		case ']', ')':
			depth--
			currentItem.WriteRune(char)
		case ',':
			if depth == 0 {
				items = append(items, strings.TrimSpace(currentItem.String()))
				currentItem.Reset()
			} else {
				currentItem.WriteRune(char)
			}
		default:
			currentItem.WriteRune(char)
		}
	}

	// append last if not empty
	if currentItem.Len() > 0 {
		items = append(items, strings.TrimSpace(currentItem.String()))
	}

	return items
}

// LoadScriptConfig attempts to load a script configuration file
func LoadScriptConfig(dir string) (*Config, error) {
	// check specific path first if provided
	if dir != "" {
		if filepath.Ext(dir) == ".script" {
			// if path is directly to a script file
			return ParseScript(dir)
		}

		// check for styx.script in `dir`
		candidates := []string{
			filepath.Join(dir, "styx.script"),
			filepath.Join(dir, "Styx.script"),
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return ParseScript(candidate)
			}
		}
	}

	// check current directory
	candidates := []string{
		"styx.script",
		"Styx.script",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return ParseScript(candidate)
		}
	}

	return nil, errors.New("no script configuration file found")
}
