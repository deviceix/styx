package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deviceix/styx/internal/compiler"
	"github.com/deviceix/styx/internal/config"
	"github.com/deviceix/styx/internal/dependency"
	"github.com/deviceix/styx/internal/logger"
	"github.com/deviceix/styx/internal/platform"
)

// Builder is responsible for the build process
type Builder struct {
	Config       *config.Config
	Compiler     compiler.Compiler
	Scanner      *dependency.DependencyScanner
	Graph        *dependency.Graph
	Cache        *Cache
	Executor     *Executor
	Target       string
	OutputDir    string
	Verbose      bool
	HasCppFiles  bool
	platformInfo *platform.PlatformInfo
	logger       *logger.Logger
}

// NewBuilder creates a new builder for the given configuration
func NewBuilder(cfg *config.Config) (*Builder, error) {
	outputDir := "build"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}

	cacheDir := filepath.Join(".styx", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	platformInfo := platform.GetPlatformInfo()
	compilerName := cfg.Toolchain.Compiler
	if compilerName == "" || compilerName == "auto" {
		comp, err := compiler.GetDefaultCompiler("")
		if err != nil {
			return nil, fmt.Errorf("failed to find a suitable compiler: %w", err)
		}
		compilerName = comp.GetName()
	}

	comp, err := compiler.GetCompiler(compilerName)
	if err != nil {
		// try again, one more time
		compiler.DetectCompilers()
		comp, err = compiler.GetCompiler(compilerName)
		if err != nil {
			return nil, fmt.Errorf("compiler not found: %s", compilerName)
		}
	}

	scanner := dependency.NewDependencyScanner(cfg.Build.IncludeDirs)
	log := logger.New(false)
	cache := NewCache(filepath.Join(cacheDir, "build.json"))
	if err := cache.Load(); err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	// `workerCount` 0 means use all available
	executor := NewExecutor(0)
	executor.SetLogger(log)
	return &Builder{
		Config:       cfg,
		Compiler:     comp,
		Scanner:      scanner,
		Graph:        dependency.NewGraph(),
		Cache:        cache,
		Executor:     executor,
		Target:       "debug", // default to debug
		OutputDir:    outputDir,
		platformInfo: platformInfo,
		logger:       log, // Set the logger
	}, nil
}

// SetVerbose sets verbose output mode
func (b *Builder) SetVerbose(verbose bool) {
	b.Verbose = verbose
	// Update the logger with the new verbosity setting
	b.logger = logger.New(verbose)
}

// SetTarget sets the build target
func (b *Builder) SetTarget(target string) error {
	if target == "" {
		b.Target = "debug"
		return nil
	}

	if _, exists := b.Config.Targets[target]; !exists {
		return fmt.Errorf("target not found: %s", target)
	}

	b.Target = target
	return nil
}

// SetOutputDir sets the output directory
func (b *Builder) SetOutputDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("output directory cannot be empty")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	b.OutputDir = dir
	return nil
}

