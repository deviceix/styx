package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/deviceix/styx/internal/build"
	"github.com/deviceix/styx/internal/compiler"
	"github.com/deviceix/styx/internal/config"
	"github.com/deviceix/styx/internal/platform"
)

var (
	configPath    string
	target        string
	outputDir     string
	verbose       bool
	jobs          int
	showCompilers bool

	version = "0.1.0"
)

// setupLogging configures the logger
func setupLogging(verbose bool) {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}

	log.Logger = zerolog.New(output).With().Timestamp().Logger()
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

// main is the entry point of the application
func main() {
	rootCmd := &cobra.Command{
		Use:   "styx",
		Short: "Styx build system for C/C++ projects",
		Long: `Styx is a modern, lightweight build system for C and C++ projects.
It provides simple configuration, fast incremental builds, and 
supports specialized environments like OSDev and embedded systems.`,
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging(verbose)
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: styx.toml in current directory)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build the project",
		Long:  `Build the project according to the configuration file.`,
		Run: func(cmd *cobra.Command, args []string) {
			runBuild()
		},
	}

	buildCmd.Flags().StringVarP(&target, "target", "t", "", "Build target (e.g., debug, release)")
	buildCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "Output directory")
	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", runtime.NumCPU(), "Number of parallel jobs")
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean build artifacts",
		Long:  `Remove build artifacts and clear build cache.`,
		Run: func(cmd *cobra.Command, args []string) {
			runClean()
		},
	}

	cleanCmd.Flags().StringVarP(&target, "target", "t", "", "Clean specific target (default: all)")
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Build and run the project",
		Long:  `Build and then execute the resulting binary.`,
		Run: func(cmd *cobra.Command, args []string) {
			runBuildAndExecute(args)
		},
	}

	runCmd.Flags().StringVarP(&target, "target", "t", "", "Build target (e.g., debug, release)")
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new project",
		Long:  `Create a new Styx project in the current directory.`,
		Run: func(cmd *cobra.Command, args []string) {
			runInit()
		},
	}

	compilerCmd := &cobra.Command{
		Use:   "compiler",
		Short: "Show compiler information",
		Long:  `Display information about available compilers.`,
		Run: func(cmd *cobra.Command, args []string) {
			showCompilerInfo()
		},
	}

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(compilerCmd)
	if err := rootCmd.Execute(); err != nil {
		_, err := fmt.Fprintln(os.Stderr, err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
}

// loadConfig loads the configuration file
func loadConfig() (*config.Config, error) {
	// use if provided
	if configPath != "" {
		return config.ParseFile(configPath)
	}

	// otherwise try to find configuration file
	cfg, err := config.LoadConfig("")
	if err != nil {
		cfg, err = config.LoadScriptConfig("")
		if err != nil {
			return nil, fmt.Errorf("no configuration file found")
		}
	}

	return cfg, nil
}

// runBuild executes the build process
func runBuild() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	builder, err := build.NewBuilder(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create builder")
	}

	if target != "" {
		if err := builder.SetTarget(target); err != nil {
			log.Fatal().Err(err).Msg("Invalid target")
		}
	}

	if outputDir != "" {
		if err := builder.SetOutputDir(outputDir); err != nil {
			log.Fatal().Err(err).Msg("Invalid output directory")
		}
	}

	builder.SetVerbose(verbose)
	if err := builder.Build(); err != nil {
		log.Fatal().Err(err).Msg("Build failed")
	}
}

// runClean cleans build artifacts
func runClean() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	builder, err := build.NewBuilder(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create builder")
	}

	if target != "" {
		if err := builder.SetTarget(target); err != nil {
			log.Fatal().Err(err).Msg("Invalid target")
		}
	}

	if err := builder.Clean(); err != nil {
		log.Fatal().Err(err).Msg("Clean failed")
	}

	log.Info().Msg("Clean completed successfully")
}

// runBuildAndExecute builds and then runs the executable
func runBuildAndExecute(args []string) {
	runBuild()
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	if cfg.Build.OutputType != "executable" {
		log.Fatal().Msg("Cannot run non-executable output")
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
		log.Fatal().Str("path", exePath).Msg("Executable not found")
	}

	log.Info().Str("executable", exePath).Msg("Running executable")

	cmd := exec.Command(exePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatal().Err(err).Msg("Execution failed")
	}
}

// runInit initializes a new Styx project
func runInit() {
	if _, err := os.Stat("styx.toml"); err == nil {
		log.Fatal().Msg("Project already initialized; styx.toml exists")
	}

	dirs := []string{"src", "include", "build"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal().Err(err).Str("dir", dir).Msg("Failed to create directory")
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get current directory")
	}

	projectName := filepath.Base(wd)
	configContent := fmt.Sprintf(`[project]
name = "%s"
version = "0.1.0"
language = "c++"
standard = "c++17"

[build]
output_type = "executable"
output_name = "%s"
sources = ["src/*.cpp", "src/**/*.cpp"]
include_dirs = ["include"]

[toolchain]
compiler = "auto"
c_flags = ["-Wall", "-Wextra"]
cxx_flags = ["-Wall", "-Wextra"]
linker_flags = []

[targets.debug]
c_flags = ["-g", "-O0"]
cxx_flags = ["-g", "-O0"]

[targets.release]
c_flags = ["-O2", "-DNDEBUG"]
cxx_flags = ["-O2", "-DNDEBUG"]
`, projectName, projectName)

	if err := os.WriteFile("styx.toml", []byte(configContent), 0644); err != nil {
		log.Fatal().Err(err).Msg("Failed to write configuration file")
	}

	mainContent := `#include <iostream>

int main(int argc, char* argv[]) 
{
    std::cout << "Hello from " << argv[0] << "!" << std::endl;
    return 0;
}
`

	if err := os.WriteFile("src/main.cpp", []byte(mainContent), 0644); err != nil {
		log.Fatal().Err(err).Msg("Failed to write main.cpp")
	}

	log.Info().
		Str("project", projectName).
		Msg("Project initialized successfully")
}

// showCompilerInfo displays information about available compilers
func showCompilerInfo() {
	compilers := compiler.DetectCompilers()

	if len(compilers) == 0 {
		log.Fatal().Msg("No compilers found")
	}

	fmt.Println("Available compilers:")
	fmt.Println()

	for i, comp := range compilers {
		fmt.Printf("%d. %s\n", i+1, comp.GetName())
		fmt.Printf("   Version: %s\n", comp.GetVersion())
		fmt.Printf("   Object extension: %s\n", comp.GetObjectExtension())
		fmt.Printf("   Executable extension: %s\n", comp.GetExecutableExtension())
		fmt.Printf("   Static library extension: %s\n", comp.GetStaticLibraryExtension())
		fmt.Printf("   Shared library extension: %s\n", comp.GetSharedLibraryExtension())
		fmt.Println()
	}
}
