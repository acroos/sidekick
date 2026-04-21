package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/austinroos/sidekick/pkg/config"
)

// varFlags collects repeatable --var KEY=VALUE flags.
type varFlags []string

func (v *varFlags) String() string { return strings.Join(*v, ", ") }
func (v *varFlags) Set(val string) error {
	*v = append(*v, val)
	return nil
}

func newClient() (*Client, error) {
	cfg, err := config.LoadClient()
	if err != nil {
		return nil, err
	}
	return NewClient(cfg.ServerURL, cfg.APIKey), nil
}

// RunSubmit handles the "submit" subcommand.
func RunSubmit(args []string) error {
	fs := flag.NewFlagSet("submit", flag.ExitOnError)
	workflow := fs.String("workflow", "", "Workflow name (required)")
	webhook := fs.String("webhook", "", "Webhook URL for completion notification")
	follow := fs.Bool("follow", false, "Stream events after submission")
	var vars varFlags
	fs.Var(&vars, "var", "Variable in KEY=VALUE format (repeatable)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sidekick submit --workflow NAME [--var KEY=VALUE]... [--webhook URL] [--follow]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *workflow == "" {
		fs.Usage()
		return fmt.Errorf("--workflow is required")
	}

	variables := make(map[string]string)
	for _, v := range vars {
		key, value, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("invalid --var format %q (expected KEY=VALUE)", v)
		}
		variables[key] = value
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	task, err := client.Submit(*workflow, variables, *webhook)
	if err != nil {
		return fmt.Errorf("submitting task: %w", err)
	}

	fmt.Printf("Task created: %s\n", task.ID)
	fmt.Printf("Status: %s\n", task.Status)

	if *follow {
		fmt.Println()
		return StreamEvents(client, task.ID, "", os.Stdout)
	}

	return nil
}

// RunStatus handles the "status" subcommand.
func RunStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sidekick status <task-id>")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		// No task ID — list tasks.
		return runList()
	}

	taskID := fs.Arg(0)

	client, err := newClient()
	if err != nil {
		return err
	}

	task, err := client.Get(taskID)
	if err != nil {
		return fmt.Errorf("getting task: %w", err)
	}

	fmt.Print(FormatTask(task))
	return nil
}

func runList() error {
	client, err := newClient()
	if err != nil {
		return err
	}

	tasks, err := client.List("", "", 20)
	if err != nil {
		return fmt.Errorf("listing tasks: %w", err)
	}

	fmt.Print(FormatTaskList(tasks))
	return nil
}

// RunLogs handles the "logs" subcommand.
func RunLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	types := fs.String("types", "", "Comma-separated event types to filter")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sidekick logs <task-id> [--types step.started,step.completed,...]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("task ID is required")
	}

	taskID := fs.Arg(0)

	client, err := newClient()
	if err != nil {
		return err
	}

	return StreamEvents(client, taskID, *types, os.Stdout)
}
