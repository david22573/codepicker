package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/batch"
	"github.com/david22573/codepicker/internal/database"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	batchConcurrency int
	batchPriority    int
	batchCleanDays   int
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Manage and execute background job queues",
	Long:  `The batch system allows you to queue multiple agent tasks and execute them concurrently in the background. Jobs run with the restricted 'Batch' policy (no shell access).`,
}

var batchAddCmd = &cobra.Command{
	Use:   "add [task]",
	Short: "Add a task to the queue",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		q := batch.NewQueue(store.DB())

		id, err := q.Add(task, batchPriority)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Job added to queue. ID: %s\n", id)
		fmt.Printf("👉 Run 'codepicker batch run' to process it.\n")
		return nil
	},
}

var batchRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Start processing the job queue",
	RunE: func(cmd *cobra.Command, args []string) error {

		if os.Getenv("OPENROUTER_API_KEY") == "" {
			return fmt.Errorf("OPENROUTER_API_KEY is not set")
		}

		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		q := batch.NewQueue(store.DB())
		absSrc, _ := filepath.Abs(srcDir)

		runner := batch.NewRunner(q, store, appLogger, batchConcurrency, absSrc)

		return runner.Start(cmd.Context())
	},
}

var batchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "View queue status",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		q := batch.NewQueue(store.DB())
		jobs, err := q.List(20)
		if err != nil {
			return err
		}

		if len(jobs) == 0 {
			fmt.Println("Queue is empty.")
			return nil
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"ID", "Pri", "Status", "Task", "Duration", "Created"})

		for _, j := range jobs {
			id := j.ID[:8]
			task := j.Task
			if len(task) > 40 {
				task = task[:37] + "..."
			}

			dur := "-"
			if j.Status == batch.StatusRunning && j.StartedAt != nil {
				dur = time.Since(*j.StartedAt).Round(time.Second).String()
			} else if (j.Status == batch.StatusCompleted || j.Status == batch.StatusFailed) && j.StartedAt != nil && j.CompletedAt != nil {
				dur = j.CompletedAt.Sub(*j.StartedAt).Round(time.Second).String()
			}

			table.Append([]string{
				id,
				strconv.Itoa(j.Priority),
				string(j.Status),
				task,
				dur,
				j.CreatedAt.Format("15:04:05"),
			})
		}
		table.Render()
		return nil
	},
}

var batchCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old completed/failed jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		q := batch.NewQueue(store.DB())
		count, err := q.Clear(time.Duration(batchCleanDays) * 24 * time.Hour)
		if err != nil {
			return err
		}

		fmt.Printf("🧹 Cleaned %d old jobs.\n", count)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(batchCmd)

	batchCmd.AddCommand(batchAddCmd)
	batchAddCmd.Flags().IntVarP(&batchPriority, "priority", "p", 0, "Job priority (higher runs first)")

	batchCmd.AddCommand(batchRunCmd)
	batchRunCmd.Flags().IntVarP(&batchConcurrency, "concurrent", "c", 1, "Number of concurrent workers")

	batchCmd.AddCommand(batchStatusCmd)

	batchCmd.AddCommand(batchCleanCmd)
	batchCleanCmd.Flags().IntVar(&batchCleanDays, "days", 7, "Delete jobs older than N days")
}
