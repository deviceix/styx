package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/deviceix/styx/internal/builder"
	"github.com/deviceix/styx/internal/compiler"
	"github.com/deviceix/styx/internal/config"
	"github.com/deviceix/styx/internal/logger"
	"github.com/deviceix/styx/internal/platform"
)

var (
	configPath string
	target     string
	outputDir  string
	verbose    bool
	jobs       int
	log        *logger.Logger

	version = "0.1.0"
)

// setupLogging configures the logger
func setupLogging(verbose bool) {
	log = logger.New(verbose)
}

// main is the entry point of the application
func main() {
	// Initialize logger early to prevent nil pointer errors
	log = logger.New(false)

	rootCmd := &cobra.Command{
		Use:   "styx",
		Short: "Styx build system for C/C++ projects",
		Long: `Styx is a modern, lightweight build system for C and C++ projects.
it provides simple configuration, fast incremental builds, and 
supports specialized environments like OSDev and embedded systems.`,
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging(verbose)
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to configuration file (default: styx.toml in current directory)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "build the project",
		Long:  `build the project according to the configuration file.`,
		Run: func(cmd *cobra.Command, args []string) {
			runBuild()
		},
	}

	buildCmd.Flags().StringVarP(&target, "target", "t", "", "build target (e.g., debug, release)")
	buildCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "output directory")
	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", runtime.NumCPU(), "number of parallel jobs")
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "clean build artifacts",
		Long:  `remove build artifacts and clear build cache.`,
		Run: func(cmd *cobra.Command, args []string) {
			runClean()
		},
	}

	cleanCmd.Flags().StringVarP(&target, "target", "t", "", "Clean specific target (default: all)")
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "build and run the project",
		Long:  `build and then execute the resulting binary.`,
		Run: func(cmd *cobra.Command, args []string) {
			runBuildAndExecute(args)
		},
	}

	runCmd.Flags().StringVarP(&target, "target", "t", "", "build target (e.g., debug, release)")
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "initialize a new project",
		Long:  `create a new Styx project in the current directory.`,
		Run: func(cmd *cobra.Command, args []string) {
			runInit()
		},
	}

	compilerCmd := &cobra.Command{
		Use:   "compiler",
		Short: "show compiler information",
		Long:  `display information about available compilers.`,
		Run: func(cmd *cobra.Command, args []string) {
			showCompilerInfo()
		},
	}

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(compilerCmd)
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig loads the configuration file
func loadConfig() (*config.Config, error) {
	// use if provided
	if configPath != "" {
		log.Info("using configuration file: %s", configPath)
		return config.ParseFile(configPath)
	}

	// otherwise try to find configuration file
	log.Info("searching for configuration file...")
	cfg, err := config.LoadConfig("")
	if err != nil {
		log.Info("no TOML configuration found, trying script configuration...")
		cfg, err = config.LoadScriptConfig("")
		if err != nil {
			return nil, fmt.Errorf("no configuration file found")
		}
		log.Success("found script configuration")
	} else {
		log.Success("found TOML configuration")
	}

	return cfg, nil
}

