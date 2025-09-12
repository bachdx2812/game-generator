package handlers

import (
	"backend/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateCodeJobReq struct {
	GameSpecID string                 `json:"game_spec_id"`
	GameSpec   map[string]interface{} `json:"game_spec"`
	OutputPath string                 `json:"output_path,omitempty"`
}

type CodeJobStatusResp struct {
	JobID       string    `json:"job_id"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	OutputPath  *string   `json:"output_path,omitempty"`
	ArtifactURL *string   `json:"artifact_url,omitempty"`
	Error       *string   `json:"error,omitempty"`
	Logs        []string  `json:"logs,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func PostCodeJob(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req CreateCodeJobReq
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
		}

		// Validate GameSpec
		if req.GameSpecID == "" && len(req.GameSpec) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Either game_spec_id or game_spec must be provided"})
		}

		// Set default output path
		if req.OutputPath == "" {
			req.OutputPath = "/tmp"
		}

		jobID := uuid.New().String()
		now := time.Now()

		// Insert job into database
		_, err := db.Exec(context.Background(), `
			INSERT INTO code_jobs (id, game_spec_id, game_spec, output_path, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'queued', $5, $6)
		`, jobID, req.GameSpecID, req.GameSpec, req.OutputPath, now, now)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create job"})
		}

		// Start background processing
		go processCodeGeneration(db, jobID, req)

		return c.JSON(fiber.Map{
			"job_id": jobID,
			"status": "queued",
		})
	}
}

func GetCodeJob(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		jobID := c.Params("id")
		if jobID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Job ID is required"})
		}

		var resp CodeJobStatusResp
		err := db.QueryRow(context.Background(), `
			SELECT id, status, progress, artifact_url, error, logs, created_at, updated_at
			FROM code_jobs WHERE id = $1
		`, jobID).Scan(
			&resp.JobID, &resp.Status, &resp.Progress, &resp.ArtifactURL, &resp.Error, &resp.Logs, &resp.CreatedAt, &resp.UpdatedAt,
		)

		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Job not found"})
		}

		return c.JSON(resp)
	}
}

// GetCodeJobBySpecID gets the latest code job for a specific game spec
func GetCodeJobBySpecID(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		specID := c.Params("spec_id")
		if specID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Spec ID is required"})
		}

		var resp CodeJobStatusResp
		err := db.QueryRow(context.Background(), `
			SELECT id, status, progress, output_path, artifact_url, error, logs, created_at, updated_at
			FROM code_jobs
			WHERE game_spec_id = $1
			ORDER BY created_at DESC
			LIMIT 1
		`, specID).Scan(
			&resp.JobID, &resp.Status, &resp.Progress, &resp.OutputPath, &resp.ArtifactURL, &resp.Error, &resp.Logs, &resp.CreatedAt, &resp.UpdatedAt,
		)

		if err != nil {
			// No code job found for this spec
			return c.JSON(fiber.Map{"status": "not_started"})
		}

		return c.JSON(resp)
	}
}

// RetryCodeJob creates a new code generation job for failed ones
func RetryCodeJob(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		specID := c.Params("spec_id")
		if specID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Spec ID is required"})
		}

		// Get the game spec
		var gameSpec map[string]interface{}
		err := db.QueryRow(context.Background(), "SELECT spec_json FROM game_specs WHERE id = $1", specID).Scan(&gameSpec)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Game spec not found"})
		}

		// Create new code job
		req := CreateCodeJobReq{
			GameSpecID: specID,
			GameSpec:   gameSpec,
			OutputPath: "/tmp",
		}

		jobID := uuid.New().String()
		now := time.Now()

		// Insert job into database
		_, err = db.Exec(context.Background(), `
			INSERT INTO code_jobs (id, game_spec_id, game_spec, output_path, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'queued', $5, $6)
		`, jobID, req.GameSpecID, req.GameSpec, req.OutputPath, now, now)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create retry job"})
		}

		// Start background processing
		go processCodeGeneration(db, jobID, req)

		return c.JSON(fiber.Map{
			"job_id":  jobID,
			"status":  "queued",
			"message": "Code generation retry started",
		})
	}
}

