package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

type ConsoleUI struct {
	scanner *bufio.Scanner
}

func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{
		scanner: bufio.NewScanner(os.Stdin),
	}
}

func (c *ConsoleUI) Info(format string, args ...interface{}) {
	fmt.Printf("ℹ️  "+format+"\n", args...)
}

func (c *ConsoleUI) Success(format string, args ...interface{}) {
	// Green text
	fmt.Printf("\033[32m✅ "+format+"\033[0m\n", args...)
}

func (c *ConsoleUI) Warn(format string, args ...interface{}) {
	// Yellow text
	fmt.Printf("\033[33m⚠️  "+format+"\033[0m\n", args...)
}

func (c *ConsoleUI) Error(format string, args ...interface{}) {
	// Red text
	fmt.Fprintf(os.Stderr, "\033[31m❌ "+format+"\033[0m\n", args...)
}

func (c *ConsoleUI) Confirm(question string, defaultYes bool) bool {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	fmt.Printf("%s %s: ", question, suffix)

	if !c.scanner.Scan() {
		return defaultYes
	}

	resp := strings.ToLower(strings.TrimSpace(c.scanner.Text()))
	if resp == "" {
		return defaultYes
	}
	return resp == "y" || resp == "yes"
}

func (c *ConsoleUI) Select(question string, items []string) (int, string, error) {
	fmt.Println(question)
	for i, item := range items {
		fmt.Printf("  [%d] %s\n", i+1, item)
	}

	for {
		fmt.Print("Select an option: ")
		if !c.scanner.Scan() {
			return -1, "", fmt.Errorf("input closed")
		}
		input := strings.TrimSpace(c.scanner.Text())
		idx, err := strconv.Atoi(input)
		if err == nil && idx > 0 && idx <= len(items) {
			return idx - 1, items[idx-1], nil
		}
		fmt.Println("Invalid selection. Please try again.")
	}
}

func (c *ConsoleUI) Input(question string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}

	if !c.scanner.Scan() {
		return defaultValue
	}
	input := strings.TrimSpace(c.scanner.Text())
	if input == "" {
		return defaultValue
	}
	return input
}

func (c *ConsoleUI) Table(headers []string, rows [][]string) {
	// Use standard tabwriter for aligned output
	// minwidth, tabwidth, padding, padchar, flags
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Print Headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print Separator Line
	sep := make([]string, len(headers))
	for i, h := range headers {
		sep[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(w, strings.Join(sep, "\t"))

	// Print Rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

func init() {
	Standard = NewConsoleUI()
}
