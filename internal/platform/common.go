package platform

import (
	"runtime"
)

// Platform represents a supported operating system
type Platform int

const (
	PlatformUnknown Platform = iota
	PlatformWindows
	PlatformLinux
	PlatformMacOS
)

// PlatformInfo contains platform-specific configuration
type PlatformInfo struct {
	Platform           Platform
	Name               string
	ObjExtension       string
	ExeExtension       string
	StaticLibExtension string
	SharedLibExtension string
	PathSeparator      string
}

// DetectPlatform determines the current platform
func DetectPlatform() Platform {
	switch runtime.GOOS {
	case "windows":
		return PlatformWindows
	case "linux":
		return PlatformLinux
	case "darwin":
		return PlatformMacOS
	default:
		return PlatformUnknown
	}
}

// GetPlatformInfo returns info about the current platform
// note: defaults to UNIX-like OS as a fallback
func GetPlatformInfo() *PlatformInfo {
	platform := DetectPlatform()

	switch platform {
	case PlatformWindows:
		return &PlatformInfo{
			Platform:           PlatformWindows,
			Name:               "windows",
			ObjExtension:       ".obj",
			ExeExtension:       ".exe",
			StaticLibExtension: ".lib",
			SharedLibExtension: ".dll",
			PathSeparator:      "\\",
		}
	case PlatformLinux:
		return &PlatformInfo{
			Platform:           PlatformLinux,
			Name:               "linux",
			ObjExtension:       ".o",
			ExeExtension:       "",
			StaticLibExtension: ".a",
			SharedLibExtension: ".so",
			PathSeparator:      "/",
		}
	case PlatformMacOS:
		return &PlatformInfo{
			Platform:           PlatformMacOS,
			Name:               "macos",
			ObjExtension:       ".o",
			ExeExtension:       "",
			StaticLibExtension: ".a",
			SharedLibExtension: ".dylib",
			PathSeparator:      "/",
		}
	default:
		return &PlatformInfo{
			Platform:           PlatformUnknown,
			Name:               "unknown",
			ObjExtension:       ".o",
			ExeExtension:       "",
			StaticLibExtension: ".a",
			SharedLibExtension: ".so",
			PathSeparator:      "/",
		}
	}
}

// IsUnixLike returns true if the platform is Unix-like
func IsUnixLike(platform Platform) bool {
	return platform == PlatformLinux || platform == PlatformMacOS
}
