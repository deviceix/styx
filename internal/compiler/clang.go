package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/deviceix/styx/internal/platform"
)

// ClangCompiler implements the Compiler interface for Clang
type ClangCompiler struct {
	Path         string
	Version      string
	Platform     platform.Platform
	TargetTriple string
}

// GetName returns the compiler name
func (c *ClangCompiler) GetName() string {
	return "Clang"
}

// GetVersion returns the compiler version
func (c *ClangCompiler) GetVersion() string {
	return c.Version
}

// Compile compiles a source file into an object file
func (c *ClangCompiler) Compile(source, output string, flags []string) error {
	args := append([]string{"-c", source, "-o", output}, flags...)
	if c.TargetTriple != "" {
		args = append([]string{"-target", c.TargetTriple}, args...)
	}

	cmd := exec.Command(c.Path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// would capturing error messages then parse it be possible here?
	// as a feature because c++ template errors suck
	return cmd.Run()
}

func (c *ClangCompiler) GetCXXCompilerName() string {
	return "clang++"
}

// Link links object files into an executable
func (c *ClangCompiler) Link(objects []string, output string, flags []string) error {
	args := append(objects, "-o", output)
	args = append(args, flags...)
	if c.TargetTriple != "" {
		args = append([]string{"-target", c.TargetTriple}, args...)
	}

	cmd := exec.Command(c.Path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Archive creates a static library from object files
func (c *ClangCompiler) Archive(objects []string, output string, flags []string) error {
	arPath, err := exec.LookPath("llvm-ar")
	if err != nil {
		arPath, err = exec.LookPath("ar")
		if err != nil {
			return fmt.Errorf("ar not found: %w", err)
		}
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
func (c *ClangCompiler) GetObjectExtension() string {
	return ".o"
}

// GetExecutableExtension returns the file extension for executables
func (c *ClangCompiler) GetExecutableExtension() string {
	if c.Platform == platform.PlatformWindows {
		return ".exe"
	}
	return ""
}

// GetStaticLibraryExtension returns the file extension for static libraries
func (c *ClangCompiler) GetStaticLibraryExtension() string {
	return ".a"
}

// GetSharedLibraryExtension returns the file extension for shared libraries
func (c *ClangCompiler) GetSharedLibraryExtension() string {
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
func (c *ClangCompiler) SupportsFlag(flag string) bool {
	cmd := exec.Command(c.Path, "-Werror", "-fsyntax-only", "-xc", "-", flag)
	cmd.Stdin = strings.NewReader("int main() { return 0; }")

	err := cmd.Run()
	return err == nil
}

// SupportsLanguage checks if the compiler supports a specific language
func (c *ClangCompiler) SupportsLanguage(language string) bool {
	language = strings.ToLower(language)

	switch language {
	case "c":
		return true
	case "c++":
		return true // Clang supports C++ out of the box
	case "objective-c":
		return true // Clang has excellent Objective-C support
	default:
		return false
	}
}

// NewClangCompiler creates a new Clang compiler instance
func NewClangCompiler(path string, targetTriple string) (*ClangCompiler, error) {
	if path == "" {
		// try finding Clang in $PATH
		var err error
		path, err = exec.LookPath("clang")
		if err != nil {
			return nil, fmt.Errorf("clang not found: %w", err)
		}
	}

	version := getCompilerVersion(path, "--version")
	return &ClangCompiler{
		Path:         path,
		Version:      version,
		Platform:     platform.DetectPlatform(),
		TargetTriple: targetTriple,
	}, nil
}
