package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Pantani/gorchestrator/internal/app"
	"github.com/Pantani/gorchestrator/internal/doctor"
	"github.com/Pantani/gorchestrator/internal/domain"
)

func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	application := app.New(app.Options{StateDir: ".bgorch/state"})

	switch args[0] {
	case "validate":
		return runValidate(application, args[1:])
	case "render":
		return runRender(application, args[1:])
	case "plan":
		return runPlan(application, args[1:])
	case "apply":
		return runApply(application, args[1:])
	case "status":
		return runStatus(application, args[1:])
	case "doctor":
		return runDoctor(application, args[1:])
	case "-h", "--help", "help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printUsage()
		return 2
	}
}

func runValidate(application *app.App, args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	var filePath string
	var output string
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&output, "output", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	_, diags, err := application.ValidateSpec(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate failed: %v\n", err)
		return 1
	}

	if output == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(diags)
	} else {
		printDiagnostics(diags)
	}

	if app.HasErrors(diags) {
		return 1
	}
	fmt.Println("validation passed")
	return 0
}

func runRender(application *app.App, args []string) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	var filePath string
	var outputDir string
	var writeState bool
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&outputDir, "o", ".bgorch/render", "Output directory")
	fs.StringVar(&outputDir, "output-dir", ".bgorch/render", "Output directory")
	fs.BoolVar(&writeState, "write-state", false, "Persist desired state snapshot for future plan comparisons")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	desired, diags, err := application.Render(context.Background(), filePath, outputDir, writeState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render failed: %v\n", err)
		return 1
	}
	printDiagnostics(diags)
	if app.HasErrors(diags) {
		return 1
	}

	fmt.Printf("rendered %d artifact(s) to %s\n", len(desired.Artifacts), outputDir)
	paths := make([]string, 0, len(desired.Artifacts))
	for _, a := range desired.Artifacts {
		paths = append(paths, filepath.Join(outputDir, a.Path))
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Printf("- %s\n", p)
	}
	if writeState {
		fmt.Println("state snapshot saved to .bgorch/state")
	}
	return 0
}

func runPlan(application *app.App, args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	var filePath string
	var output string
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&output, "output", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	plan, diags, err := application.Plan(context.Background(), filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plan failed: %v\n", err)
		return 1
	}
	printDiagnostics(diags)
	if app.HasErrors(diags) {
		return 1
	}

	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(plan)
		return 0
	}

	printPlan(plan)
	return 0
}

func runApply(application *app.App, args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	var filePath string
	var outputDir string
	var output string
	var dryRun bool
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&outputDir, "o", ".bgorch/render", "Output directory")
	fs.StringVar(&outputDir, "output-dir", ".bgorch/render", "Output directory")
	fs.StringVar(&output, "output", "text", "Output format: text|json")
	fs.BoolVar(&dryRun, "dry-run", false, "Compute plan without writing artifacts or state")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	result, diags, err := application.Apply(context.Background(), filePath, app.ApplyOptions{OutputDir: outputDir, DryRun: dryRun})
	if err != nil {
		fmt.Fprintf(os.Stderr, "apply failed: %v\n", err)
		return 1
	}
	printDiagnostics(diags)
	if app.HasErrors(diags) {
		return 1
	}

	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return 0
	}

	fmt.Printf("cluster: %s\n", result.ClusterName)
	fmt.Printf("backend: %s\n", result.Backend)
	if result.LockPath != "" {
		fmt.Printf("lock: %s\n", result.LockPath)
	}
	printPlan(result.Plan)
	if result.DryRun {
		fmt.Println("dry-run enabled: no artifacts or state were written")
		return 0
	}
	fmt.Printf("artifacts written: %d\n", result.ArtifactsWritten)
	if result.SnapshotUpdated {
		fmt.Println("state snapshot updated")
	}
	return 0
}

