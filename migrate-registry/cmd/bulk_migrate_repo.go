package cmd

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/spf13/cobra"
)

var (
	workers        int
	dryRun         bool
	continueOnErr  bool
)

var bulkMigrateRepoCmd = &cobra.Command{
	Use:   "bulk-migrate-repo [source_registry/repository] [destination_registry/repository]",
	Short: "Migrate all tags from a source repository to a destination repository",
	Long: `Migrate all tags from a source repository to a destination repository with parallel transfers.

Examples:
  # Migrate all tags from Docker Hub to ADS Registry
  migrate-registry bulk-migrate-repo docker.io/library/nginx registry.example.com/nginx

  # Migrate with 10 parallel workers
  migrate-registry bulk-migrate-repo source.io/repo dest.io/repo --workers 10

  # Dry run to see what would be migrated
  migrate-registry bulk-migrate-repo source.io/repo dest.io/repo --dry-run
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		srcRepo := args[0]
		dstRepo := args[1]

		fmt.Printf("🔍 Fetching tags from %s...\n", srcRepo)
		tags, err := crane.ListTags(srcRepo, getCraneOptions()...)
		if err != nil {
			return fmt.Errorf("listing tags: %w", err)
		}

		if len(tags) == 0 {
			fmt.Println("⚠️  No tags found in source repository")
			return nil
		}

		fmt.Printf("📦 Found %d tags to migrate\n", len(tags))

		if dryRun {
			fmt.Println("\n🔍 DRY RUN - No actual migration will occur")
			for _, tag := range tags {
				fmt.Printf("  Would migrate: %s:%s → %s:%s\n", srcRepo, tag, dstRepo, tag)
			}
			return nil
		}

		// Migrate with parallel workers
		return bulkMigrateTags(srcRepo, dstRepo, tags)
	},
}

func bulkMigrateTags(srcRepo, dstRepo string, tags []string) error {
	var (
		successCount  int64
		failureCount  int64
		wg            sync.WaitGroup
		semaphore     = make(chan struct{}, workers)
		mu            sync.Mutex
		failures      []string
	)

	fmt.Printf("\n🚀 Starting migration with %d workers...\n\n", workers)

	for i, tag := range tags {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire worker slot

		go func(idx int, t string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release worker slot

			src := fmt.Sprintf("%s:%s", srcRepo, t)
			dst := fmt.Sprintf("%s:%s", dstRepo, t)

			if verbose {
				fmt.Printf("[%d/%d] Migrating %s → %s\n", idx+1, len(tags), src, dst)
			}

			if err := crane.Copy(src, dst, getCraneOptions()...); err != nil {
				atomic.AddInt64(&failureCount, 1)
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%s: %v", t, err))
				mu.Unlock()

				if !continueOnErr {
					fmt.Printf("❌ [%d/%d] Failed: %s (%v)\n", idx+1, len(tags), t, err)
				}

				if !continueOnErr {
					return
				}
			} else {
				atomic.AddInt64(&successCount, 1)
				if !verbose {
					fmt.Printf("✓ [%d/%d] %s\n", idx+1, len(tags), t)
				}
			}
		}(i, tag)

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
	fmt.Printf("📦 Total:      %d\n", len(tags))

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
	rootCmd.AddCommand(bulkMigrateRepoCmd)
	bulkMigrateRepoCmd.Flags().IntVarP(&workers, "workers", "w", 5, "Number of parallel workers")
	bulkMigrateRepoCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without actually migrating")
	bulkMigrateRepoCmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "Continue migration even if some tags fail")
}
