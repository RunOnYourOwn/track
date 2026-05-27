package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/RunOnYourOwn/track/internal/models"
)

// RecordCommit inserts or replaces a commit association for a task.
// filesChanged is marshalled to a JSON array.
func RecordCommit(db *sql.DB, taskID, commitHash, repo, committedAt, message string, filesChanged []string) error {
	fc, err := json.Marshal(filesChanged)
	if err != nil {
		return fmt.Errorf("marshal files_changed: %w", err)
	}
	_, err = db.Exec(`
		INSERT OR REPLACE INTO task_commits (task_id, commit_hash, repo, committed_at, message, files_changed)
		VALUES (?, ?, ?, ?, ?, ?)`,
		taskID, commitHash, repo, committedAt, message, string(fc))
	return err
}

// ListCommitsForTask returns all commits associated with a task, newest first.
func ListCommitsForTask(db *sql.DB, taskID string) ([]models.TaskCommit, error) {
	rows, err := db.Query(`
		SELECT task_id, commit_hash, repo, committed_at, message, files_changed
		FROM task_commits
		WHERE task_id = ?
		ORDER BY committed_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commits []models.TaskCommit
	for rows.Next() {
		var c models.TaskCommit
		var committedAt string
		if err := rows.Scan(&c.TaskID, &c.CommitHash, &c.Repo, &committedAt, &c.Message, &c.FilesChanged); err != nil {
			return nil, err
		}
		c.CommittedAt, _ = parseTime(committedAt)
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// RecordDeploy inserts a deploy record and returns the created Deploy.
// taskIDs is marshalled to a JSON array.
func RecordDeploy(db *sql.DB, projectID, commitHash, tag, environment, triggeredBy string, taskIDs []string) (*models.Deploy, error) {
	if environment == "" {
		environment = "production"
	}
	if triggeredBy == "" {
		triggeredBy = "human"
	}

	ids, err := json.Marshal(taskIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal task_ids: %w", err)
	}

	id := NewID()
	now := Now()

	_, err = db.Exec(`
		INSERT INTO deploys (id, project_id, environment, deployed_at, commit_hash, tag, task_ids, triggered_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, environment, now, commitHash, tag, string(ids), triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("record deploy: %w", err)
	}

	return GetDeploy(db, id)
}

// GetDeploy returns a single deploy by ID.
func GetDeploy(db *sql.DB, id string) (*models.Deploy, error) {
	row := db.QueryRow(`
		SELECT id, project_id, environment, deployed_at, commit_hash, tag, task_ids, triggered_by
		FROM deploys WHERE id = ?`, id)
	return scanDeploy(row)
}

// ListDeploys returns the most recent deploys for a project, capped at limit.
// Pass limit <= 0 for no cap.
func ListDeploys(db *sql.DB, projectID string, limit int) ([]models.Deploy, error) {
	query := `
		SELECT id, project_id, environment, deployed_at, commit_hash, tag, task_ids, triggered_by
		FROM deploys
		WHERE project_id = ?
		ORDER BY deployed_at DESC`
	var args []any
	args = append(args, projectID)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deploys []models.Deploy
	for rows.Next() {
		var d models.Deploy
		var deployedAt string
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Environment, &deployedAt, &d.CommitHash, &d.Tag, &d.TaskIDs, &d.TriggeredBy); err != nil {
			return nil, err
		}
		d.DeployedAt, _ = parseTime(deployedAt)
		deploys = append(deploys, d)
	}
	return deploys, rows.Err()
}

type deployScanner interface {
	Scan(dest ...any) error
}

func scanDeploy(row deployScanner) (*models.Deploy, error) {
	var d models.Deploy
	var deployedAt string
	if err := row.Scan(&d.ID, &d.ProjectID, &d.Environment, &deployedAt, &d.CommitHash, &d.Tag, &d.TaskIDs, &d.TriggeredBy); err != nil {
		return nil, err
	}
	d.DeployedAt, _ = parseTime(deployedAt)
	return &d, nil
}
