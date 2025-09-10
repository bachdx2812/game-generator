package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

		llmvec := os.Getenv("LLMVEC_URL")
		if llmvec == "" {
			llmvec = "http://localhost:8000"
		}

		greq := genSpecReq{Brief: req.Brief, Constraints: req.Constraints}
		gb, _ := json.Marshal(greq)
		resp, err := http.Post(llmvec+"/llm/generate-spec", "application/json", bytes.NewReader(gb))
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
		resp2, err := http.Post(llmvec+"/vector/search", "application/json", bytes.NewReader(sb))
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
		resp3, err := http.Post(llmvec+"/vector/upsert", "application/json", bytes.NewReader(ub))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "vector upsert failed: "+err.Error())
		}
		defer resp3.Body.Close()
		if resp3.StatusCode != 200 {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("upsert status %d", resp3.StatusCode))
		}

		_, _ = db.Exec(ctx, `UPDATE gen_spec_jobs SET status='COMPLETED', result_spec_id=$2, finished_at=now() WHERE id=$1`, jobID, specID)

		return c.Status(201).JSON(fiber.Map{"job_id": jobID, "status": "COMPLETED", "result_spec_id": specID})
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
		ctx := context.Background()
		id := c.Params("id")
		var title, brief, specMD string
		var specJSON map[string]interface{}
		row := db.QueryRow(ctx, `SELECT title,brief,spec_markdown,spec_json FROM game_specs WHERE id=$1`, id)
		if err := row.Scan(&title, &brief, &specMD, &specJSON); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return c.JSON(fiber.Map{"id": id, "title": title, "brief": brief, "spec_markdown": specMD, "spec_json": specJSON})
	}
}
