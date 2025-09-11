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
		_, err = db.Exec(ctx, `INSERT INTO game_specs (id,title,brief,spec_markdown,spec_json,spec_hash,genre,duration_sec)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			specID, g.Title, req.Brief, g.SpecMarkdown, g.SpecJSON, hash, g.SpecJSON["genre"], g.SpecJSON["duration_sec"])
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
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

		var codeJobID string
		if os.Getenv("AUTO_CODE_GENERATION") == "true" {
			codeJobID = uuid.New().String()
			go func() {
				// Initialize git repository
				gitRepo := utils.NewGitRepo()
				outputPath := "/tmp" // fallback to /tmp if git not configured

				if gitRepo.IsConfigured() {
					if err := gitRepo.InitializeRepo(); err != nil {
						log.Printf("Failed to initialize git repo: %v", err)
					} else {
						outputPath = gitRepo.RepoPath
					}
				}

				codeReq := CreateCodeJobReq{
					GameSpecID: specID,
					GameSpec:   g.SpecJSON,
					OutputPath: outputPath,
				}

				// Call the existing code generation logic
				now := time.Now()

				// Insert code job
				_, err := db.Exec(context.Background(), `
		INSERT INTO code_jobs (id, game_spec_id, game_spec, output_path, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'queued', $5, $6)
		`, codeJobID, specID, g.SpecJSON, codeReq.OutputPath, now, now)

				if err == nil {
					// Start background code generation
					go processCodeGeneration(db, codeJobID, codeReq)
					log.Printf("[INFO] Auto-triggered code generation job %s for spec %s", codeJobID, specID)
				}
			}()
		}

		response := fiber.Map{
			"job_id":         jobID,
			"status":         "COMPLETED",
			"result_spec_id": specID,
		}

		if os.Getenv("AUTO_CODE_GENERATION") == "true" {
			response["code_generation"] = "triggered"
			response["code_job_id"] = codeJobID
		}

		return c.Status(201).JSON(response)
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
		rows, err := db.Query(ctx, `SELECT id,title,created_at FROM game_specs ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer rows.Close()
		type item struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			CreatedAt time.Time `json:"created_at"`
		}
		var out []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.Title, &it.CreatedAt); err != nil {
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
			ID           string `json:"id"`
			Title        string `json:"title"`
			Brief        string `json:"brief"`
			SpecMarkdown string `json:"spec_markdown"`
			SpecJSON     []byte `json:"spec_json"`
		}

		err := db.QueryRow(ctx, `
			SELECT id, title, brief, spec_markdown, spec_json
			FROM game_specs
			WHERE id = $1
		`, id).Scan(&spec.ID, &spec.Title, &spec.Brief, &spec.SpecMarkdown, &spec.SpecJSON)

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

		return c.JSON(fiber.Map{
			"id":            spec.ID,
			"title":         spec.Title,
			"brief":         spec.Brief,
			"spec_markdown": spec.SpecMarkdown,
			"spec_json":     specJSON,
		})
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