// runBuild executes the build process
func runBuild() {
	log.Info("loading project configuration...")
	cfg, err := loadConfig()
	if err != nil {
		log.Error("failed to load configuration: %v", err)
		os.Exit(1)
	}

	log.Info("creating builder...")
	b, err := builder.NewBuilder(cfg)
	if err != nil {
		log.Error("failed to create builder: %v", err)
		os.Exit(1)
	}

	if target != "" {
		log.Info("setting target: %s", target)
		if err := b.SetTarget(target); err != nil {
			log.Error("invalid target: %v", err)
			os.Exit(1)
		}
	}

	if outputDir != "" {
		log.Info("setting output directory: %s", outputDir)
		if err := b.SetOutputDir(outputDir); err != nil {
			log.Error("invalid output directory: %v", err)
			os.Exit(1)
		}
	}

	b.SetVerbose(verbose)
	start := time.Now()
	if err := b.Build(); err != nil {
		log.Error("build failed: %v", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	log.Success("build completed in %.2f seconds", duration.Seconds())
}

// runClean cleans build artifacts
func runClean() {
	log.Info("loading project configuration...")
	cfg, err := loadConfig()
	if err != nil {
		log.Error("failed to load configuration: %v", err)
		os.Exit(1)
	}

	log.Info("creating builder...")
	b, err := builder.NewBuilder(cfg)
	if err != nil {
		log.Error("Failed to create builder: %v", err)
		os.Exit(1)
	}

	if target != "" {
		log.Info("setting target: %s", target)
		if err := b.SetTarget(target); err != nil {
			log.Error("invalid target: %v", err)
			os.Exit(1)
		}
	}

	if err := b.Clean(); err != nil {
		log.Error("clean failed: %v", err)
		os.Exit(1)
	}

	log.Success("clean completed successfully")
}

// runBuildAndExecute builds and then runs the executable
func runBuildAndExecute(args []string) {
	runBuild()

	log.Info("loading project configuration...")
	cfg, err := loadConfig()
	if err != nil {
		log.Error("failed to load configuration: %v", err)
		os.Exit(1)
	}

	if cfg.Build.OutputType != "executable" {
		log.Error("cannot run non-executable output")
		os.Exit(1)
	}

	targetDir := "build"
	if outputDir != "" {
		targetDir = outputDir
	}

	if target == "" {
		target = "debug" // default target build type
	}

	targetDir = filepath.Join(targetDir, target)

	outputName := cfg.Build.OutputName
	if outputName == "" {
		outputName = cfg.Project.Name
	}

	platformInfo := platform.GetPlatformInfo()
	exePath := filepath.Join(targetDir, outputName+platformInfo.ExeExtension)
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		log.Error("executable not found: %s", exePath)
		os.Exit(1)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Error("execution failed: %v", err)
		os.Exit(1)
	}
}

// runInit initializes a new Styx project
func runInit() {
	if _, err := os.Stat("styx.toml"); err == nil {
		log.Error("project already initialized; styx.toml exists")
		os.Exit(1)
	}

	log.Info("creating project directories...")
	dirs := []string{"src", "include", "build"}
	for _, dir := range dirs {
		log.Info("creating directory: %s", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Error("failed to create directory %s: %v", dir, err)
			os.Exit(1)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Error("failed to get current directory: %v", err)
		os.Exit(1)
	}

	projectName := filepath.Base(wd)
	log.Info("project name: %s", projectName)

	log.Info("creating configuration file...")
	configContent := fmt.Sprintf(`[project]
name = "%s"
version = "0.1.0"
language = "c++"
standard = "c++23"

[build]
output_type = "executable"
output_name = "%s"
sources = [ "src/*.cpp", "src/**/*.cpp" ]
include_dirs = [ "include" ]

[toolchain]
compiler = "auto"
c_flags = [ "-Wall", "-Wextra" ]
cxx_flags = [ "-Wall", "-Wextra" ]
linker_flags = []

[targets.debug]
c_flags = ["-g", "-O0"]
cxx_flags = ["-g", "-O0"]

[targets.release]
c_flags = ["-O2", "-DNDEBUG"]
cxx_flags = ["-O2", "-DNDEBUG"]
`, projectName, projectName)

	if err := os.WriteFile("styx.toml", []byte(configContent), 0644); err != nil {
		log.Error("failed to write configuration file: %v", err)
		os.Exit(1)
	}
	log.Success("created styx.toml")

	log.Info("creating main.cpp...")
	mainContent := `#include <iostream>

int main(int argc, char* argv[]) 
{
    std::cout << "Hello from " << argv[0] << "!" << std::endl;
    return 0;
}
`

	if err := os.WriteFile("src/main.cpp", []byte(mainContent), 0644); err != nil {
		log.Error("Failed to write main.cpp: %v", err)
		os.Exit(1)
	}
	log.Success("created src/main.cpp")

	log.Success("project %s initialized successfully", projectName)
	log.Note("run 'styx build' to build the project")
	log.Note("run 'styx run' to build and run the project")
}

// showCompilerInfo displays information about available compilers
func showCompilerInfo() {
	log.Info("detecting available compilers...")

	compilers := compiler.DetectCompilers()

	if len(compilers) == 0 {
		log.Error("no compilers found")
		os.Exit(1)
	}

	log.Success("found %d compiler(s)", len(compilers))
	for i, comp := range compilers {
		log.Info("compiler #%d: %s", i+1, comp.GetName())
		log.Note("  version: %s", comp.GetVersion())
		log.Note("  object extension: %s", comp.GetObjectExtension())
		log.Note("  executable extension: %s", comp.GetExecutableExtension())
		log.Note("  static library extension: %s", comp.GetStaticLibraryExtension())
		log.Note("  shared library extension: %s", comp.GetSharedLibraryExtension())
	}
}
