package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/deviceix/styx/internal/platform"
)

// GCCCompiler implements the Compiler interface for GCC
type GCCCompiler struct {
	Path         string
	Version      string
	Platform     platform.Platform
	TargetTriple string // ttt for cross-compiling
}

// GetName returns the compiler name
func (c *GCCCompiler) GetName() string {
	return "GCC"
}

// GetVersion returns the compiler version
func (c *GCCCompiler) GetVersion() string {
	return c.Version
}

// Compile compiles a source file into an object file
func (c *GCCCompiler) Compile(source, output string, flags []string) error {
	args := append([]string{"-c", source, "-o", output}, flags...)

	cmd := exec.Command(c.Path, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// would capturing error messages then parse it be possible here?
	// as a feature because c++ template errors suck
	return cmd.Run()
}

func (c *GCCCompiler) GetCXXCompilerName() string {
	return "g++"
}

// Link links object files into an executable
func (c *GCCCompiler) Link(objects []string, output string, flags []string) error {
	args := append(objects, "-o", output)
	args = append(args, flags...)

	cmd := exec.Command(c.Path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Archive creates a static library from object files
func (c *GCCCompiler) Archive(objects []string, output string, flags []string) error {
	// GCC doesn't archive directly, use ar instead
	arPath, err := exec.LookPath("ar")
	if err != nil {
		return fmt.Errorf("ar not found: %w", err)
	}

	defaultFlags := []string{"rcs", output}
	args := append(defaultFlags, objects...)
	if len(flags) > 0 {
		args = append(flags, args...)
	}

	cmd := exec.Command(arPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetObjectExtension returns the file extension for object files
func (c *GCCCompiler) GetObjectExtension() string {
	if c.Platform == platform.PlatformWindows {
		return ".o" // gcc uses .o even on Windows
	}
	return ".o"
}

// GetExecutableExtension returns the file extension for executables
func (c *GCCCompiler) GetExecutableExtension() string {
	if c.Platform == platform.PlatformWindows {
		return ".exe"
	}
	return ""
}

// GetStaticLibraryExtension returns the file extension for static libraries
func (c *GCCCompiler) GetStaticLibraryExtension() string {
	if c.Platform == platform.PlatformWindows {
		return ".a" // gcc uses .a even on Windows
	}
	return ".a"
}

// GetSharedLibraryExtension returns the file extension for shared libraries
func (c *GCCCompiler) GetSharedLibraryExtension() string {
	switch c.Platform {
	case platform.PlatformWindows:
		return ".dll"
	case platform.PlatformMacOS:
		return ".dylib"
	default:
		return ".so"
	}
}

// SupportsFlag checks if the compiler supports a specific flag
func (c *GCCCompiler) SupportsFlag(flag string) bool {
	cmd := exec.Command(c.Path, "-Werror", "-fsyntax-only", "-c", "-", "-o", os.DevNull, flag)
	cmd.Stdin = strings.NewReader("int main() { return 0; }")

	err := cmd.Run()
	return err == nil
}

// SupportsLanguage checks if the compiler supports a specific language
func (c *GCCCompiler) SupportsLanguage(language string) bool {
	language = strings.ToLower(language)

	switch language {
	case "c":
		// Check for C support
		return true
	case "c++":
		_, err := exec.LookPath("g++")
		return err == nil
	case "objective-c":
		return c.SupportsFlag("-ObjC")
	default:
		return false
	}
}

// NewGCCCompiler creates a new GCC compiler instance
func NewGCCCompiler(path string, targetTriple string) (*GCCCompiler, error) {
	if path == "" {
		// find in $PATH
		var err error
		path, err = exec.LookPath("gcc")
		if err != nil {
			return nil, fmt.Errorf("gcc not found: %w", err)
		}
	}

	version := getCompilerVersion(path, "--version")
	return &GCCCompiler{
		Path:         path,
		Version:      version,
		Platform:     platform.DetectPlatform(),
		TargetTriple: targetTriple,
	}, nil
}
