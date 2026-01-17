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
	"github.com/david22573/codepicker/pkg/openrouter"
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
	Long:  `The batch system allows you to queue multiple agent tasks and execute them concurrently or sequentially in the background.`,
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

		q := batch.NewQueue(store.DB()) // Expose DB from store or add getter

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
		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		q := batch.NewQueue(store.DB())
		client := openrouter.NewClient(apiKey)

		// Get absolute src dir
		absSrc, _ := filepath.Abs(srcDir)

		runner := batch.NewRunner(q, store, client, appLogger, batchConcurrency, absSrc, apiKey)

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
		jobs, err := q.List(20) // List last 20
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
			if j.Status == batch.StatusRunning {
				dur = time.Since(*j.StartedAt).Round(time.Second).String()
			} else if (j.Status == batch.StatusCompleted || j.Status == batch.StatusFailed) && j.StartedAt != nil && j.CompletedAt != nil {
				dur = j.CompletedAt.Sub(*j.StartedAt).Round(time.Second).String()
			}

			// Colorize status
			status := string(j.Status)

			table.Append([]string{
				id,
				strconv.Itoa(j.Priority),
				status,
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