func processCodeGeneration(db *pgxpool.Pool, jobID string, req CreateCodeJobReq) {
	updateJobStatus(db, jobID, "processing", 20, []string{"Starting automated git folder generation"})

	// Retrieve game spec from database using GameSpecID
	ctx := context.Background()
	var gameSpec struct {
		ID           string                 `json:"id"`
		Title        string                 `json:"title"`
		SpecMarkdown string                 `json:"spec_markdown"`
		SpecJSON     map[string]interface{} `json:"spec_json"`
	}

	var specJSONBytes []byte
	err := db.QueryRow(ctx, `
		SELECT id, title, spec_markdown, spec_json
		FROM game_specs
		WHERE id = $1
	`, req.GameSpecID).Scan(&gameSpec.ID, &gameSpec.Title, &gameSpec.SpecMarkdown, &specJSONBytes)

	if err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to retrieve game spec: %v", err)})
		return
	}

	// Parse spec_json
	if err := json.Unmarshal(specJSONBytes, &gameSpec.SpecJSON); err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to parse spec JSON: %v", err)})
		return
	}

	updateJobStatus(db, jobID, "processing", 40, []string{"Game spec retrieved successfully"})

	// Create combined game spec for git operations
	combinedGameSpec := make(map[string]interface{})
	combinedGameSpec["spec_json"] = gameSpec.SpecJSON
	combinedGameSpec["spec_markdown"] = gameSpec.SpecMarkdown
	combinedGameSpec["title"] = gameSpec.Title

	// Initialize git repository
	gitRepo := utils.NewGitRepo()
	var outputURL string

	if !gitRepo.IsConfigured() {
		updateJobStatus(db, jobID, "failed", 0, []string{"Git repository not configured. Automated workflow requires git integration."})
		return
	}

	if err := gitRepo.InitializeRepo(); err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to initialize git repo: %v", err)})
		return
	}

	updateJobStatus(db, jobID, "processing", 60, []string{"Git repository initialized"})

	// Extract game title from spec
	gameTitle := "untitled-game"
	if title, ok := combinedGameSpec["title"].(string); ok && title != "" {
		gameTitle = title
	}

	// Create game folder with README containing the game spec
	projectPath, err := gitRepo.CreateGameFolder(req.GameSpecID, gameTitle, combinedGameSpec)
	if err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to create game folder: %v", err)})
		return
	}

	updateJobStatus(db, jobID, "processing", 80, []string{"Game folder created with README.md", fmt.Sprintf("Path: %s", projectPath)})

	// Construct GitHub URL
	repoURL := os.Getenv("GIT_REPO_URL")
	repoURL = strings.TrimSuffix(repoURL, ".git")
	outputURL = fmt.Sprintf("%s/tree/main/%s", repoURL, req.GameSpecID)

	// Commit and push the README-only folder
	if err := gitRepo.CommitAndPush(projectPath, gameTitle, req.GameSpecID); err != nil {
		updateJobStatus(db, jobID, "completed", 100, []string{
			"Automated generation completed",
			"Warning: Failed to push to git repository",
			fmt.Sprintf("Git error: %v", err),
		})
	} else {
		// After successful push, automatically trigger Devin task
		if devinSessionID, err := gitRepo.CreateDevinTask(req.GameSpecID, gameTitle); err != nil {
			log.Printf("Warning: Failed to create Devin task: %v", err)
			updateJobStatus(db, jobID, "completed", 100, []string{
				"Automated generation completed successfully",
				"README.md committed and pushed to git repository",
				fmt.Sprintf("GitHub URL: %s", outputURL),
				"Warning: Failed to create Devin task",
			})
		} else {
			// Update game spec with Devin session ID
			devinSessionID = strings.TrimPrefix(devinSessionID, "devin-")
			_, err := db.Exec(ctx, "UPDATE game_specs SET devin_session_id = $1 WHERE id = $2", devinSessionID, req.GameSpecID)
			if err != nil {
				log.Printf("Warning: Failed to update game spec with Devin session ID: %v", err)
			}

			updateJobStatus(db, jobID, "completed", 100, []string{
				"Automated generation completed successfully",
				"README.md committed and pushed to git repository",
				fmt.Sprintf("GitHub URL: %s", outputURL),
				fmt.Sprintf("Devin task created: %s", devinSessionID),
			})
		}
	}

	// Update output path in database with GitHub URL
	db.Exec(context.Background(), "UPDATE code_jobs SET output_path = $1 WHERE id = $2", outputURL, jobID)
}

func updateJobStatus(db *pgxpool.Pool, jobID, status string, progress int, logs []string) {
	logsJSON, _ := json.Marshal(logs)
	db.Exec(context.Background(), `
		UPDATE code_jobs
		SET status = $1, progress = $2, logs = $3, updated_at = $4
		WHERE id = $5
	`, status, progress, logsJSON, time.Now(), jobID)
}
