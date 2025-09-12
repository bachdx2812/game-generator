package handlers

import (
	"backend/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

		// Step 1: Update game spec state to 'creating' and return immediately
		if err := updateGameSpecState(db, req.GameSpecID, StateCreating, "Code generation job created"); err != nil {
			log.Printf("Failed to update initial state: %v", err)
		}

		// Steps 2-5: Start background processing in goroutine
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
	if !gitRepo.IsConfigured() {
		updateJobStatus(db, jobID, "failed", 0, []string{"Git repository not configured"})
		return
	}

	updateJobStatus(db, jobID, "processing", 60, []string{"Creating game folder with README.md"})

	// Create game folder with README.md (correct function signature: gameID, gameTitle, gameSpec)
	gamePath, err := gitRepo.CreateGameFolder(req.GameSpecID, gameSpec.Title, combinedGameSpec)
	if err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to create game folder: %v", err)})
		return
	}

	updateJobStatus(db, jobID, "processing", 80, []string{"Committing and pushing to repository"})

	// Commit and push changes (correct function signature: gamePath, gameTitle, gameID)
	if err := gitRepo.CommitAndPush(gamePath, gameSpec.Title, req.GameSpecID); err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to commit and push: %v", err)})
		return
	}

	// Step 3: Update to git_inited after successful git operations
	if err := updateGameSpecState(db, req.GameSpecID, StateGitInited, "Git repository initialized and README.md pushed"); err != nil {
		log.Printf("Failed to update to git_inited state: %v", err)
	}

	updateJobStatus(db, jobID, "processing", 85, []string{"Git operations completed, starting Devin code generation"})

	// Step 4: Update to code_generating and create Devin task
	if err := updateGameSpecState(db, req.GameSpecID, StateCodeGenerating, "Starting Devin code generation"); err != nil {
		log.Printf("Failed to update to code_generating state: %v", err)
	}

	// Create Devin task for actual code generation
	sessionID, err := gitRepo.CreateDevinTask(req.GameSpecID, gameSpec.Title)
	if err != nil {
		log.Printf("[ERROR] Failed to create Devin task for spec %s: %v", req.GameSpecID, err)
		updateJobStatus(db, jobID, "failed", 85, []string{fmt.Sprintf("Failed to create Devin task: %v", err)})
		return
	}

	// Store session ID in database
	_, err = db.Exec(ctx, `UPDATE game_specs SET devin_session_id = $1 WHERE id = $2`, sessionID, req.GameSpecID)
	if err != nil {
		log.Printf("[ERROR] Failed to store Devin session ID in database: %v", err)
	}

	updateJobStatus(db, jobID, "processing", 90, []string{fmt.Sprintf("Devin task created with session ID: %s", sessionID)})

	updateJobStatus(db, jobID, "completed", 100, []string{
		"Git repository setup completed and Devin task created",
		fmt.Sprintf("Devin session: https://app.devin.ai/sessions/%s", sessionID),
		"Monitoring Devin progress for completion...",
	})

	log.Printf("[SUCCESS] Code generation pipeline initiated for spec %s with Devin session %s", req.GameSpecID, sessionID)
}

func updateJobStatus(db *pgxpool.Pool, jobID, status string, progress int, logs []string) {
	logsJSON, _ := json.Marshal(logs)
	db.Exec(context.Background(), `
		UPDATE code_jobs
		SET status = $1, progress = $2, logs = $3, updated_at = $4
		WHERE id = $5
	`, status, progress, logsJSON, time.Now(), jobID)
}
