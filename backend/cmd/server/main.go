package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"

	"backend/internal/db"
	"backend/internal/handlers"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("[WARNING] No .env file found or error loading it:", err)
	}

	ctx := context.Background()

	// Debug: Log the DATABASE_URL being used
	dbDSN := os.Getenv("DATABASE_URL")
	if dbDSN == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	log.Printf("[DEBUG] Connecting to database with DSN: %s", dbDSN)

	pool, err := db.Open(ctx)
	if err != nil {
		log.Fatalf("[ERROR] Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Debug: Test database connection with a simple ping
	log.Println("[DEBUG] Testing database connection...")
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctxTimeout); err != nil {
		log.Fatalf("[ERROR] Database ping failed: %v", err)
	}
	log.Println("[SUCCESS] Database connection established successfully!")

	// Debug: Check if tables exist
	log.Println("[DEBUG] Checking if required tables exist...")
	var tableCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('gen_spec_jobs', 'game_specs')").Scan(&tableCount)
	if err != nil {
		log.Printf("[WARNING] Could not check table existence: %v", err)
	} else {
		log.Printf("[DEBUG] Found %d/2 required tables", tableCount)
		if tableCount < 2 {
			log.Println("[WARNING] Some required tables are missing. Run 'make migrate-up' to create them.")
		}
	}

	app := fiber.New()
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*", AllowHeaders: "*"}))

	api := app.Group("/api")
	api.Post("/spec-jobs", handlers.PostSpecJob(pool))
	api.Get("/spec-jobs/:id", handlers.GetJob(pool))
	api.Get("/specs", handlers.ListSpecs(pool))
	api.Get("/specs/:id", handlers.GetSpec(pool))
	api.Delete("/specs/:id", handlers.DeleteSpec(pool))
	api.Post("/specs/:id/devin-task", handlers.CreateDevinTask(pool))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("[INFO] Server starting on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