func runStatus(application *app.App, args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	var filePath string
	var output string
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&output, "output", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	result, diags, err := application.Status(context.Background(), filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
		return 1
	}
	printDiagnostics(diags)
	if app.HasErrors(diags) {
		return 1
	}

	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return 0
	}

	printStatus(result)
	return 0
}

func runDoctor(application *app.App, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var filePath string
	var output string
	fs.StringVar(&filePath, "f", "", "Path to spec file")
	fs.StringVar(&filePath, "file", "", "Path to spec file")
	fs.StringVar(&output, "output", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "-f/--file is required")
		return 2
	}

	report, err := application.Doctor(context.Background(), filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
		return 1
	}

	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printDoctorReport(report)
	}

	if report.HasFailures() {
		return 1
	}
	return 0
}

func printDiagnostics(diags []domain.Diagnostic) {
	if len(diags) == 0 {
		return
	}
	for _, d := range diags {
		line := fmt.Sprintf("[%s] %s", strings.ToUpper(string(d.Severity)), d.Message)
		if d.Path != "" {
			line += " (" + d.Path + ")"
		}
		if d.Hint != "" {
			line += " | hint: " + d.Hint
		}
		fmt.Println(line)
	}
}

func printPlan(plan domain.Plan) {
	if len(plan.Changes) == 0 {
		fmt.Println("no resources in desired state")
		return
	}
	changes := make([]domain.PlanChange, 0, len(plan.Changes))
	for _, c := range plan.Changes {
		if c.Type != domain.ChangeNoop {
			changes = append(changes, c)
		}
	}
	if len(changes) == 0 {
		fmt.Println("plan: no changes")
		return
	}
	fmt.Printf("plan: %d change(s)\n", len(changes))
	for _, c := range changes {
		fmt.Printf("- %s %s %s", strings.ToUpper(string(c.Type)), c.ResourceType, c.Name)
		if c.Reason != "" {
			fmt.Printf(" (%s)", c.Reason)
		}
		fmt.Println()
	}
}

func printStatus(result app.StatusResult) {
	fmt.Printf("cluster: %s\n", result.ClusterName)
	fmt.Printf("backend: %s\n", result.Backend)
	fmt.Printf("snapshot path: %s\n", result.SnapshotPath)
	if result.SnapshotExists && result.Snapshot != nil {
		fmt.Printf("snapshot: present (%s)\n", result.Snapshot.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	} else {
		fmt.Println("snapshot: not found")
	}
	fmt.Printf("desired services: %d\n", result.DesiredServices)
	fmt.Printf("desired artifacts: %d\n", result.DesiredArtifacts)
	printPlan(result.Plan)
	if len(result.Observations) > 0 {
		fmt.Println("observations:")
		for _, o := range result.Observations {
			fmt.Printf("- %s\n", o)
		}
	}
}

func printDoctorReport(report doctor.Report) {
	if report.ClusterName != "" {
		fmt.Printf("cluster: %s\n", report.ClusterName)
	}
	if report.Backend != "" {
		fmt.Printf("backend: %s\n", report.Backend)
	}
	for _, c := range report.Checks {
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(string(c.Status)), c.Name, c.Message)
		if c.Hint != "" {
			fmt.Printf("  hint: %s\n", c.Hint)
		}
	}
}

func printUsage() {
	fmt.Println(`BGorch - The Blockchain Gorchestrator (MVP)

Usage:
  bgorch validate -f <spec.yaml> [--output text|json]
  bgorch render   -f <spec.yaml> [-o <out-dir>] [--write-state]
  bgorch plan     -f <spec.yaml> [--output text|json]
  bgorch apply    -f <spec.yaml> [-o <out-dir>] [--dry-run] [--output text|json]
  bgorch status   -f <spec.yaml> [--output text|json]
  bgorch doctor   -f <spec.yaml> [--output text|json]

Notes:
  - Current MVP implements plugin: generic-process
  - Current MVP implements backends: docker-compose, ssh-systemd
  - State snapshots and locks are stored in .bgorch/state`)
}
