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
	repoPrefix       string
	destPrefix       string
	maxRepos         int
	skipTags         []string
)

var bulkMigrateCatalogCmd = &cobra.Command{
	Use:   "bulk-migrate-catalog [source_registry] [destination_registry]",
	Short: "Migrate multiple repositories from a source registry to a destination",
	Long: `Discover and migrate multiple repositories from a source registry.

This command is useful for migrating entire namespaces or registry catalogs.

Examples:
  # Migrate all repos from source to destination with same structure
  migrate-registry bulk-migrate-catalog source.io dest.io

  # Migrate with a prefix filter (only repos starting with "myorg/")
  migrate-registry bulk-migrate-catalog source.io dest.io --repo-prefix myorg/

  # Migrate and change prefix (myorg/* → neworg/*)
  migrate-registry bulk-migrate-catalog source.io dest.io --repo-prefix myorg/ --dest-prefix neworg/

  # Migrate with 10 parallel workers
  migrate-registry bulk-migrate-catalog source.io dest.io --workers 10
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		srcRegistry := args[0]
		dstRegistry := args[1]

		fmt.Printf("🔍 Discovering repositories from %s...\n", srcRegistry)

		// Note: crane.Catalog requires v1 registry API support
		// Some registries don't implement this endpoint
		repos, err := crane.Catalog(srcRegistry, getCraneOptions()...)
		if err != nil {
			return fmt.Errorf("listing repositories: %w (note: some registries don't support catalog API)", err)
		}

		// Filter by prefix if specified
		var filteredRepos []string
		if repoPrefix != "" {
			for _, repo := range repos {
				if strings.HasPrefix(repo, repoPrefix) {
					filteredRepos = append(filteredRepos, repo)
				}
			}
			fmt.Printf("📦 Found %d repositories (filtered from %d total with prefix '%s')\n",
				len(filteredRepos), len(repos), repoPrefix)
		} else {
			filteredRepos = repos
			fmt.Printf("📦 Found %d repositories\n", len(filteredRepos))
		}

		if len(filteredRepos) == 0 {
			fmt.Println("⚠️  No repositories found")
			return nil
		}

		// Apply max repos limit
		if maxRepos > 0 && len(filteredRepos) > maxRepos {
			fmt.Printf("⚠️  Limiting to first %d repositories (use --max-repos to adjust)\n", maxRepos)
			filteredRepos = filteredRepos[:maxRepos]
		}

		if dryRun {
			fmt.Println("\n🔍 DRY RUN - No actual migration will occur")
			fmt.Println("Repositories that would be migrated:")
			for _, repo := range filteredRepos {
				dstRepo := transformRepoName(repo)
				fmt.Printf("  %s/%s → %s/%s\n", srcRegistry, repo, dstRegistry, dstRepo)
			}
			return nil
		}

		return bulkMigrateCatalog(srcRegistry, dstRegistry, filteredRepos)
	},
}

func transformRepoName(repo string) string {
	if repoPrefix != "" && destPrefix != "" {
		// Replace prefix
		if strings.HasPrefix(repo, repoPrefix) {
			return destPrefix + strings.TrimPrefix(repo, repoPrefix)
		}
	}
	return repo
}

func bulkMigrateCatalog(srcRegistry, dstRegistry string, repos []string) error {
	var (
		successRepoCount int64
		failureRepoCount int64
		totalTagsMigrated int64
		wg               sync.WaitGroup
		semaphore        = make(chan struct{}, workers)
		mu               sync.Mutex
		failures         []string
	)

	fmt.Printf("\n🚀 Starting catalog migration with %d workers...\n\n", workers)

	for i, repo := range repos {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire worker slot

		go func(idx int, r string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release worker slot

			srcRepo := fmt.Sprintf("%s/%s", srcRegistry, r)
			dstRepo := fmt.Sprintf("%s/%s", dstRegistry, transformRepoName(r))

			fmt.Printf("\n[%d/%d] 📦 Processing repository: %s\n", idx+1, len(repos), r)

			// List tags
			tags, err := crane.ListTags(srcRepo, getCraneOptions()...)
			if err != nil {
				atomic.AddInt64(&failureRepoCount, 1)
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%s: failed to list tags: %v", r, err))
				mu.Unlock()
				fmt.Printf("❌ [%d/%d] Failed to list tags for %s\n", idx+1, len(repos), r)
				return
			}

			// Filter out skipped tags
			var validTags []string
			for _, tag := range tags {
				skip := false
				for _, skipTag := range skipTags {
					if tag == skipTag {
						skip = true
						break
					}
				}
				if !skip {
					validTags = append(validTags, tag)
				}
			}

			if len(validTags) == 0 {
				fmt.Printf("⚠️  [%d/%d] No valid tags found in %s\n", idx+1, len(repos), r)
				return
			}

			fmt.Printf("  Found %d tags to migrate\n", len(validTags))

			// Migrate each tag
			tagSuccess := 0
			for _, tag := range validTags {
				src := fmt.Sprintf("%s:%s", srcRepo, tag)
				dst := fmt.Sprintf("%s:%s", dstRepo, tag)

				if err := crane.Copy(src, dst, getCraneOptions()...); err != nil {
					mu.Lock()
					failures = append(failures, fmt.Sprintf("%s:%s: %v", r, tag, err))
					mu.Unlock()
					fmt.Printf("  ❌ Failed: %s\n", tag)

					if !continueOnErr {
						atomic.AddInt64(&failureRepoCount, 1)
						return
					}
				} else {
					tagSuccess++
					fmt.Printf("  ✓ %s\n", tag)
				}
			}

			atomic.AddInt64(&totalTagsMigrated, int64(tagSuccess))
			if tagSuccess == len(validTags) {
				atomic.AddInt64(&successRepoCount, 1)
				fmt.Printf("✓ [%d/%d] Completed %s (%d tags)\n", idx+1, len(repos), r, tagSuccess)
			} else {
				atomic.AddInt64(&failureRepoCount, 1)
				fmt.Printf("⚠️  [%d/%d] Partial success %s (%d/%d tags)\n",
					idx+1, len(repos), r, tagSuccess, len(validTags))
			}
		}(i, repo)

		// If we're not continuing on error and we've had a failure, stop queuing
		if !continueOnErr && atomic.LoadInt64(&failureRepoCount) > 0 {
			break
		}
	}

	wg.Wait()

	// Print summary
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Printf("📊 Catalog Migration Summary\n")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("✓ Successful repositories: %d\n", successRepoCount)
	fmt.Printf("✗ Failed repositories:     %d\n", failureRepoCount)
	fmt.Printf("📦 Total repositories:      %d\n", len(repos))
	fmt.Printf("🏷️  Total tags migrated:    %d\n", totalTagsMigrated)

	if len(failures) > 0 {
		fmt.Println("\n❌ Failures:")
		maxFailuresToShow := 20
		for i, f := range failures {
			if i >= maxFailuresToShow {
				fmt.Printf("  ... and %d more failures\n", len(failures)-maxFailuresToShow)
				break
			}
			fmt.Printf("  - %s\n", f)
		}
		return fmt.Errorf("%d repositories had failures", failureRepoCount)
	}

	fmt.Println("\n✅ All migrations completed successfully!")
	return nil
}

func init() {
	rootCmd.AddCommand(bulkMigrateCatalogCmd)
	bulkMigrateCatalogCmd.Flags().IntVarP(&workers, "workers", "w", 3, "Number of parallel repository workers")
	bulkMigrateCatalogCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without actually migrating")
	bulkMigrateCatalogCmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "Continue migration even if some repositories fail")
	bulkMigrateCatalogCmd.Flags().StringVar(&repoPrefix, "repo-prefix", "", "Only migrate repositories with this prefix")
	bulkMigrateCatalogCmd.Flags().StringVar(&destPrefix, "dest-prefix", "", "Replace repo prefix in destination (requires --repo-prefix)")
	bulkMigrateCatalogCmd.Flags().IntVar(&maxRepos, "max-repos", 0, "Maximum number of repositories to migrate (0 = unlimited)")
	bulkMigrateCatalogCmd.Flags().StringSliceVar(&skipTags, "skip-tags", []string{}, "Tags to skip during migration (e.g., latest,temp)")
}
