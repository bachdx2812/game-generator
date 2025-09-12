package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitRepo struct {
	RepoPath string
	RepoURL  string
	Username string
	Token    string
	AutoPush bool
}

func NewGitRepo() *GitRepo {
	return &GitRepo{
		RepoPath: os.Getenv("GIT_REPO_PATH"),
		RepoURL:  os.Getenv("GIT_REPO_URL"),
		Username: os.Getenv("GIT_USERNAME"),
		Token:    os.Getenv("GIT_TOKEN"),
	}
}

func (g *GitRepo) IsConfigured() bool {
	return g.RepoPath != "" && g.RepoURL != "" && g.Token != ""
}

func (g *GitRepo) getAuthenticatedURL() (string, error) {
	if g.Token == "" {
		return g.RepoURL, nil
	}

	// Parse the original URL
	parsedURL, err := url.Parse(g.RepoURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse repository URL: %v", err)
	}

	// Add token authentication to URL
	if g.Username != "" {
		parsedURL.User = url.UserPassword(g.Username, g.Token)
	} else {
		// For GitHub, you can use token as username with empty password
		parsedURL.User = url.UserPassword(g.Token, "")
	}

	return parsedURL.String(), nil
}

// pullFromRemote pulls the latest changes from remote repository
func (g *GitRepo) pullFromRemote() error {
	// Check if we have a remote configured
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		// No remote configured, skip pull
		return nil
	}

	// Check if we have any commits
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		// No commits yet, skip pull
		return nil
	}

	// Try to pull from main branch first
	cmd = exec.Command("git", "pull", "origin", "main")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		// Try master branch if main fails
		cmd = exec.Command("git", "pull", "origin", "master")
		cmd.Dir = g.RepoPath
		if err := cmd.Run(); err != nil {
			// If both fail, try a simple pull
			cmd = exec.Command("git", "pull")
			cmd.Dir = g.RepoPath
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to pull from remote: %v", err)
			}
		}
	}

	return nil
}

func (g *GitRepo) InitializeRepo() error {
	if _, err := os.Stat(g.RepoPath); os.IsNotExist(err) {
		err := os.MkdirAll(g.RepoPath, 0755)
		if err != nil {
			return fmt.Errorf("failed to create repo directory: %v", err)
		}
	}

	// Check if it's already a git repository
	gitDir := filepath.Join(g.RepoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Initialize git repository
		cmd := exec.Command("git", "init")
		cmd.Dir = g.RepoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize git repo: %v", err)
		}

		// Set default branch to main
		cmd = exec.Command("git", "branch", "-M", "main")
		cmd.Dir = g.RepoPath
		cmd.Run() // Ignore error as this might fail on older git versions

		// Add remote origin with authentication if URL is provided
		if g.RepoURL != "" {
			authURL, err := g.getAuthenticatedURL()
			if err != nil {
				return fmt.Errorf("failed to create authenticated URL: %v", err)
			}

			cmd = exec.Command("git", "remote", "add", "origin", authURL)
			cmd.Dir = g.RepoPath
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to add remote origin: %v", err)
			}
		}
	} else {
		// Update remote URL with authentication if needed
		if g.RepoURL != "" && g.Token != "" {
			authURL, err := g.getAuthenticatedURL()
			if err != nil {
				return fmt.Errorf("failed to create authenticated URL: %v", err)
			}

			cmd := exec.Command("git", "remote", "set-url", "origin", authURL)
			cmd.Dir = g.RepoPath
			cmd.Run() // Ignore error in case remote doesn't exist
		}
	}

	// Configure git user if not already set
	if g.Username != "" {
		cmd := exec.Command("git", "config", "user.name", g.Username)
		cmd.Dir = g.RepoPath
		cmd.Run() // Ignore error

		cmd = exec.Command("git", "config", "user.email", fmt.Sprintf("%s@users.noreply.github.com", g.Username))
		cmd.Dir = g.RepoPath
		cmd.Run() // Ignore error
	}

	return nil
}