// Build performs the build process
func (b *Builder) Build() error {
	b.logger.Info("starting build for target: %s", b.Target)
	b.logger.Info("project: %s (version %s)", b.Config.Project.Name, b.Config.Project.Version)
	b.logger.Info("compiler: %s", b.Compiler.GetName())

	startTime := time.Now()
	targetOutputDir := filepath.Join(b.OutputDir, b.Target)
	if err := os.MkdirAll(targetOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create target output directory: %w", err)
	}

	if err := b.executePreBuildCommands(); err != nil {
		return fmt.Errorf("pre-build commands failed: %w", err)
	}
	b.logger.Info("finding source files...")
	sourceFiles, err := dependency.FindSourceFiles(b.Config.Build.Sources, b.Config.Build.Exclude)
	if err != nil {
		return fmt.Errorf("failed to find source files: %w", err)
	}

	if len(sourceFiles) == 0 {
		b.logger.Warning("no source files found. Check your sources configuration.")
		return fmt.Errorf("no source files found")
	}

	b.logger.Info("found %d source files", len(sourceFiles))
	if err := b.buildDependencyGraph(sourceFiles); err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	b.Executor.Start()
	defer b.Executor.Shutdown()

	b.logger.Info("compiling source files...")
	objectFiles, err := b.scheduleCompilationTasks(sourceFiles, targetOutputDir)
	if err != nil {
		return fmt.Errorf("failed to compile source files: %w", err)
	}

	outputPath := b.getOutputPath(targetOutputDir)
	switch b.Config.Build.OutputType {
	case "executable":
		b.logger.Info("linking executable: %s", filepath.Base(outputPath))
		if err := b.scheduleLinkingTask(objectFiles, outputPath); err != nil {
			return fmt.Errorf("failed to link object files: %w", err)
		}
	case "static_lib":
		b.logger.Info("creating static library: %s", filepath.Base(outputPath))
		if err := b.scheduleArchiveTask(objectFiles, outputPath); err != nil {
			return fmt.Errorf("failed to create static library: %w", err)
		}
	case "shared_lib":
		b.logger.Info("creating shared library: %s", filepath.Base(outputPath))
		if err := b.scheduleSharedLibTask(objectFiles, outputPath); err != nil {
			return fmt.Errorf("failed to create shared library: %w", err)
		}
	default:
		return fmt.Errorf("unsupported output type: %s", b.Config.Build.OutputType)
	}

	if err := b.executePostBuildCommands(); err != nil {
		return fmt.Errorf("post-build commands failed: %w", err)
	}

	if err := b.Cache.Save(); err != nil {
		b.logger.Warning("failed to save build cache: %v", err)
	}

	buildTime := time.Since(startTime)
	b.logger.Success("build completed in %.2f seconds", buildTime.Seconds())
	b.logger.Success("output: %s", outputPath)

	return nil
}

// executePreBuildCommands executes pre-build commands
func (b *Builder) executePreBuildCommands() error {
	if len(b.Config.Build.PreBuildCmds) == 0 {
		return nil
	}

	b.logger.Info("executing pre-build commands...")
	b.logger.StartProgress(len(b.Config.Build.PreBuildCmds), "running pre-build commands")

	for i, cmdStr := range b.Config.Build.PreBuildCmds {
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		b.logger.UpdateProgress(i+1, fmt.Sprintf("running: %s", cmd))

		task := &Task{
			ID:      fmt.Sprintf("pre-build-%s", cmd),
			Command: cmd,
			Args:    args,
		}

		b.Executor.Submit(task)
		result := b.Executor.WaitForTask(task)
		if result == nil || !result.Success {
			b.logger.StopProgress()
			if result != nil {
				b.logger.Error("pre-build command failed: %v", result.Error)
				return fmt.Errorf("pre-build command failed: %v", result.Error)
			}
			b.logger.Error("pre-build command failed: unknown error")
			return fmt.Errorf("pre-build command failed: unknown error")
		}
	}

	b.logger.StopProgress()
	b.logger.Success("pre-build commands completed")
	return nil
}

// executePostBuildCommands executes post-build commands
func (b *Builder) executePostBuildCommands() error {
	if len(b.Config.Build.PostBuildCmds) == 0 {
		return nil
	}

	b.logger.Info("Executing post-build commands...")
	b.logger.StartProgress(len(b.Config.Build.PostBuildCmds), "Running post-build commands")

	for i, cmdStr := range b.Config.Build.PostBuildCmds {
		// Split command and arguments
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		// Replace variables in arguments
		for j, arg := range args {
			// Replace ${output} with the actual output path
			args[j] = strings.ReplaceAll(arg, "${output}", b.getOutputPath(filepath.Join(b.OutputDir, b.Target)))
		}

		b.logger.UpdateProgress(i+1, fmt.Sprintf("Running: %s", cmd))

		task := &Task{
			ID:      fmt.Sprintf("post-build-%s", cmd),
			Command: cmd,
			Args:    args,
		}

		// Execute synchronously
		b.Executor.Submit(task)
		result := b.Executor.WaitForTask(task)

		if result == nil || !result.Success {
			b.logger.StopProgress()
			if result != nil {
				b.logger.Error("post-build command failed: %v", result.Error)
				return fmt.Errorf("post-build command failed: %v", result.Error)
			}
			b.logger.Error("post-build command failed: unknown error")
			return fmt.Errorf("post-build command failed: unknown error")
		}
	}

	b.logger.StopProgress()
	b.logger.Success("post-build commands completed")
	return nil
}

