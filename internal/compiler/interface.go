package compiler

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/deviceix/styx/internal/platform"
)

// Compiler defines the interface for compiler operations
type Compiler interface {
	GetName() string
	GetVersion() string
	Compile(source, output string, flags []string) error
	Link(objects []string, output string, flags []string) error
	Archive(objects []string, output string, flags []string) error

	GetObjectExtension() string
	GetExecutableExtension() string
	GetStaticLibraryExtension() string
	GetSharedLibraryExtension() string
	GetCXXCompilerName() string

	SupportsFlag(flag string) bool
	SupportsLanguage(language string) bool
}

// CompilerType represents the type of compiler
type CompilerType int

const (
	CompilerUnknown CompilerType = iota
	CompilerGCC
	CompilerClang
	CompilerMSVC
)

// compilerRegistry stores available compilers
var compilerRegistry = make(map[string]Compiler)

// RegisterCompiler adds a compiler to the registry
func RegisterCompiler(compiler Compiler) {
	compilerRegistry[strings.ToLower(compiler.GetName())] = compiler
}

// GetCompiler retrieves a compiler by name
func GetCompiler(name string) (Compiler, error) {
	compiler, exists := compilerRegistry[strings.ToLower(name)]
	if !exists {
		return nil, fmt.Errorf("compiler not found: %s", name)
	}
	return compiler, nil
}

// DetectCompilers finds available compilers on the system
func DetectCompilers() []Compiler {
	var compilers []Compiler

	if path, err := exec.LookPath("gcc"); err == nil {
		version := getCompilerVersion(path, "--version")

		compiler := &GCCCompiler{
			Path:     path,
			Version:  version,
			Platform: platform.DetectPlatform(),
		}

		compilers = append(compilers, compiler)
		RegisterCompiler(compiler)
	}

	if path, err := exec.LookPath("clang"); err == nil {
		version := getCompilerVersion(path, "--version")

		compiler := &ClangCompiler{
			Path:     path,
			Version:  version,
			Platform: platform.DetectPlatform(),
		}

		compilers = append(compilers, compiler)
		RegisterCompiler(compiler)
	}

	if platform.DetectPlatform() == platform.PlatformWindows {
		// TODO: impl
	}

	return compilers
}

// getCompilerVersion runs the compiler with version flag and parses the output
func getCompilerVersion(path, versionFlag string) string {
	cmd := exec.Command(path, versionFlag)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return "unknown"
}

// GetDefaultCompiler tries to find a suitable compiler
func GetDefaultCompiler(preferredType string) (Compiler, error) {
	// if users have any preferred compilers
	if preferredType != "" && preferredType != "auto" {
		if compiler, err := GetCompiler(preferredType); err == nil {
			return compiler, nil
		}
	}

	// otherwise detect all available compilers
	compilers := DetectCompilers()
	if len(compilers) == 0 {
		return nil, errors.New("no compilers found on the system")
	}

	// clang is pretty goated so clang comes first
	// submit an issue if you think otherwise
	for _, compilerType := range []string{"clang", "gcc", "msvc"} {
		for _, comp := range compilers {
			if strings.Contains(strings.ToLower(comp.GetName()), compilerType) {
				return comp, nil
			}
		}
	}
	// return first found; it shouldn't reach here anyway
	return compilers[0], nil
}
