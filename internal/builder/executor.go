package builder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/deviceix/styx/internal/logger"
)

// Task represents a build task
type Task struct {
	ID           string
	Command      string
	Args         []string
	Dir          string
	Env          map[string]string
	Output       *bytes.Buffer
	SourceFile   string
	OutputFile   string
	Dependencies []*Task
	CompleteCh   chan struct{}
	Completed    bool
	Error        error
	StartTime    time.Time
	EndTime      time.Time
}

// Result represents the result of a task execution
type Result struct {
	Task     *Task
	Success  bool
	Error    error
	Output   string
	Duration time.Duration
}

// Executor manages parallel execution of build tasks
type Executor struct {
	WorkerCount    int
	Tasks          chan *Task
	Results        chan *Result
	WaitGroup      sync.WaitGroup
	Context        context.Context
	Cancel         context.CancelFunc
	CompletedTasks map[string]bool
	TasksMutex     sync.Mutex
	logger         *logger.Logger
}

// NewExecutor creates a new executor with the specified number of workers
func NewExecutor(workerCount int) *Executor {
	if workerCount <= 0 {
		// default to use all
		workerCount = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Executor{
		WorkerCount:    workerCount,
		Tasks:          make(chan *Task, 100),   // buffer for pending tasks
		Results:        make(chan *Result, 100), // buffer for results
		Context:        ctx,
		Cancel:         cancel,
		CompletedTasks: make(map[string]bool),
		logger:         logger.New(false), // Default logger with normal verbosity
	}
}

// SetLogger sets the logger for the executor
func (e *Executor) SetLogger(log *logger.Logger) {
	e.logger = log
}

// SetVerbose sets the verbose mode for the executor's logger
func (e *Executor) SetVerbose(verbose bool) {
	e.logger = logger.New(verbose)
}

// Start starts the worker pool
func (e *Executor) Start() {
	e.logger.Info("starting build executor with %d workers", e.WorkerCount)

	for i := 0; i < e.WorkerCount; i++ {
		e.WaitGroup.Add(1)
		go e.worker(i)
	}
}

// worker processes tasks from the queue
func (e *Executor) worker(id int) {
	defer e.WaitGroup.Done()

	for {
		select {
		case <-e.Context.Done():
			return

		case task, ok := <-e.Tasks:
			if !ok {
				// channel closed, exit worker
				//if e.logger != nil {
				//	e.logger.Note("worker %d stopped due to closed channel", id)
				//}
				return
			}

			// check if all dependencies are completed
			canExecute := true
			for _, dep := range task.Dependencies {
				e.TasksMutex.Lock()
				completed := e.CompletedTasks[dep.ID]
				e.TasksMutex.Unlock()

				if !completed {
					canExecute = false
					break
				}
			}

			if !canExecute {
				// requeue task for later
				e.Tasks <- task
				time.Sleep(10 * time.Millisecond) // avoid processor exhaustion
				continue
			}

			result := &Result{
				Task: task,
			}

			task.StartTime = time.Now()

			cmd := exec.CommandContext(e.Context, task.Command, task.Args...)
			cmd.Dir = task.Dir

			env := os.Environ()
			for k, v := range task.Env {
				env = append(env, k+"="+v)
			}
			cmd.Env = env

			var stderr bytes.Buffer
			if task.Output == nil {
				task.Output = &bytes.Buffer{}
			}

			cmd.Stdout = task.Output
			cmd.Stderr = &stderr
			err := cmd.Run()

			task.EndTime = time.Now()
			result.Duration = task.EndTime.Sub(task.StartTime)

			if err != nil {
				// failed
				errOutput := stderr.String()
				task.Error = fmt.Errorf("%w: %s", err, errOutput)
				result.Success = false
				result.Error = task.Error
				result.Output = errOutput

				if e.logger != nil {
					e.logger.Error("ask %s failed: %v", task.ID, err)
					if len(errOutput) > 0 {
						e.logger.Note("error output: %s", errOutput)
					}
				}

				task.Completed = true
				if task.CompleteCh != nil {
					// to prevent deadlock
					close(task.CompleteCh)
				}
			} else {
				result.Success = true
				result.Output = task.Output.String()

				e.TasksMutex.Lock()
				e.CompletedTasks[task.ID] = true
				e.TasksMutex.Unlock()

				if task.CompleteCh != nil {
					close(task.CompleteCh)
				}

				if e.logger != nil {
					e.logger.Note("task %s completed successfully in %.2f seconds",
						task.ID, result.Duration.Seconds())
				}
			}

			e.Results <- result
		}
	}
}

// Submit submits a task for execution
func (e *Executor) Submit(task *Task) {
	if task.CompleteCh == nil {
		task.CompleteCh = make(chan struct{})
	}
	e.Tasks <- task
}

// Shutdown stops all workers after they finish their current tasks
func (e *Executor) Shutdown() {
	if e.logger != nil {
		e.logger.Info("shutting down build executor")
	}

	close(e.Tasks)
	e.WaitGroup.Wait()
	close(e.Results)

	if e.logger != nil {
		e.logger.Info("nuild executor shut down")
	}
}

// ShutdownNow stops all workers immediately
func (e *Executor) ShutdownNow() {
	if e.logger != nil {
		e.logger.Info("forcefully shutting down build executor")
	}

	// cancel context to signal immediate stop; as noted in `worker()`
	e.Cancel()
	e.WaitGroup.Wait()

	close(e.Tasks)
	close(e.Results)

	if e.logger != nil {
		e.logger.Info("build executor forcefully shut down")
	}
}

// WaitForTask waits for a specific task to complete
func (e *Executor) WaitForTask(task *Task) *Result {
	if task.CompleteCh == nil {
		return nil
	}

	<-task.CompleteCh

	return &Result{
		Task:     task,
		Success:  task.Error == nil,
		Error:    task.Error,
		Output:   task.Output.String(),
		Duration: task.EndTime.Sub(task.StartTime),
	}
}

// WaitForAll waits for all submitted tasks to complete
func (e *Executor) WaitForAll() []Result {
	var results []Result

	// close the task channel to signal no more tasks
	close(e.Tasks)
	e.WaitGroup.Wait()

	// collect
	close(e.Results)
	for result := range e.Results {
		results = append(results, *result)
	}

	return results
}