// buildDependencyGraph builds the dependency graph for the project
func (b *Builder) buildDependencyGraph(sourceFiles []string) error {
	b.logger.Info("analyzing dependencies...")
	b.logger.StartProgress(len(sourceFiles), "scanning dependencies")

	for i, sourceFile := range sourceFiles {
		b.logger.UpdateProgress(i+1, fmt.Sprintf("scanning %s", filepath.Base(sourceFile)))

		// source node
		sourceNode := &dependency.Node{
			ID:   sourceFile,
			Type: dependency.NodeTypeSource,
			Path: sourceFile,
		}

		if err := b.Graph.AddNode(sourceNode); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				b.logger.StopProgress()
				return fmt.Errorf("failed to add source node: %w", err)
			}
		}

		// find deps
		deps, err := b.Scanner.Scan(sourceFile)
		if err != nil {
			b.logger.StopProgress()
			return fmt.Errorf("failed to scan dependencies for %s: %w", sourceFile, err)
		}

		if b.Verbose {
			b.logger.Note("found %d dependencies for %s", len(deps), sourceFile)
		}

		for _, depPath := range deps {
			headerNode := &dependency.Node{
				ID:   depPath,
				Type: dependency.NodeTypeHeader,
				Path: depPath,
			}

			if err := b.Graph.AddNode(headerNode); err != nil {
				if !strings.Contains(err.Error(), "already exists") {
					b.logger.StopProgress()
					return fmt.Errorf("failed to add header node: %w", err)
				}
			}

			if err := b.Graph.AddDependency(sourceFile, depPath); err != nil {
				b.logger.StopProgress()
				return fmt.Errorf("failed to add dependency: %w", err)
			}
		}
	}

	b.logger.StopProgress()
	b.logger.Success("dependency analysis complete")
	return nil
}

