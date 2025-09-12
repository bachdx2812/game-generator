package handlers

import (
	"backend/internal/utils"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateJobReq struct {
	Brief       string                 `json:"brief"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
}

type JobStatusResp struct {
	Status        string        `json:"status"`
	ResultSpecID  *string       `json:"result_spec_id,omitempty"`
	DuplicateList []SimilarSpec `json:"duplicate_list,omitempty"`
	Error         *string       `json:"error,omitempty"`
}

type SimilarSpec struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

type genSpecReq struct {
	Brief       string                 `json:"brief"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
}
type genSpecResp struct {
	Title        string                 `json:"title"`
	SpecMarkdown string                 `json:"spec_markdown"`
	SpecJSON     map[string]interface{} `json:"spec_json"`
}

type searchReq struct {
	Text      string  `json:"text"`
	TopK      int     `json:"top_k"`
	Threshold float64 `json:"threshold"`
}
type searchResp struct {
	Similar []struct {
		SpecID string  `json:"spec_id"`
		Title  string  `json:"title"`
		Score  float64 `json:"score"`
	} `json:"similar"`
}

type upsertReq struct {
	SpecID  string                 `json:"spec_id"`
	Text    string                 `json:"text"`
	Payload map[string]interface{} `json:"payload"`
}

func hashSpec(specJSON map[string]interface{}) (string, error) {
	b, err := json.Marshal(specJSON)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// State constants
const (
	StateCreating       = "creating"
	StateGitIniting     = "git_initing"
	StateGitInited      = "git_inited"
	StateCodeGenerating = "code_generating"
	StateCodeGenerated  = "code_generated"
)

// Helper function to update game spec state and log the transition
func updateGameSpecState(db *pgxpool.Pool, specID, newState, detail string) error {
	ctx := context.Background()

	// Get current state
	var currentState string
	err := db.QueryRow(ctx, "SELECT state FROM game_specs WHERE id = $1", specID).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("failed to get current state: %v", err)
	}

	// Update game spec state
	_, err = db.Exec(ctx, "UPDATE game_specs SET state = $1 WHERE id = $2", newState, specID)
	if err != nil {
		return fmt.Errorf("failed to update state: %v", err)
	}

	// Log state transition
	_, err = db.Exec(ctx, `
		INSERT INTO game_spec_states (game_spec_id, state_before, state_after, detail)
		VALUES ($1, $2, $3, $4)
	`, specID, currentState, newState, detail)
	if err != nil {
		return fmt.Errorf("failed to log state transition: %v", err)
	}

	log.Printf("[STATE] Spec %s: %s → %s (%s)", specID, currentState, newState, detail)
	return nil
}

func PostSpecJob(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req CreateJobReq
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if req.Brief == "" {
			return fiber.NewError(fiber.StatusBadRequest, "brief is required")
		}

		ctx := context.Background()
		jobID := uuid.New().String()
		_, err := db.Exec(ctx, `INSERT INTO gen_spec_jobs (id,status,brief,created_at) VALUES ($1,'QUEUED',$2,now())`, jobID, req.Brief)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		_, err = db.Exec(ctx, `UPDATE gen_spec_jobs SET status='RUNNING', started_at=now() WHERE id=$1`, jobID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		llmBackend := os.Getenv("LLM_BACKEND_URL")
		if llmBackend == "" {
			llmBackend = "http://localhost:8000"
		}

		greq := genSpecReq{Brief: req.Brief, Constraints: req.Constraints}
		gb, _ := json.Marshal(greq)
		resp, err := http.Post(llmBackend+"/llm/generate-spec", "application/json", bytes.NewReader(gb))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "llm generate-spec failed: "+err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("llm status %d", resp.StatusCode))
		}
		var g genSpecResp
		if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}

		normText := fmt.Sprintf("%s\ncontrols:%v\nmechanics:%v\nconstraints:%v", g.Title, g.SpecJSON["controls"], g.SpecJSON["mechanics"], g.SpecJSON["constraints"])
		topK := 5
		if v := os.Getenv("TOP_K"); v != "" {
			fmt.Sscanf(v, "%d", &topK)
		}
		threshold := 0.86
		if v := os.Getenv("SIM_THRESHOLD"); v != "" {
			fmt.Sscanf(v, "%f", &threshold)
		}
		sreq := searchReq{Text: normText, TopK: topK, Threshold: threshold}
		sb, _ := json.Marshal(sreq)
		resp2, err := http.Post(llmBackend+"/vector/search", "application/json", bytes.NewReader(sb))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "vector search failed: "+err.Error())
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("vector status %d", resp2.StatusCode))
		}
		var s searchResp
		if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}

		if len(s.Similar) > 0 {
			maxScore := s.Similar[0].Score
			if maxScore >= threshold {
				dupIDs := make([]string, 0, len(s.Similar))
				for _, it := range s.Similar {
					dupIDs = append(dupIDs, it.SpecID)
				}
				_, _ = db.Exec(ctx, `UPDATE gen_spec_jobs SET status='DUPLICATE', duplicate_of=$2, score_similarity=$3, finished_at=now() WHERE id=$1`,
					jobID, dupIDs, maxScore)
				list := make([]SimilarSpec, 0, len(s.Similar))
				for _, it := range s.Similar {
					list = append(list, SimilarSpec{ID: it.SpecID, Title: it.Title, Score: it.Score})
				}
				return c.Status(200).JSON(fiber.Map{"job_id": jobID, "status": "DUPLICATE", "duplicate_list": list})
			}
		}

		hash, err := hashSpec(g.SpecJSON)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		specID := uuid.New().String()
		_, err = db.Exec(ctx, `INSERT INTO game_specs (id,title,brief,spec_markdown,spec_json,spec_hash,genre,duration_sec,state)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			specID, g.Title, req.Brief, g.SpecMarkdown, g.SpecJSON, hash, g.SpecJSON["genre"], g.SpecJSON["duration_sec"], StateCreating)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		// Use updateGameSpecState instead of manual insert
		if err := updateGameSpecState(db, specID, StateCreating, "Game spec created"); err != nil {
			log.Printf("Failed to log initial state: %v", err)
		}

		up := upsertReq{SpecID: specID, Text: normText, Payload: map[string]interface{}{"title": g.Title}}
		ub, _ := json.Marshal(up)
		resp3, err := http.Post(llmBackend+"/vector/upsert", "application/json", bytes.NewReader(ub))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "vector upsert failed: "+err.Error())
		}
		defer resp3.Body.Close()
		if resp3.StatusCode != 200 {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("upsert status %d", resp3.StatusCode))
		}

		_, _ = db.Exec(ctx, `UPDATE gen_spec_jobs SET status='COMPLETED', result_spec_id=$2, finished_at=now() WHERE id=$1`, jobID, specID)

		// Always trigger code generation automatically (removed flag check)
		codeJobID := uuid.New().String()
		go func() {
			// Update state to git_initing
			if err := updateGameSpecState(db, specID, StateGitIniting, "Starting git repository initialization"); err != nil {
				log.Printf("Failed to update state to git_initing: %v", err)
			}

			// Initialize git repository
			gitRepo := utils.NewGitRepo()

			codeReq := CreateCodeJobReq{
				GameSpecID: specID,
				GameSpec:   g.SpecJSON,
				OutputPath: gitRepo.RepoPath,
			}

			// Call the existing code generation logic
			now := time.Now()

			// Insert code job
			_, err := db.Exec(context.Background(), `
		INSERT INTO code_jobs (id, game_spec_id, game_spec, output_path, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'queued', $5, $6)
		`, codeJobID, specID, g.SpecJSON, codeReq.OutputPath, now, now)

			if err == nil {
				go processCodeGeneration(db, codeJobID, codeReq)

				log.Printf("[INFO] Auto-triggered code generation job %s for spec %s", codeJobID, specID)
			} else {
				log.Printf("[ERROR] Failed to create code job: %v", err)
			}
		}()

		return c.Status(200).JSON(fiber.Map{"job_id": jobID, "status": "COMPLETED", "result_spec_id": specID})
	}
}

func GetJob(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx := context.Background()
		var status string
		var resultID *string
		var dupIDs []uuid.UUID
		var errStr *string
		row := db.QueryRow(ctx, `SELECT status, result_spec_id, duplicate_of, error FROM gen_spec_jobs WHERE id=$1`, id)
		if err := row.Scan(&status, &resultID, &dupIDs, &errStr); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "job not found")
		}
		resp := JobStatusResp{Status: status, Error: errStr}
		if resultID != nil {
			v := *resultID
			resp.ResultSpecID = &v
		}
		if len(dupIDs) > 0 {
			items := []SimilarSpec{}
			for _, d := range dupIDs {
				var t string
				row := db.QueryRow(ctx, `SELECT title FROM game_specs WHERE id=$1`, d)
				_ = row.Scan(&t)
				items = append(items, SimilarSpec{ID: d.String(), Title: t, Score: 0})
			}
			resp.DuplicateList = items
		}
		return c.JSON(resp)
	}
}

func ListSpecs(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := context.Background()
		rows, err := db.Query(ctx, `
			SELECT id, title, brief, state, created_at
			FROM game_specs
			ORDER BY created_at DESC
			LIMIT 50
		`)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer rows.Close()

		type item struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Brief     string    `json:"brief"`
			State     string    `json:"state"`
			CreatedAt time.Time `json:"created_at"`
		}

		var out []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.Title, &it.Brief, &it.State, &it.CreatedAt); err != nil {
				continue
			}
			out = append(out, it)
		}
		return c.JSON(out)
	}
}

func GetSpec(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx := context.Background()

		var spec struct {
			ID             string  `json:"id"`
			Title          string  `json:"title"`
			Brief          string  `json:"brief"`
			SpecMarkdown   string  `json:"spec_markdown"`
			SpecJSON       []byte  `json:"spec_json"`
			State          string  `json:"state"`
			DevinSessionID *string `json:"devin_session_id"`
		}

		err := db.QueryRow(ctx, `
			SELECT id, title, brief, spec_markdown, spec_json, state, devin_session_id
			FROM game_specs
			WHERE id = $1
		`, id).Scan(&spec.ID, &spec.Title, &spec.Brief, &spec.SpecMarkdown, &spec.SpecJSON, &spec.State, &spec.DevinSessionID)

		if err != nil {
			if err == sql.ErrNoRows {
				return fiber.NewError(fiber.StatusNotFound, "Spec not found")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}

		// Parse spec_json
		var specJSON map[string]interface{}
		if err := json.Unmarshal(spec.SpecJSON, &specJSON); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to parse spec JSON")
		}

		// Fetch state logs
		stateLogsRows, err := db.Query(ctx, `
			SELECT state_before, state_after, detail, created_at
			FROM game_spec_states
			WHERE game_spec_id = $1
			ORDER BY created_at ASC
		`, id)
		if err != nil {
			log.Printf("Error fetching state logs: %v", err)
			// Continue without state logs rather than failing
		}
		defer stateLogsRows.Close()

		type StateLog struct {
			StateBefore *string   `json:"state_before"`
			StateAfter  string    `json:"state_after"`
			Detail      *string   `json:"detail,omitempty"`
			CreatedAt   time.Time `json:"created_at"`
		}

		var stateLogs []StateLog
		for stateLogsRows.Next() {
			var stateLog StateLog
			if err := stateLogsRows.Scan(&stateLog.StateBefore, &stateLog.StateAfter, &stateLog.Detail, &stateLog.CreatedAt); err != nil {
				log.Printf("Error scanning state log: %v", err)
				continue
			}
			stateLogs = append(stateLogs, stateLog)
		}

		response := fiber.Map{
			"id":            spec.ID,
			"title":         spec.Title,
			"brief":         spec.Brief,
			"spec_markdown": spec.SpecMarkdown,
			"spec_json":     specJSON,
			"state":         spec.State,
			"state_logs":    stateLogs,
		}

		// Add Devin session information if available
		if spec.DevinSessionID != nil && *spec.DevinSessionID != "" {
			response["devin_session_id"] = *spec.DevinSessionID
			response["devin_session_url"] = fmt.Sprintf("https://app.devin.ai/sessions/%s", *spec.DevinSessionID)
		}

		return c.JSON(response)
	}
}

// DeleteSpec deletes a game spec from both database and vector database
func DeleteSpec(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx := context.Background()

		// First, check if the spec exists and get its title
		var exists bool
		var gameTitle string
		err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM game_specs WHERE id = $1), COALESCE((SELECT title FROM game_specs WHERE id = $1), '')", id).Scan(&exists, &gameTitle)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}

		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "Spec not found")
		}

		// Initialize git repository for cleanup with enhanced error handling
		gitRepo := utils.NewGitRepo()
		gitCleanupSuccess := false
		if gitRepo.IsConfigured() {
			log.Printf("[INFO] Git repository configured, attempting to remove folder for spec %s", id)
			if err := gitRepo.InitializeRepo(); err != nil {
				log.Printf("[ERROR] Failed to initialize git repo for cleanup: %v", err)
			} else {
				// Find and remove game folders associated with this spec
				if err := gitRepo.RemoveGameFolders(id, gameTitle); err != nil {
					// Log the error but don't fail the deletion
					log.Printf("[ERROR] Failed to remove game folders from git: %v", err)
				} else {
					log.Printf("[SUCCESS] Successfully removed git folder for spec %s", id)
					gitCleanupSuccess = true
				}
			}
		} else {
			log.Printf("[INFO] Git repository not configured, skipping folder cleanup for spec %s", id)
		}

		// Get LLM backend URL
		llmBackend := os.Getenv("LLM_BACKEND_URL")
		if llmBackend == "" {
			llmBackend = "http://localhost:8000"
		}

		// Delete from vector database first
		vectorDeleteURL := fmt.Sprintf("%s/vector/spec/%s", llmBackend, id)
		req, err := http.NewRequest("DELETE", vectorDeleteURL, nil)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create delete request")
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete from vector database")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete from vector database")
		}

		// Delete related code_jobs first to avoid foreign key constraint violation
		_, err = db.Exec(ctx, "DELETE FROM code_jobs WHERE game_spec_id = $1", id)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete related code jobs")
		}

		// Now delete the game spec
		_, err = db.Exec(ctx, "DELETE FROM game_specs WHERE id = $1", id)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete from database")
		}

		// Prepare response with git cleanup status
		response := fiber.Map{
			"message": "Spec deleted successfully",
			"id":      id,
		}

		if gitRepo.IsConfigured() {
			if gitCleanupSuccess {
				response["git_cleanup"] = "success"
			} else {
				response["git_cleanup"] = "failed"
				response["git_cleanup_warning"] = "Git folder may still exist in repository"
			}
		} else {
			response["git_cleanup"] = "skipped - not configured"
		}

		return c.JSON(response)
	}
}

// CreateDevinTask creates a Devin task for a specific game spec
func CreateDevinTask(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		specID := c.Params("id")
		if specID == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "Spec ID is required",
			})
		}

		ctx := context.Background()

		// Check if spec exists and get spec content
		var gameTitle, specContent string
		err := db.QueryRow(ctx, `SELECT title, spec_markdown FROM game_specs WHERE id = $1`, specID).Scan(&gameTitle, &specContent)
		if err != nil {
			if err == sql.ErrNoRows {
				return c.Status(404).JSON(fiber.Map{
					"error": "Game spec not found",
				})
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Database error",
			})
		}

		// Initialize git repository
		gitRepo := utils.NewGitRepo()
		if !gitRepo.IsConfigured() {
			return c.Status(400).JSON(fiber.Map{
				"error": "Git repository not configured. Devin tasks require git integration.",
			})
		}

		// Create Devin task and get session ID
		sessionID, err := gitRepo.CreateDevinTask(specID, gameTitle)
		if err != nil {
			log.Printf("[ERROR] Failed to create Devin task for spec %s: %v", specID, err)
			return c.Status(500).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to create Devin task: %v", err),
			})
		}

		log.Printf("[DEBUG] Original session ID from Devin: '%s' (length: %d)", sessionID, len(sessionID))

		_, err = db.Exec(ctx, `UPDATE game_specs SET devin_session_id = $1 WHERE id = $2`, sessionID, specID)
		if err != nil {
			log.Printf("[ERROR] Failed to store Devin session ID in database: %v", err)
			// Don't fail the request since the task was created successfully
		}

		log.Printf("[SUCCESS] Created Devin task for game spec %s (%s) with session ID: %s", specID, gameTitle, sessionID)

		// Get repository URL for response
		repoURL := os.Getenv("GIT_REPO_URL")
		cleanRepoURL := strings.TrimSuffix(repoURL, ".git")

		return c.JSON(fiber.Map{
			"message":     "Devin task created successfully",
			"spec_id":     specID,
			"game_title":  gameTitle,
			"session_id":  sessionID,
			"session_url": fmt.Sprintf("https://app.devin.ai/sessions/%s", sessionID),
			"repository":  fmt.Sprintf("%s/tree/main/%s", cleanRepoURL, specID),
			"status":      "success",
		})
	}
}

func GetSpecStateLogs(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx := context.Background()

		// Check if spec exists
		var exists bool
		err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM game_specs WHERE id = $1)", id).Scan(&exists)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "Spec not found")
		}

		// Fetch state logs
		rows, err := db.Query(ctx, `
			SELECT state_before, state_after, detail, created_at
			FROM game_spec_states
			WHERE game_spec_id = $1
			ORDER BY created_at ASC
		`, id)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch state logs")
		}
		defer rows.Close()

		type StateLog struct {
			StateBefore *string   `json:"state_before"`
			StateAfter  string    `json:"state_after"`
			Detail      *string   `json:"detail,omitempty"`
			CreatedAt   time.Time `json:"created_at"`
		}

		var stateLogs []StateLog
		for rows.Next() {
			var stateLog StateLog
			if err := rows.Scan(&stateLog.StateBefore, &stateLog.StateAfter, &stateLog.Detail, &stateLog.CreatedAt); err != nil {
				continue
			}
			stateLogs = append(stateLogs, stateLog)
		}

		return c.JSON(fiber.Map{
			"spec_id":    id,
			"state_logs": stateLogs,
		})
	}
}
