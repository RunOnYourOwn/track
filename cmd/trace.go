package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

// taskIDPattern matches PREFIX-NNN anywhere in a string.
var taskIDPattern = regexp.MustCompile(`\b([A-Z]{2,6}-\d+)\b`)

func init() {
	rootCmd.AddCommand(commitsCmd)
	rootCmd.AddCommand(deployCmd)

	commitsCmd.AddCommand(commitsScanCmd)
	deployCmd.AddCommand(deployRecordCmd)
	deployCmd.AddCommand(deployListCmd)

	// commits scan flags
	commitsScanCmd.Flags().String("project", "", "Project prefix (used to filter task IDs)")
	commitsScanCmd.Flags().String("since", "7d", "How far back to scan (e.g. 7d, 14d, 30d)")

	// deploy record flags
	deployRecordCmd.Flags().String("project", "", "Project prefix (required)")
	deployRecordCmd.Flags().String("commit", "", "Commit hash (required)")
	deployRecordCmd.Flags().String("tag", "", "Release tag (e.g. 0.9.0)")
	deployRecordCmd.Flags().String("env", "production", "Environment (production, staging, dev)")
	deployRecordCmd.Flags().String("triggered-by", "human", "Who triggered the deploy")
	_ = deployRecordCmd.MarkFlagRequired("project")
	_ = deployRecordCmd.MarkFlagRequired("commit")

	// deploy list flags
	deployListCmd.Flags().String("project", "", "Project prefix (required)")
	deployListCmd.Flags().Int("limit", 10, "Number of deploys to show")
	_ = deployListCmd.MarkFlagRequired("project")
}

// --- commits ---

var commitsCmd = &cobra.Command{
	Use:   "commits",
	Short: "Commit traceability",
}

var commitsScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan git log and link commits to tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()

		projectPrefix, _ := cmd.Flags().GetString("project")
		sinceFlag, _ := cmd.Flags().GetString("since")

		// Resolve project if provided, so we can restrict to its prefix.
		var projectPrefix2 string
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			projectPrefix2 = p.Prefix
		}

		// Convert e.g. "7d" → "7 days ago"
		sinceGit, err := convertSince(sinceFlag)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}

		// Repo name: basename of cwd.
		cwd, _ := os.Getwd()
		repo := repoName(cwd)

		// Run: git log --format="%H %ai %s" --since=<sinceGit>
		gitArgs := []string{"log", "--format=%H %ai %s", "--since=" + sinceGit}
		out, err := exec.Command("git", gitArgs...).Output()
		if err != nil {
			return fmt.Errorf("git log failed: %w", err)
		}

		var recorded, skipped int
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Format: "<hash> <date+time+tz> <message>"
			// %ai gives "2026-05-27 14:30:00 +0000" (with space-separated parts)
			// We split on first space (hash), next two tokens are date+time, next is tz, rest is subject.
			parts := strings.SplitN(line, " ", 5)
			if len(parts) < 5 {
				continue
			}
			hash := parts[0]
			committedAt := parts[1] + "T" + parts[2] + parts[3] // "2026-05-27T14:30:00+0000"
			// Normalise tz offset: "+0000" → "+00:00"
			committedAt = normalizeTZ(committedAt)
			message := parts[4]

			// Find task IDs in the message
			matches := taskIDPattern.FindAllString(message, -1)
			if len(matches) == 0 {
				continue
			}

			// Optionally restrict to the specified project prefix
			for _, displayID := range matches {
				if projectPrefix2 != "" {
					prefix := strings.SplitN(displayID, "-", 2)[0]
					if !strings.EqualFold(prefix, projectPrefix2) {
						skipped++
						continue
					}
				}

				taskID, err := resolveID(displayID)
				if err != nil {
					// Task not in DB yet — skip silently
					skipped++
					continue
				}

				if err := db.RecordCommit(conn, taskID, hash, repo, committedAt, message, nil); err != nil {
					fmt.Fprintf(os.Stderr, "warning: record commit %s for %s: %v\n", hash[:8], displayID, err)
				} else {
					recorded++
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		fmt.Printf("Scanned git log (since %s): %d commit-task links recorded, %d skipped\n", sinceFlag, recorded, skipped)
		return nil
	},
}

// --- deploy ---

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy traceability",
}

var deployRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a deploy",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()

		projectPrefix, _ := cmd.Flags().GetString("project")
		commitHash, _ := cmd.Flags().GetString("commit")
		tag, _ := cmd.Flags().GetString("tag")
		env, _ := cmd.Flags().GetString("env")
		triggeredBy, _ := cmd.Flags().GetString("triggered-by")

		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		// If a tag is provided, scan git log from previous tag to this tag for task IDs.
		var taskIDs []string
		if tag != "" {
			taskIDs = extractTaskIDsFromTagRange(p.Prefix, tag)
		}

		deploy, err := db.RecordDeploy(conn, p.ID, commitHash, tag, env, triggeredBy, taskIDs)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(deploy)
		}

		fmt.Printf("Deploy recorded: %s → %s", deploy.ID[:8], deploy.Environment)
		if deploy.Tag != "" {
			fmt.Printf(" (tag: %s)", deploy.Tag)
		}
		fmt.Printf("\n  commit: %s\n", deploy.CommitHash)
		if len(taskIDs) > 0 {
			fmt.Printf("  tasks linked: %s\n", strings.Join(taskIDs, ", "))
		}
		return nil
	},
}

var deployListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent deploys",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()

		projectPrefix, _ := cmd.Flags().GetString("project")
		limit, _ := cmd.Flags().GetInt("limit")

		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		deploys, err := db.ListDeploys(conn, p.ID, limit)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(deploys)
		}

		if len(deploys) == 0 {
			fmt.Println("No deploys recorded.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tENV\tTAG\tCOMMIT\tDEPLOYED AT\tTRIGGERED BY")
		for _, d := range deploys {
			shortID := d.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			shortHash := d.CommitHash
			if len(shortHash) > 8 {
				shortHash = shortHash[:8]
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				shortID, d.Environment, d.Tag, shortHash,
				d.DeployedAt.Format("2006-01-02 15:04"), d.TriggeredBy)
		}
		return w.Flush()
	},
}

// --- helpers ---

// convertSince converts strings like "7d", "14d", "30d" to git --since values
// like "7 days ago". Also passes through values that already look like git
// understood strings.
func convertSince(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "7 days ago", nil
	}
	// Handle Nd format
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid duration %q (expected Nd like 7d)", s)
		}
		if n == 1 {
			return "1 day ago", nil
		}
		return fmt.Sprintf("%d days ago", n), nil
	}
	// Handle Nw format
	if strings.HasSuffix(s, "w") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid duration %q (expected Nw like 2w)", s)
		}
		return fmt.Sprintf("%d weeks ago", n), nil
	}
	// Pass through anything else (e.g. "2 weeks ago", a date string)
	return s, nil
}

// repoName returns the last path component of the given directory path.
func repoName(dir string) string {
	dir = strings.TrimSuffix(dir, "/")
	idx := strings.LastIndex(dir, "/")
	if idx < 0 {
		return dir
	}
	return dir[idx+1:]
}

// normalizeTZ converts a timezone offset without colon ("+0000") to one with
// a colon ("+00:00") so time.Parse can handle RFC3339.
func normalizeTZ(s string) string {
	// Find the last + or - that is the timezone offset
	for i := len(s) - 5; i >= 0; i-- {
		if (s[i] == '+' || s[i] == '-') && i > 0 {
			tz := s[i:]
			if len(tz) == 5 && !strings.Contains(tz, ":") {
				return s[:i] + tz[:3] + ":" + tz[3:]
			}
			break
		}
	}
	return s
}

// extractTaskIDsFromTagRange scans git log between the previous tag and the
// given tag, returning task IDs matching the project prefix.
func extractTaskIDsFromTagRange(projectPrefix, tag string) []string {
	// Find the tag before the given one.
	prevTagOut, err := exec.Command("git", "describe", "--tags", "--abbrev=0", tag+"^").Output()
	var gitRange string
	if err == nil {
		prevTag := strings.TrimSpace(string(prevTagOut))
		gitRange = prevTag + ".." + tag
	} else {
		// No previous tag — scan entire history up to this tag.
		gitRange = tag
	}

	var rangeArgs []string
	if strings.Contains(gitRange, "..") {
		rangeArgs = []string{"log", "--format=%s", gitRange}
	} else {
		rangeArgs = []string{"log", "--format=%s", gitRange}
	}

	out, err := exec.Command("git", rangeArgs...).Output()
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var ids []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		matches := taskIDPattern.FindAllString(scanner.Text(), -1)
		for _, m := range matches {
			prefix := strings.SplitN(m, "-", 2)[0]
			if strings.EqualFold(prefix, projectPrefix) && !seen[m] {
				seen[m] = true
				ids = append(ids, m)
			}
		}
	}
	return ids
}

// Ensure time import is used (for format string in deployListCmd).
var _ = time.Now