// CreateGameFolder creates a folder using gameID as the folder name with detailed game spec content
func (g *GitRepo) CreateGameFolder(gameID, gameTitle string, gameSpec map[string]interface{}) (string, error) {
	// Use gameID directly as folder name for better control
	gamePath := filepath.Join(g.RepoPath, gameID)

	err := os.MkdirAll(gamePath, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create game folder: %v", err)
	}

	// Create a comprehensive README.md file with game spec content
	readmePath := filepath.Join(gamePath, "README.md")

	// Build README content with game spec details
	var readmeContent strings.Builder
	readmeContent.WriteString(fmt.Sprintf("# %s\n\n", gameTitle))
	readmeContent.WriteString(fmt.Sprintf("**Game ID:** %s\n", gameID))
	readmeContent.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Add spec_markdown content if available
	if specMarkdown, ok := gameSpec["spec_markdown"].(string); ok && specMarkdown != "" {
		readmeContent.WriteString("## Game Specification\n\n")
		readmeContent.WriteString(specMarkdown)
		readmeContent.WriteString("\n\n")
	}

	// Add spec_json content if available
	if specJSON := gameSpec["spec_json"]; specJSON != nil {
		readmeContent.WriteString("## Game Configuration (JSON)\n\n")
		readmeContent.WriteString("```json\n")

		// Convert spec_json to formatted JSON string
		if jsonBytes, err := json.MarshalIndent(specJSON, "", "  "); err == nil {
			readmeContent.WriteString(string(jsonBytes))
		} else {
			// Fallback to basic string representation
			readmeContent.WriteString(fmt.Sprintf("%+v", specJSON))
		}

		readmeContent.WriteString("\n```\n\n")
	}

	if err := os.WriteFile(readmePath, []byte(readmeContent.String()), 0644); err != nil {
		// Don't fail if README creation fails, just log it
		fmt.Printf("Warning: failed to create README.md: %v\n", err)
	}

	return gamePath, nil
}

func (g *GitRepo) CommitAndPush(gamePath, gameTitle, gameID string) error {
	// Pull latest changes before making new commits
	if err := g.pullFromRemote(); err != nil {
		return fmt.Errorf("failed to pull latest changes: %v", err)
	}

	// Add all files in the game folder (using gameID as folder name)
	cmd := exec.Command("git", "add", gameID)
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add files to git: %v", err)
	}

	// Create commit message
	commitTemplate := os.Getenv("GIT_COMMIT_MESSAGE_TEMPLATE")
	if commitTemplate == "" {
		commitTemplate = "Generated game: %s (ID: %s)"
	}
	commitMessage := fmt.Sprintf(commitTemplate, gameTitle, gameID)

	// Commit changes
	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	// Try to push to main branch first
	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		// Try 'master' branch if 'main' fails
		cmd = exec.Command("git", "push", "origin", "master")
		cmd.Dir = g.RepoPath
		if err := cmd.Run(); err != nil {
			// Try to push and set upstream
			cmd = exec.Command("git", "push", "-u", "origin", "main")
			cmd.Dir = g.RepoPath
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to push to remote: %v", err)
			}
		}
	}

	return nil
}

// RemoveGameFolders removes the folder with the exact gameID
func (g *GitRepo) RemoveGameFolders(gameID, gameTitle string) error {
	if !g.IsConfigured() {
		return fmt.Errorf("git repository not configured")
	}

	log.Printf("[INFO] Starting git folder removal for gameID: %s, title: %s", gameID, gameTitle)

	// Pull latest changes before making deletions
	if err := g.pullFromRemote(); err != nil {
		log.Printf("[WARNING] Failed to pull latest changes before deletion: %v", err)
		// Continue with deletion even if pull fails
	}

	// Check if the folder exists
	folderPath := filepath.Join(g.RepoPath, gameID)
	log.Printf("[INFO] Checking for folder at path: %s", folderPath)

	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		// Folder doesn't exist, nothing to remove
		log.Printf("[INFO] Folder %s does not exist, nothing to remove", gameID)
		return nil
	}

	log.Printf("[INFO] Found folder %s, proceeding with removal", gameID)

	// Remove the folder
	if err := os.RemoveAll(folderPath); err != nil {
		return fmt.Errorf("failed to remove folder %s: %v", gameID, err)
	}

	log.Printf("[INFO] Successfully removed folder from filesystem: %s", gameID)

	// Stage the deletion
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stage deletion: %v", err)
	}

	log.Printf("[INFO] Staged deletion for git commit")

	// Check if there are any changes to commit
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err == nil {
		// No changes to commit
		log.Printf("[INFO] No changes to commit after staging deletion")
		return nil
	}

	// Commit the deletion
	commitTemplate := os.Getenv("GIT_COMMIT_MESSAGE_TEMPLATE")
	if commitTemplate == "" {
		commitTemplate = "Removed game folder for deleted spec: %s (ID: %s)"
	}
	commitMessage := fmt.Sprintf(commitTemplate, gameTitle, gameID)

	log.Printf("[INFO] Committing deletion with message: %s", commitMessage)

	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit folder deletion: %v", err)
	}

	log.Printf("[INFO] Successfully committed folder deletion")

	// Push to remote if auto-push is enabled
	if g.AutoPush {
		log.Printf("[INFO] Auto-push enabled, pushing deletion to remote")
		// Try to push to main branch first
		cmd = exec.Command("git", "push", "origin", "main")
		cmd.Dir = g.RepoPath
		if err := cmd.Run(); err != nil {
			// Try 'master' branch if 'main' fails
			cmd = exec.Command("git", "push", "origin", "master")
			cmd.Dir = g.RepoPath
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to push deletion to remote: %v", err)
			}
		}
		log.Printf("[INFO] Successfully pushed folder deletion to remote")
	} else {
		log.Printf("[INFO] Auto-push disabled, deletion committed locally only")
	}

	return nil
}

