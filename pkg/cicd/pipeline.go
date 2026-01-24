package cicd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CI/CD Pipeline Framework
// Supports automated testing, building, and deployment

// Pipeline represents a CI/CD pipeline
type Pipeline struct {
	Name        string
	Description string
	Stages      []Stage
	Config      PipelineConfig
	Artifacts   map[string]string
	mu          sync.RWMutex
}

// Stage represents a pipeline stage
type Stage struct {
	Name         string
	Jobs         []Job
	Parallel     bool
	ContinueOnError bool
	Timeout      time.Duration
}

// Job represents a single job in a stage
type Job struct {
	Name        string
	Commands    []Command
	Environment map[string]string
	Artifacts   []string
	DependsOn   []string
	Timeout     time.Duration
	RetryCount  int
}

// Command represents a shell command
type Command struct {
	Name       string
	Script     string
	WorkingDir string
	Timeout    time.Duration
}

// PipelineConfig holds pipeline configuration
type PipelineConfig struct {
	WorkingDir     string
	Environment    map[string]string
	Timeout        time.Duration
	FailFast       bool
	ArtifactsDir   string
	ReportDir      string
	Parallel       bool
}

// PipelineResult represents pipeline execution result
type PipelineResult struct {
	PipelineName string
	Success      bool
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Stages       []StageResult
	Artifacts    map[string]string
	Error        error
}

// StageResult represents stage execution result
type StageResult struct {
	StageName  string
	Success    bool
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	Jobs       []JobResult
}

// JobResult represents job execution result
type JobResult struct {
	JobName   string
	Success   bool
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Output    string
	Error     error
	Retries   int
}

// Runner executes CI/CD pipelines
type Runner struct {
	config PipelineConfig
	logger Logger
}

// Logger interface for pipeline logging
type Logger interface {
	Info(msg string)
	Error(msg string)
	Debug(msg string)
}

// NewRunner creates a new pipeline runner
func NewRunner(config PipelineConfig, logger Logger) *Runner {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}

	if config.WorkingDir == "" {
		config.WorkingDir, _ = os.Getwd()
	}

	if config.ArtifactsDir == "" {
		config.ArtifactsDir = "./artifacts"
	}

	if config.ReportDir == "" {
		config.ReportDir = "./reports"
	}

	return &Runner{
		config: config,
		logger: logger,
	}
}

