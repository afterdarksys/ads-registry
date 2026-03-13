package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

type imagePair struct {
	source      string
	destination string
}

var bulkMigrateListCmd = &cobra.Command{
	Use:   "bulk-migrate-list [file.txt]",
	Short: "Migrate multiple images from a list file",
	Long: `Migrate multiple images specified in a text file with parallel transfers.

File Format:
  Each line should contain either:
  1. Two space/tab-separated values: source_image destination_image
  2. Single image (will use same destination)

  Lines starting with # are treated as comments and ignored.
  Empty lines are ignored.

Examples:
  # File: migrations.txt
  docker.io/library/nginx:latest registry.example.com/nginx:latest
  docker.io/library/redis:7 registry.example.com/redis:7
  docker.io/library/postgres:15 registry.example.com/postgres:15

  # Migrate from file
  migrate-registry bulk-migrate-list migrations.txt

  # With 10 parallel workers
  migrate-registry bulk-migrate-list migrations.txt --workers 10
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filename := args[0]

		pairs, err := parseListFile(filename)
		if err != nil {
			return fmt.Errorf("parsing list file: %w", err)
		}

		if len(pairs) == 0 {
			fmt.Println("⚠️  No valid image pairs found in file")
			return nil
		}

		fmt.Printf("📦 Found %d images to migrate\n", len(pairs))

		if dryRun {
			fmt.Println("\n🔍 DRY RUN - No actual migration will occur")
			for i, pair := range pairs {
				fmt.Printf("  [%d] %s → %s\n", i+1, pair.source, pair.destination)
			}
			return nil
		}

		return bulkMigrateList(pairs)
	},
}

func parseListFile(filename string) ([]imagePair, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var pairs []imagePair
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the line
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		var pair imagePair
		if len(parts) == 1 {
			// Single image - use same for source and dest
			pair = imagePair{
				source:      parts[0],
				destination: parts[0],
			}
		} else if len(parts) >= 2 {
			// Source and destination specified
			pair = imagePair{
				source:      parts[0],
				destination: parts[1],
			}
		} else {
			fmt.Printf("⚠️  Warning: Skipping invalid line %d: %s\n", lineNum, line)
			continue
		}

		pairs = append(pairs, pair)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return pairs, nil
}

func bulkMigrateList(pairs []imagePair) error {
	var (
		successCount int64
		failureCount int64
		wg           sync.WaitGroup
		semaphore    = make(chan struct{}, workers)
		mu           sync.Mutex
		failures     []string
	)

	fmt.Printf("\n🚀 Starting migration with %d workers...\n\n", workers)

	for i, pair := range pairs {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire worker slot

		go func(idx int, p imagePair) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release worker slot

			if verbose {
				fmt.Printf("[%d/%d] Migrating %s → %s\n", idx+1, len(pairs), p.source, p.destination)
			}

			if err := crane.Copy(p.source, p.destination, getCraneOptions()...); err != nil {
				atomic.AddInt64(&failureCount, 1)
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%s → %s: %v", p.source, p.destination, err))
				mu.Unlock()

				fmt.Printf("❌ [%d/%d] Failed: %s\n", idx+1, len(pairs), p.source)

				if !continueOnErr {
					return
				}
			} else {
				atomic.AddInt64(&successCount, 1)
				if !verbose {
					fmt.Printf("✓ [%d/%d] %s\n", idx+1, len(pairs), p.source)
				}
			}
		}(i, pair)

		// If we're not continuing on error and we've had a failure, stop queuing
		if !continueOnErr && atomic.LoadInt64(&failureCount) > 0 {
			break
		}
	}

	wg.Wait()

	// Print summary
	fmt.Println("\n" + strings.Repeat("═", 50))
	fmt.Printf("📊 Migration Summary\n")
	fmt.Println(strings.Repeat("═", 50))
	fmt.Printf("✓ Successful: %d\n", successCount)
	fmt.Printf("✗ Failed:     %d\n", failureCount)
	fmt.Printf("📦 Total:      %d\n", len(pairs))

	if len(failures) > 0 {
		fmt.Println("\n❌ Failures:")
		for _, f := range failures {
			fmt.Printf("  - %s\n", f)
		}
		return fmt.Errorf("%d migrations failed", failureCount)
	}

	fmt.Println("\n✅ All migrations completed successfully!")
	return nil
}

func init() {
	rootCmd.AddCommand(bulkMigrateListCmd)
	bulkMigrateListCmd.Flags().IntVarP(&workers, "workers", "w", 5, "Number of parallel workers")
	bulkMigrateListCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without actually migrating")
	bulkMigrateListCmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "Continue migration even if some images fail")
}
