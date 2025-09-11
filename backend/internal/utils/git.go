package utils

import (
	"encoding/json"
	"fmt"
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
		AutoPush: os.Getenv("GIT_AUTO_PUSH") == "true",
	}
}

func (g *GitRepo) IsConfigured() bool {
	return g.RepoPath != "" && g.RepoURL != "" && g.Token != ""
}

// safeSubstring safely extracts substring without panic
func safeSubstring(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length]
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

	// Push to remote if auto-push is enabled
	if g.AutoPush {
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
	}

	return nil
}

// RemoveGameFolders removes the folder with the exact gameID
func (g *GitRepo) RemoveGameFolders(gameID, gameTitle string) error {
	if !g.IsConfigured() {
		return fmt.Errorf("git repository not configured")
	}

	// Pull latest changes before making deletions
	if err := g.pullFromRemote(); err != nil {
		return fmt.Errorf("failed to pull latest changes: %v", err)
	}

	// Check if the folder exists
	folderPath := filepath.Join(g.RepoPath, gameID)
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		// Folder doesn't exist, nothing to remove
		return nil
	}

	// Remove the folder
	if err := os.RemoveAll(folderPath); err != nil {
		return fmt.Errorf("failed to remove folder %s: %v", gameID, err)
	}

	// Stage the deletion
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stage deletion: %v", err)
	}

	// Check if there are any changes to commit
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err == nil {
		// No changes to commit
		return nil
	}

	// Commit the deletion
	commitTemplate := os.Getenv("GIT_COMMIT_MESSAGE_TEMPLATE")
	if commitTemplate == "" {
		commitTemplate = "Removed game folder for deleted spec: %s (ID: %s)"
	}
	commitMessage := fmt.Sprintf(commitTemplate, gameTitle, gameID)

	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = g.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit folder deletion: %v", err)
	}

	// Push to remote if auto-push is enabled
	if g.AutoPush {
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
	}

	return nil
}
