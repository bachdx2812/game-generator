package handlers

import (
	"backend/internal/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

type LLMCodeRequest struct {
	GameSpec     map[string]interface{} `json:"game_spec"`
	OutputFormat string                 `json:"output_format"`
}

type GeneratedFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	FileType string `json:"file_type"`
}

type LLMCodeResponse struct {
	Success           bool                   `json:"success"`
	Files             []GeneratedFile        `json:"files"`
	ProjectStructure  map[string]interface{} `json:"project_structure"`
	BuildInstructions string                 `json:"build_instructions"`
	Error             *string                `json:"error,omitempty"`
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
	updateJobStatus(db, jobID, "processing", 10, []string{"Starting LLM-based code generation"})

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

	// Create combined game spec for LLM and git operations
	combinedGameSpec := make(map[string]interface{})
	combinedGameSpec["spec_json"] = gameSpec.SpecJSON
	combinedGameSpec["spec_markdown"] = gameSpec.SpecMarkdown
	combinedGameSpec["title"] = gameSpec.Title

	// Call LLM for code generation
	llmResp, err := callLLMCodeGeneration(combinedGameSpec)
	if err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to call LLM: %v", err)})
		return
	}

	if !llmResp.Success {
		errorMsg := "Unknown error"
		if llmResp.Error != nil {
			errorMsg = *llmResp.Error
		}
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("LLM generation failed: %s", errorMsg)})
		return
	}

	updateJobStatus(db, jobID, "processing", 60, []string{"Code generated by LLM", fmt.Sprintf("Generated %d files", len(llmResp.Files))})

	// Initialize git repository and create project path
	gitRepo := utils.NewGitRepo()
	var projectPath string
	var outputURL string

	if gitRepo.IsConfigured() {
		// Use git repository
		if err := gitRepo.InitializeRepo(); err != nil {
			updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to initialize git repo: %v", err)})
			return
		}

		// Extract game title from spec
		gameTitle := "untitled-game"
		if title, ok := combinedGameSpec["title"].(string); ok && title != "" {
			gameTitle = title
		}

		// Pass the combined game spec to CreateGameFolder
		projectPath, err = gitRepo.CreateGameFolder(req.GameSpecID, gameTitle, combinedGameSpec)
		if err != nil {
			updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to create game folder: %v", err)})
			return
		}

		// Construct GitHub URL directly: GIT_REPO_URL + '/tree/main/' + gameSpecID
		repoURL := os.Getenv("GIT_REPO_URL")
		repoURL = strings.TrimSuffix(repoURL, ".git")
		outputURL = fmt.Sprintf("%s/tree/main/%s", repoURL, req.GameSpecID)
	} else {
		// Fallback to /tmp
		projectPath = filepath.Join(req.OutputPath, fmt.Sprintf("game_%s_%s", jobID[:8], time.Now().Format("20060102_150405")))
		err = os.MkdirAll(projectPath, 0755)
		if err != nil {
			updateJobStatus(db, jobID, "failed", 0, []string{"Failed to create project directory"})
			return
		}
		outputURL = projectPath
	}

	updateJobStatus(db, jobID, "processing", 70, []string{"Project directory created", fmt.Sprintf("Path: %s", projectPath)})

	// Write generated files to disk
	err = writeGeneratedFiles(projectPath, llmResp.Files)
	if err != nil {
		updateJobStatus(db, jobID, "failed", 0, []string{fmt.Sprintf("Failed to write files: %v", err)})
		return
	}

	updateJobStatus(db, jobID, "processing", 90, []string{"Files written to disk", fmt.Sprintf("Build instructions: %s", llmResp.BuildInstructions)})

	// Git operations if configured
	if gitRepo.IsConfigured() {
		gameTitle := "untitled-game"
		if title, ok := combinedGameSpec["title"].(string); ok && title != "" {
			gameTitle = title
		}

		// Commit and push using the gitRepo.CommitAndPush method
		if err := gitRepo.CommitAndPush(projectPath, gameTitle, req.GameSpecID); err != nil {
			updateJobStatus(db, jobID, "completed", 100, []string{
				"Code generation completed",
				"Warning: Failed to push to git repository",
				fmt.Sprintf("Git error: %v", err),
			})
		} else {
			// After successful push, trigger Devin task if configured
			repoURL := os.Getenv("GIT_REPO_URL")
			if err := gitRepo.CreateDevinTask(req.GameSpecID, gameTitle, repoURL); err != nil {
				log.Printf("Warning: Failed to create Devin task: %v", err)
			}

			updateJobStatus(db, jobID, "completed", 100, []string{
				"Code generation completed successfully",
				"Files committed and pushed to git repository",
				fmt.Sprintf("GitHub URL: %s", outputURL),
			})
		}
	} else {
		updateJobStatus(db, jobID, "completed", 100, []string{"LLM-based code generation completed successfully"})
	}

	// Update output path in database with GitHub URL
	db.Exec(context.Background(), "UPDATE code_jobs SET output_path = $1 WHERE id = $2", outputURL, jobID)
}

func callLLMCodeGeneration(gameSpec map[string]interface{}) (*LLMCodeResponse, error) {
	llmURL := os.Getenv("LLM_BACKEND_URL")
	if llmURL == "" {
		llmURL = "http://localhost:8000"
	}

	// Prepare request
	reqData := LLMCodeRequest{
		GameSpec:     gameSpec,
		OutputFormat: "files",
	}

	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Log the request for debugging
	fmt.Printf("[DEBUG] Calling LLM service at %s with game spec: %s\n", llmURL, gameSpec["title"])

	// Make HTTP request to LLM service
	resp, err := http.Post(llmURL+"/llm/generate-code", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM service: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[ERROR] LLM service returned status %d: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("LLM service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read the full response body first
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Log response size and first few characters for debugging
	fmt.Printf("[DEBUG] LLM service response size: %d bytes\n", len(body))
	if len(body) == 0 {
		return nil, fmt.Errorf("LLM service returned empty response body")
	}

	// Log first 200 characters of response for debugging
	preview := string(body)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("[DEBUG] LLM response preview: %s\n", preview)

	// Parse response
	var llmResp LLMCodeResponse
	err = json.Unmarshal(body, &llmResp)
	if err != nil {
		fmt.Printf("[ERROR] Failed to parse LLM response as JSON: %v\n", err)
		fmt.Printf("[ERROR] Raw response body: %s\n", string(body))
		return nil, fmt.Errorf("failed to decode LLM response: %v", err)
	}

	return &llmResp, nil
}

func writeGeneratedFiles(projectPath string, files []GeneratedFile) error {
	for _, file := range files {
		filePath := filepath.Join(projectPath, file.Path)

		// Create directory if it doesn't exist
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}

		// Write file content
		if err := os.WriteFile(filePath, []byte(file.Content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filePath, err)
		}
	}
	return nil
}

func updateJobStatus(db *pgxpool.Pool, jobID, status string, progress int, logs []string) {
	logsJSON, _ := json.Marshal(logs)
	db.Exec(context.Background(), `
		UPDATE code_jobs
		SET status = $1, progress = $2, logs = $3, updated_at = $4
		WHERE id = $5
	`, status, progress, logsJSON, time.Now(), jobID)
}