// scheduleCompilationTasks schedules source file compilations
func (b *Builder) scheduleCompilationTasks(sourceFiles []string, outputDir string) ([]string, error) {
	var objectFiles []string
	var filesToCompile []*Task

	for _, sourceFile := range sourceFiles {
		ext := filepath.Ext(sourceFile)
		if ext == ".cpp" || ext == ".cc" || ext == ".cxx" || ext == ".C" {
			b.HasCppFiles = true
			break
		}
	}

	cFlags := b.getCompilationFlags()

	totalFiles := len(sourceFiles)
	compiledCount := 0
	b.logger.StartProgress(totalFiles, "compiling")

	for _, sourceFile := range sourceFiles {
		ext := filepath.Ext(sourceFile)
		isCpp := ext == ".cpp" || ext == ".cc" || ext == ".cxx" || ext == ".C"

		objectFile := b.getObjectFilePath(sourceFile, outputDir)
		objectFiles = append(objectFiles, objectFile)

		objectNode := &dependency.Node{
			ID:   objectFile,
			Type: dependency.NodeTypeObject,
			Path: objectFile,
		}

		if err := b.Graph.AddNode(objectNode); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				b.logger.StopProgress()
				return nil, fmt.Errorf("failed to add object node: %w", err)
			}
		}

		if err := b.Graph.AddDependency(objectFile, sourceFile); err != nil {
			b.logger.StopProgress()
			return nil, fmt.Errorf("failed to add dependency: %w", err)
		}

		sourceNode, _ := b.Graph.GetNode(sourceFile)
		for _, dep := range sourceNode.Dependencies {
			if err := b.Graph.AddDependency(objectFile, dep.ID); err != nil {
				b.logger.StopProgress()
				return nil, fmt.Errorf("failed to add dependency: %w", err)
			}
		}

		commandHash := b.Cache.CalculateCommandHash(b.Compiler.GetName(), cFlags)
		var dependencies []string
		dependencies = append(dependencies, sourceFile)
		for _, dep := range sourceNode.Dependencies {
			dependencies = append(dependencies, dep.Path)
		}

		needsRebuild, reason := b.needsRebuild(objectFile, dependencies, commandHash)
		if reason == "" {
			b.logger.Warning("error checking if %s needs rebuild: %v", sourceFile, reason)
			needsRebuild = true
			reason = "error occurred"
		}

		if !needsRebuild {
			compiledCount++
			b.logger.UpdateProgress(compiledCount, fmt.Sprintf("Skipping %s (up to date)", filepath.Base(sourceFile)))
			if b.Verbose {
				b.logger.Note("Skipping up-to-date file: %s", sourceFile)
			}
			continue
		}

		objDir := filepath.Dir(objectFile)
		if err := os.MkdirAll(objDir, 0755); err != nil {
			b.logger.StopProgress()
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}

		compilerCmd := b.Compiler.GetName()
		if isCpp {
			if strings.Contains(compilerCmd, "clang") {
				compilerCmd = "clang++"
			} else if strings.Contains(compilerCmd, "gcc") {
				compilerCmd = "g++"
			}
		}

		task := &Task{
			ID:           sourceFile,
			Command:      compilerCmd,
			Args:         append([]string{"-c", sourceFile, "-o", objectFile}, cFlags...),
			Dir:          "",
			Env:          nil,
			Output:       nil,
			SourceFile:   sourceFile,
			OutputFile:   objectFile,
			Dependencies: nil,
		}

		filesToCompile = append(filesToCompile, task)
	}

	for _, task := range filesToCompile {
		b.Executor.Submit(task)
	}

	var compilationErrors []string
	for _, task := range filesToCompile {
		result := b.Executor.WaitForTask(task)
		compiledCount++
		b.logger.UpdateProgress(compiledCount, fmt.Sprintf("Compiled %s", filepath.Base(task.SourceFile)))

		if result == nil || !result.Success {
			if result != nil {
				errorMsg := fmt.Sprintf("Compilation of %s failed: %v", task.SourceFile, result.Error)
				compilationErrors = append(compilationErrors, errorMsg)
				b.parseCompilerOutput(result.Error.Error(), task.SourceFile)
			} else {
				errorMsg := fmt.Sprintf("Compilation of %s failed: unknown error", task.SourceFile)
				compilationErrors = append(compilationErrors, errorMsg)
				b.logger.Error(errorMsg)
			}
			continue
		}

		if b.Verbose {
			b.logger.Note("Compiled %s in %.2f seconds", filepath.Base(task.SourceFile), result.Duration.Seconds())
		}

		sourceNode, _ := b.Graph.GetNode(task.SourceFile)
		var dependencies []string
		dependencies = append(dependencies, task.SourceFile)
		for _, dep := range sourceNode.Dependencies {
			dependencies = append(dependencies, dep.Path)
		}

		commandHash := b.Cache.CalculateCommandHash(b.Compiler.GetName(), cFlags)
		compilationTime := result.Duration

		if err := b.Cache.UpdateEntry(task.OutputFile, dependencies, commandHash, task.OutputFile, compilationTime); err != nil {
			b.logger.Warning("Failed to update cache entry for %s: %v", task.SourceFile, err)
		}
	}

	b.logger.StopProgress()
	if len(compilationErrors) > 0 {
		for _, err := range compilationErrors {
			b.logger.Error("%s", err)
		}
		return nil, fmt.Errorf("compilation failed with %d errors", len(compilationErrors))
	}

	b.logger.Success("Compilation complete")
	return objectFiles, nil
}

// needsRebuild determines if a file needs to be rebuilt
func (b *Builder) needsRebuild(objectFile string, dependencies []string, commandHash string) (bool, string) {
	if needsRebuild, err := b.Cache.NeedsRebuild(objectFile, dependencies, commandHash); err == nil && needsRebuild {
		return true, "cache indicates rebuild needed"
	}

	objInfo, err := os.Stat(objectFile)
	if os.IsNotExist(err) {
		return true, "object file doesn't exist"
	}

	sourceFile := dependencies[0] // first dependency is the source file
	srcInfo, err := os.Stat(sourceFile)
	if err != nil {
		return true, "cannot stat source file"
	}

	if srcInfo.ModTime().After(objInfo.ModTime()) {
		return true, "source file modified"
	}

	for _, dep := range dependencies[1:] {
		depInfo, err := os.Stat(dep)
		if err != nil {
			return true, "cannot stat dependency"
		}

		if depInfo.ModTime().After(objInfo.ModTime()) {
			return true, fmt.Sprintf("dependency %s modified", filepath.Base(dep))
		}
	}

	return false, "up to date"
}