// Run executes a pipeline
func (r *Runner) Run(ctx context.Context, pipeline *Pipeline) (*PipelineResult, error) {
	result := &PipelineResult{
		PipelineName: pipeline.Name,
		StartTime:    time.Now(),
		Stages:       make([]StageResult, 0, len(pipeline.Stages)),
		Artifacts:    make(map[string]string),
	}

	// Create timeout context
	timeout := pipeline.Config.Timeout
	if timeout == 0 {
		timeout = r.config.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.logger.Info(fmt.Sprintf("Starting pipeline: %s", pipeline.Name))

	// Execute stages
	for _, stage := range pipeline.Stages {
		stageResult := r.runStage(ctx, stage, pipeline.Config)
		result.Stages = append(result.Stages, stageResult)

		if !stageResult.Success {
			if r.config.FailFast || !stage.ContinueOnError {
				result.Success = false
				result.Error = fmt.Errorf("stage %s failed", stage.Name)
				break
			}
		}
	}

	// Check if all stages succeeded
	result.Success = true
	for _, stageResult := range result.Stages {
		if !stageResult.Success {
			result.Success = false
			break
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	r.logger.Info(fmt.Sprintf("Pipeline completed: %s (success: %v, duration: %v)",
		pipeline.Name, result.Success, result.Duration))

	return result, nil
}

// runStage executes a stage
func (r *Runner) runStage(ctx context.Context, stage Stage, config PipelineConfig) StageResult {
	result := StageResult{
		StageName: stage.Name,
		StartTime: time.Now(),
		Jobs:      make([]JobResult, 0, len(stage.Jobs)),
	}

	r.logger.Info(fmt.Sprintf("Starting stage: %s", stage.Name))

	// Set stage timeout
	timeout := stage.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	stageCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute jobs
	if stage.Parallel {
		result.Jobs = r.runJobsParallel(stageCtx, stage.Jobs, config)
	} else {
		result.Jobs = r.runJobsSequential(stageCtx, stage.Jobs, config)
	}

	// Check if all jobs succeeded
	result.Success = true
	for _, jobResult := range result.Jobs {
		if !jobResult.Success {
			result.Success = false
			break
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	r.logger.Info(fmt.Sprintf("Stage completed: %s (success: %v, duration: %v)",
		stage.Name, result.Success, result.Duration))

	return result
}

// runJobsSequential runs jobs sequentially
func (r *Runner) runJobsSequential(ctx context.Context, jobs []Job, config PipelineConfig) []JobResult {
	results := make([]JobResult, 0, len(jobs))

	for _, job := range jobs {
		result := r.runJob(ctx, job, config)
		results = append(results, result)

		if !result.Success && r.config.FailFast {
			break
		}
	}

	return results
}

// runJobsParallel runs jobs in parallel
func (r *Runner) runJobsParallel(ctx context.Context, jobs []Job, config PipelineConfig) []JobResult {
	var wg sync.WaitGroup
	resultChan := make(chan JobResult, len(jobs))

	for _, job := range jobs {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			result := r.runJob(ctx, j, config)
			resultChan <- result
		}(job)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	results := make([]JobResult, 0, len(jobs))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// runJob executes a single job
func (r *Runner) runJob(ctx context.Context, job Job, config PipelineConfig) JobResult {
	result := JobResult{
		JobName:   job.Name,
		StartTime: time.Now(),
	}

	r.logger.Info(fmt.Sprintf("Starting job: %s", job.Name))

	// Set job timeout
	timeout := job.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute commands with retry
	var lastErr error
	for attempt := 0; attempt <= job.RetryCount; attempt++ {
		if attempt > 0 {
			r.logger.Info(fmt.Sprintf("Retrying job: %s (attempt %d/%d)", job.Name, attempt, job.RetryCount))
			result.Retries++
		}

		output, err := r.executeCommands(jobCtx, job, config)
		result.Output = output

		if err == nil {
			result.Success = true
			break
		}

		lastErr = err
	}

	if !result.Success {
		result.Error = lastErr
		r.logger.Error(fmt.Sprintf("Job failed: %s - %v", job.Name, lastErr))
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	r.logger.Info(fmt.Sprintf("Job completed: %s (success: %v, duration: %v)",
		job.Name, result.Success, result.Duration))

	return result
}

// executeCommands executes job commands
func (r *Runner) executeCommands(ctx context.Context, job Job, config PipelineConfig) (string, error) {
	var output strings.Builder

	// Set environment variables
	env := os.Environ()
	for key, value := range config.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	for key, value := range job.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Execute each command
	for _, command := range job.Commands {
		r.logger.Debug(fmt.Sprintf("Executing command: %s", command.Name))

		workingDir := command.WorkingDir
		if workingDir == "" {
			workingDir = config.WorkingDir
		}

		// Set command timeout
		cmdTimeout := command.Timeout
		if cmdTimeout == 0 {
			cmdTimeout = 1 * time.Minute
		}

		cmdCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
		defer cancel()

		// Execute command
		cmd := exec.CommandContext(cmdCtx, "sh", "-c", command.Script)
		cmd.Dir = workingDir
		cmd.Env = env

		cmdOutput, err := cmd.CombinedOutput()
		output.Write(cmdOutput)

		if err != nil {
			return output.String(), fmt.Errorf("command '%s' failed: %w\nOutput: %s",
				command.Name, err, string(cmdOutput))
		}
	}

	return output.String(), nil
}

// Predefined pipelines

// NewTestPipeline creates a test pipeline
func NewTestPipeline() *Pipeline {
	return &Pipeline{
		Name:        "Test Pipeline",
		Description: "Run unit and integration tests",
		Stages: []Stage{
			{
				Name: "Setup",
				Jobs: []Job{
					{
						Name: "Install Dependencies",
						Commands: []Command{
							{Name: "Go Mod Download", Script: "go mod download"},
						},
					},
				},
			},
			{
				Name: "Test",
				Jobs: []Job{
					{
						Name: "Unit Tests",
						Commands: []Command{
							{Name: "Run Tests", Script: "go test -v -race -coverprofile=coverage.out ./..."},
							{Name: "Coverage Report", Script: "go tool cover -html=coverage.out -o coverage.html"},
						},
						Artifacts: []string{"coverage.out", "coverage.html"},
					},
					{
						Name: "Integration Tests",
						Commands: []Command{
							{Name: "Run Integration Tests", Script: "go test -v -tags=integration ./..."},
						},
					},
				},
				Parallel: true,
			},
			{
				Name: "Lint",
				Jobs: []Job{
					{
						Name: "Go Lint",
						Commands: []Command{
							{Name: "Run Linter", Script: "golangci-lint run ./..."},
						},
					},
				},
			},
		},
	}
}

// NewBuildPipeline creates a build pipeline
func NewBuildPipeline() *Pipeline {
	return &Pipeline{
		Name:        "Build Pipeline",
		Description: "Build and package application",
		Stages: []Stage{
			{
				Name: "Build",
				Jobs: []Job{
					{
						Name: "Build Binary",
						Commands: []Command{
							{Name: "Build", Script: "go build -o bin/opentx ./cmd/gateway"},
						},
						Artifacts: []string{"bin/opentx"},
					},
				},
			},
			{
				Name: "Package",
				Jobs: []Job{
					{
						Name: "Docker Build",
						Commands: []Command{
							{Name: "Build Image", Script: "docker build -t opentx:latest ."},
							{Name: "Tag Image", Script: "docker tag opentx:latest opentx:$(git rev-parse --short HEAD)"},
						},
					},
				},
			},
		},
	}
}

// NewDeployPipeline creates a deployment pipeline
func NewDeployPipeline(environment string) *Pipeline {
	return &Pipeline{
		Name:        fmt.Sprintf("Deploy to %s", environment),
		Description: fmt.Sprintf("Deploy application to %s environment", environment),
		Stages: []Stage{
			{
				Name: "Pre-Deploy",
				Jobs: []Job{
					{
						Name: "Run Smoke Tests",
						Commands: []Command{
							{Name: "Smoke Test", Script: "go test -v -tags=smoke ./..."},
						},
					},
				},
			},
			{
				Name: "Deploy",
				Jobs: []Job{
					{
						Name: "Deploy to Kubernetes",
						Commands: []Command{
							{Name: "Apply Manifests", Script: fmt.Sprintf("kubectl apply -f k8s/%s/", environment)},
							{Name: "Wait for Rollout", Script: "kubectl rollout status deployment/opentx"},
						},
					},
				},
			},
			{
				Name: "Post-Deploy",
				Jobs: []Job{
					{
						Name: "Health Check",
						Commands: []Command{
							{Name: "Check Health", Script: "curl -f http://localhost:8080/health || exit 1"},
						},
						RetryCount: 3,
					},
				},
			},
		},
	}
}

// ConsoleLogger is a simple console logger
type ConsoleLogger struct{}

func (l *ConsoleLogger) Info(msg string) {
	fmt.Printf("[INFO] %s\n", msg)
}

func (l *ConsoleLogger) Error(msg string) {
	fmt.Printf("[ERROR] %s\n", msg)
}

func (l *ConsoleLogger) Debug(msg string) {
	fmt.Printf("[DEBUG] %s\n", msg)
}

// NewConsoleLogger creates a new console logger
func NewConsoleLogger() Logger {
	return &ConsoleLogger{}
}

// GenerateGitHubActionsWorkflow generates GitHub Actions workflow YAML
func GenerateGitHubActionsWorkflow(pipeline *Pipeline) string {
	var sb strings.Builder

	sb.WriteString("name: " + pipeline.Name + "\n\n")
	sb.WriteString("on:\n")
	sb.WriteString("  push:\n")
	sb.WriteString("    branches: [ main ]\n")
	sb.WriteString("  pull_request:\n")
	sb.WriteString("    branches: [ main ]\n\n")
	sb.WriteString("jobs:\n")

	for _, stage := range pipeline.Stages {
		for _, job := range stage.Jobs {
			jobName := strings.ReplaceAll(strings.ToLower(job.Name), " ", "-")
			sb.WriteString("  " + jobName + ":\n")
			sb.WriteString("    runs-on: ubuntu-latest\n")
			sb.WriteString("    steps:\n")
			sb.WriteString("      - uses: actions/checkout@v3\n")
			sb.WriteString("      - uses: actions/setup-go@v4\n")
			sb.WriteString("        with:\n")
			sb.WriteString("          go-version: '1.21'\n")

			for _, cmd := range job.Commands {
				sb.WriteString("      - name: " + cmd.Name + "\n")
				sb.WriteString("        run: " + cmd.Script + "\n")
			}

			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// ValidatePipeline validates pipeline configuration
func ValidatePipeline(pipeline *Pipeline) error {
	if pipeline.Name == "" {
		return errors.New("pipeline name is required")
	}

	if len(pipeline.Stages) == 0 {
		return errors.New("pipeline must have at least one stage")
	}

	for i, stage := range pipeline.Stages {
		if stage.Name == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}

		if len(stage.Jobs) == 0 {
			return fmt.Errorf("stage %s: must have at least one job", stage.Name)
		}

		for j, job := range stage.Jobs {
			if job.Name == "" {
				return fmt.Errorf("stage %s, job %d: name is required", stage.Name, j)
			}

			if len(job.Commands) == 0 {
				return fmt.Errorf("job %s: must have at least one command", job.Name)
			}
		}
	}

	return nil
}