// CreateDevinTask creates a Devin task for further game development and returns the session ID
func (g *GitRepo) CreateDevinTask(gameSpecID, gameTitle string) (string, error) {
	repoURL := strings.TrimSuffix(os.Getenv("GIT_REPO_URL"), ".git")
	if repoURL == "" {
		return "", fmt.Errorf("GIT_REPO_URL environment variable not set")
	}

	taskDescription := fmt.Sprintf(`Please work on the game project in folder %s.

This folder contains a README.md file that describes the complete game specification and requirements.

Your tasks:
1. Navigate to the %s folder in the repository
2. Read the README.md file to understand the game specification
3. Implement the complete game based on the specification in the README
4. Create all necessary HTML, CSS, and JavaScript files for the game
5. Ensure the game is fully functional and meets all requirements specified in the README
6. Test the game thoroughly to ensure it works correctly
7. Create a new branch for your implementation (e.g., implement/game-%s or develop/game-%s)
8. Commit your implementation to the new branch with descriptive commit messages
9. Create a pull request to merge your implementation into the main branch
10. Include screenshots or a demo video in the PR description

Repository: %s
Game Title: %s
Game Spec ID: %s

IMPORTANT: Do NOT commit directly to the main branch. Always create a feature branch and submit a pull request for review. The README.md contains the complete specification - implement the game from scratch based on these requirements.`, gameSpecID, gameSpecID, gameSpecID, gameSpecID, repoURL, gameTitle, gameSpecID)

	// Create payload for Devin API sessions endpoint
	payload := map[string]interface{}{
		"prompt":     taskDescription,
		"idempotent": true,
	}

	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Get Devin API URL from environment or use default
	apiURL := os.Getenv("DEVIN_API_URL")
	if apiURL == "" {
		apiURL = "https://api.devin.ai/v1/sessions"
	}

	// Get API key
	apiKey := os.Getenv("DEVIN_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("DEVIN_API_KEY environment variable is required")
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for better error reporting
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Log the response for debugging
	log.Printf("Devin API Response Status: %d", resp.StatusCode)
	log.Printf("Devin API Response Body: %s", string(respBody))

	// Check response status
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("Devin API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response to get session info
	var sessionResponse map[string]interface{}
	if err := json.Unmarshal(respBody, &sessionResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract session ID from response
	_, ok := sessionResponse["session_id"]
	if !ok {
		return "", fmt.Errorf("session_id not found in response")
	}

	sessionIDStr, ok := sessionResponse["session_id"].(string)
	if !ok {
		return "", fmt.Errorf("session_id is not a string")
	}
	sessionIDStr = strings.TrimPrefix(sessionIDStr, "devin-")

	// Log session creation success
	log.Printf("Successfully created Devin session: %s", sessionIDStr)
	if sessionURL, ok := sessionResponse["url"]; ok {
		log.Printf("Session URL: %s", sessionURL)
	}
	log.Printf("Game will be created in folder: %s", gameSpecID)

	return sessionIDStr, nil
}