// scheduleLinkingTask schedules the final linking task for an executable
func (b *Builder) scheduleLinkingTask(objectFiles []string, outputPath string) error {
	linkFlags := b.getLinkingFlags() // get flags
	outDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	compilerCmd := b.Compiler.GetName()
	if b.HasCppFiles {
		if strings.Contains(compilerCmd, "clang") {
			compilerCmd = "clang++"
		} else if strings.Contains(compilerCmd, "gcc") {
			compilerCmd = "g++"
		}
	}

	task := &Task{
		ID:         "link",
		Command:    compilerCmd,
		Args:       append(append(objectFiles, "-o", outputPath), linkFlags...),
		Dir:        "",
		Env:        nil,
		Output:     nil,
		OutputFile: outputPath,
	}

	b.logger.StartProgress(1, "linking executable")

	b.Executor.Submit(task)
	result := b.Executor.WaitForTask(task)

	b.logger.StopProgress()

	if result == nil || !result.Success {
		if result != nil {
			b.logger.Error("linking failed: %v", result.Error)
			return fmt.Errorf("linking failed: %v", result.Error)
		}
		b.logger.Error("linking failed: unknown error")
		return fmt.Errorf("linking failed: unknown error")
	}

	b.logger.Success("linking complete")
	return nil
}

// scheduleArchiveTask schedules the creation of a static library
func (b *Builder) scheduleArchiveTask(objectFiles []string, outputPath string) error {
	archiverFlags := b.getArchiverFlags()
	outDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	b.logger.StartProgress(1, "creating static library")

	if err := b.Compiler.Archive(objectFiles, outputPath, archiverFlags); err != nil {
		b.logger.StopProgress()
		b.logger.Error("archiving failed: %v", err)
		return fmt.Errorf("archiving failed: %w", err)
	}

	b.logger.StopProgress()
	b.logger.Success("static library created")
	return nil
}

// scheduleSharedLibTask schedules the creation of a shared library
func (b *Builder) scheduleSharedLibTask(objectFiles []string, outputPath string) error {
	linkFlags := append(b.getLinkingFlags(), "-shared")

	switch b.platformInfo.Platform {
	case platform.PlatformLinux:
		linkFlags = append(linkFlags, "-fPIC")
	case platform.PlatformMacOS:
		linkFlags = append(linkFlags, "-fPIC", "-dynamiclib")
	case platform.PlatformWindows:
	default:
		// nothing as of rn.
	}

	outDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	compilerCmd := b.Compiler.GetName()
	if b.HasCppFiles {
		if strings.Contains(compilerCmd, "clang") {
			compilerCmd = "clang++"
		} else if strings.Contains(compilerCmd, "gcc") {
			compilerCmd = "g++"
		}
	}

	task := &Task{
		ID:         "shared_lib",
		Command:    compilerCmd,
		Args:       append(append(objectFiles, "-o", outputPath), linkFlags...),
		Dir:        "",
		Env:        nil,
		Output:     nil,
		OutputFile: outputPath,
	}

	b.logger.StartProgress(1, "Creating shared library")

	// submit and wait
	b.Executor.Submit(task)
	result := b.Executor.WaitForTask(task)

	b.logger.StopProgress()

	if result == nil || !result.Success {
		if result != nil {
			b.logger.Error("shared library creation failed: %v", result.Error)
			return fmt.Errorf("shared library creation failed: %v", result.Error)
		}
		b.logger.Error("shared library creation failed: unknown error")
		return fmt.Errorf("shared library creation failed: unknown error")
	}

	b.logger.Success("Shared library created")
	return nil
}

// parseCompilerOutput parses compiler error output for better formatting
func (b *Builder) parseCompilerOutput(output, sourceFile string) {
	parser := compiler.NewErrorParser(b.logger)
	parser.Report(output, sourceFile)
}

// getCompilationFlags gets the compilation flags for the current target
func (b *Builder) getCompilationFlags() []string {
	var flags []string

	if b.Config.Project.Language == "c" {
		flags = append(flags, b.Config.Toolchain.CFlags...)
	} else if b.Config.Project.Language == "c++" {
		flags = append(flags, b.Config.Toolchain.CXXFlags...)
	}

	// c/c++ std
	if b.Config.Project.Standard != "" {
		if b.Config.Project.Language == "c" {
			flags = append(flags, "-std="+b.Config.Project.Standard)
		} else if b.Config.Project.Language == "c++" {
			flags = append(flags, "-std="+b.Config.Project.Standard)
		}
	}

	// includes
	for _, dir := range b.Config.Build.IncludeDirs {
		flags = append(flags, "-I"+dir)
	}

	if target, ok := b.Config.Targets[b.Target]; ok {
		if b.Config.Project.Language == "c" {
			flags = append(flags, target.CFlags...)
		} else if b.Config.Project.Language == "c++" {
			flags = append(flags, target.CXXFlags...)
		}
	}

	return flags
}

// getLinkingFlags gets the linking flags for the current target
func (b *Builder) getLinkingFlags() []string {
	var flags []string

	// global flags
	flags = append(flags, b.Config.Toolchain.LinkerFlags...)

	// target flags
	if target, ok := b.Config.Targets[b.Target]; ok {
		flags = append(flags, target.LinkerFlags...)
	}

	// Add C++ standard library if needed
	if b.HasCppFiles {
		flags = append(flags, "-lstdc++")
	}

	return flags
}

// getArchiverFlags gets the archiver flags
func (b *Builder) getArchiverFlags() []string {
	return b.Config.Toolchain.ArchiverFlags
}

// getObjectFilePath calculates the path for an object file
func (b *Builder) getObjectFilePath(sourceFile, outputDir string) string {
	baseName := filepath.Base(sourceFile)
	ext := filepath.Ext(baseName)
	baseName = baseName[:len(baseName)-len(ext)]

	relPath, err := filepath.Rel(".", filepath.Dir(sourceFile))
	if err != nil {
		// flat directory as fallback
		return filepath.Join(outputDir, baseName+b.Compiler.GetObjectExtension())
	}

	return filepath.Join(outputDir, relPath, baseName+b.Compiler.GetObjectExtension())
}

// getOutputPath calculates the path for the final output
func (b *Builder) getOutputPath(outputDir string) string {
	outputName := b.Config.Build.OutputName

	switch b.Config.Build.OutputType {
	case "executable":
		return filepath.Join(outputDir, outputName+b.Compiler.GetExecutableExtension())
	case "static_lib":
		return filepath.Join(outputDir, "lib"+outputName+b.Compiler.GetStaticLibraryExtension())
	case "shared_lib":
		return filepath.Join(outputDir, "lib"+outputName+b.Compiler.GetSharedLibraryExtension())
	default:
		return filepath.Join(outputDir, outputName+b.Compiler.GetExecutableExtension())
	}
}

// Clean removes build artifacts
func (b *Builder) Clean() error {
	if b.Target == "" {
		b.logger.Info("cleaning all build artifacts")
	} else {
		b.logger.Info("cleaning build artifacts for target: %s", b.Target)
	}

	targetOutputDir := filepath.Join(b.OutputDir, b.Target)
	if err := os.RemoveAll(targetOutputDir); err != nil {
		b.logger.Error("failed to clean target directory: %v", err)
		return fmt.Errorf("failed to clean target directory: %w", err)
	}

	// also clean cache directory
	cacheDir := ".styx"
	if _, err := os.Stat(cacheDir); err == nil {
		b.logger.Info("removing cache: %s", cacheDir)
		if err := os.RemoveAll(cacheDir); err != nil {
			b.logger.Warning("failed to remove cache: %v", err)
		}
	}

	b.Cache.Clean()
	if err := b.Cache.Save(); err != nil {
		b.logger.Warning("failed to save cleaned cache: %v", err)
	}

	b.logger.Success("clean completed successfully")
	return nil
}
