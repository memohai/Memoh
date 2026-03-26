package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// SkillClient handles HTTP API calls to the Memoh server
type SkillClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewSkillClient creates a new skill client
func NewSkillClient(baseURL string) *SkillClient {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &SkillClient{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetToken sets the JWT token for authentication
func (c *SkillClient) SetToken(token string) {
	c.Token = token
}

// Login authenticates and gets a JWT token
func (c *SkillClient) Login(username, password string) (string, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	resp, err := c.HTTPClient.Post(
		c.BaseURL+"/api/auth/login",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode login response: %w", err)
	}

	c.Token = result.AccessToken
	return result.AccessToken, nil
}

// doRequest makes an authenticated HTTP request
func (c *SkillClient) doRequest(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.HTTPClient.Do(req)
}

// ListSkills lists all skills for a bot
func (c *SkillClient) ListSkills(botID string) ([]SkillItem, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/bots/%s/skills/v2", botID), nil, "")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list skills (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Skills []SkillItem `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Skills, nil
}

// GetSkill gets a specific skill
func (c *SkillClient) GetSkill(botID, skillName string) (*SkillItem, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/bots/%s/skills/v2/%s", botID, skillName), nil, "")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get skill (%d): %s", resp.StatusCode, string(body))
	}

	var skill SkillItem
	if err := json.NewDecoder(resp.Body).Decode(&skill); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &skill, nil
}

// InstallSkillFromFile installs a skill from a .skill file
func (c *SkillClient) InstallSkillFromFile(botID, filePath string) (*InstallResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}
	writer.Close()

	resp, err := c.doRequest(
		"POST",
		fmt.Sprintf("/api/v1/bots/%s/skills/v2/install", botID),
		&buf,
		writer.FormDataContentType(),
	)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to install skill (%d): %s", resp.StatusCode, string(body))
	}

	var result InstallResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UninstallSkill removes a skill
func (c *SkillClient) UninstallSkill(botID, skillName string) error {
	resp, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/bots/%s/skills/v2/%s", botID, skillName), nil, "")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill not found: %s", skillName)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to uninstall skill (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// EnableSkill enables a skill
func (c *SkillClient) EnableSkill(botID, skillName string, enabled bool) error {
	reqBody, _ := json.Marshal(map[string]bool{
		"enabled": enabled,
	})

	resp, err := c.doRequest(
		"PATCH",
		fmt.Sprintf("/api/v1/bots/%s/skills/v2/%s/state", botID, skillName),
		bytes.NewBuffer(reqBody),
		"application/json",
	)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill not found: %s", skillName)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update skill state (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ExportSkill exports a skill to a .skill file
func (c *SkillClient) ExportSkill(botID, skillName, outputPath string) error {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/bots/%s/skills/v2/%s/export", botID, skillName), nil, "")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill not found: %s", skillName)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to export skill (%d): %s", resp.StatusCode, string(body))
	}

	// Write to file
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// SkillItem represents a skill in the list
type SkillItem struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	Author        string   `json:"author"`
	License       string   `json:"license"`
	AllowedTools  []string `json:"allowed_tools"`
	Compatibility string   `json:"compatibility"`
	Category      string   `json:"category"`
	Enabled       bool     `json:"enabled"`
	AutoLoad      bool     `json:"auto_load"`
	CategoryDir   string   `json:"category_dir"`
}

// InstallSkillFromTemplate installs a skill from built-in templates
func (c *SkillClient) InstallSkillFromTemplate(botID, skillName string) (*InstallResult, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"skill_name": skillName,
		"source":     "template",
	})

	resp, err := c.doRequest(
		"POST",
		fmt.Sprintf("/api/v1/bots/%s/skills/v2/install/template", botID),
		bytes.NewBuffer(reqBody),
		"application/json",
	)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to install skill (%d): %s", resp.StatusCode, string(body))
	}

	var result InstallResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// InstallSkillFromMarket installs a skill from skill.sh marketplace
func (c *SkillClient) InstallSkillFromMarket(botID, fullSkillName string) (*InstallResult, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"skill_name": fullSkillName,
	})

	resp, err := c.doRequest(
		"POST",
		fmt.Sprintf("/api/v1/bots/%s/skills/v2/install/market", botID),
		bytes.NewBuffer(reqBody),
		"application/json",
	)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to install skill (%d): %s", resp.StatusCode, string(body))
	}

	var result InstallResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

type InstallResult struct {
	Success   bool   `json:"success"`
	SkillName string `json:"skill_name"`
	Message   string `json:"message"`
}

// runSkillCmd is the entry point for the skill command
func runSkillCmd(args []string) error {
	var (
		serverURL string
		botID     string
		username  string
		password  string
		token     string
	)

	// Try to get from environment
	if serverURL == "" {
		serverURL = os.Getenv("MEMOH_SERVER")
	}
	if botID == "" {
		botID = os.Getenv("MEMOH_BOT_ID")
	}
	if token == "" {
		token = os.Getenv("MEMOH_TOKEN")
	}
	if username == "" {
		username = os.Getenv("MEMOH_USERNAME")
	}
	if password == "" {
		password = os.Getenv("MEMOH_PASSWORD")
	}

	rootCmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage bot skills",
		Long:  `Install, list, and manage skills for a bot.`,
	}

	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", serverURL, "Memoh server URL (env: MEMOH_SERVER)")
	rootCmd.PersistentFlags().StringVarP(&botID, "bot", "b", botID, "Bot ID (env: MEMOH_BOT_ID)")
	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", username, "Username (env: MEMOH_USERNAME)")
	rootCmd.PersistentFlags().StringVarP(&password, "password", "p", password, "Password (env: MEMOH_PASSWORD)")
	rootCmd.PersistentFlags().StringVarP(&token, "token", "t", token, "JWT token (env: MEMOH_TOKEN)")

	// Create client factory
	getClient := func() (*SkillClient, error) {
		if serverURL == "" {
			serverURL = "http://localhost:8080"
		}

		client := NewSkillClient(serverURL)

		if token != "" {
			client.SetToken(token)
		} else if username != "" && password != "" {
			if _, err := client.Login(username, password); err != nil {
				return nil, fmt.Errorf("authentication failed: %w", err)
			}
		} else {
			return nil, fmt.Errorf("authentication required: provide --token or --username/--password")
		}

		return client, nil
	}

	// Require bot ID check
	checkBotID := func() error {
		if botID == "" {
			return fmt.Errorf("bot ID is required: use --bot or set MEMOH_BOT_ID")
		}
		return nil
	}

	// list command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all skills for a bot",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			skills, err := client.ListSkills(botID)
			if err != nil {
				return err
			}

			if len(skills) == 0 {
				fmt.Println("No skills installed")
				return nil
			}

			fmt.Printf("%-20s %-10s %-10s %-8s %s\n", "NAME", "VERSION", "CATEGORY", "ENABLED", "DESCRIPTION")
			fmt.Println(strings.Repeat("-", 80))
			for _, s := range skills {
				enabled := "no"
				if s.Enabled {
					enabled = "yes"
				}
				desc := s.Description
				if len(desc) > 40 {
					desc = desc[:37] + "..."
				}
				fmt.Printf("%-20s %-10s %-10s %-8s %s\n", s.Name, s.Version, s.CategoryDir, enabled, desc)
			}
			fmt.Printf("\nTotal: %d skills\n", len(skills))
			return nil
		},
	}

	// get command
	getCmd := &cobra.Command{
		Use:   "get <skill-name>",
		Short: "Get details of a specific skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			skill, err := client.GetSkill(botID, args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:        %s\n", skill.Name)
			fmt.Printf("Version:     %s\n", skill.Version)
			fmt.Printf("Author:      %s\n", skill.Author)
			fmt.Printf("License:     %s\n", skill.License)
			fmt.Printf("Category:    %s\n", skill.CategoryDir)
			fmt.Printf("Enabled:     %v\n", skill.Enabled)
			fmt.Printf("Auto-load:   %v\n", skill.AutoLoad)
			if len(skill.AllowedTools) > 0 {
				fmt.Printf("Allowed tools: %s\n", strings.Join(skill.AllowedTools, ", "))
			}
			fmt.Printf("Description: %s\n", skill.Description)
			return nil
		},
	}

	// install command - supports both file path and skill name
	installCmd := &cobra.Command{
		Use:   "install <skill-name-or-path>",
		Short: "Install a skill from marketplace or .skill file",
		Long: `Install a skill by name from skill.sh marketplace or from a .skill archive file.

Examples:
  memoh skill install chart-visualization          # Install from built-in templates
  memoh skill install vercel-labs/agent-skills@vercel-react-best-practices  # Install from skill.sh
  memoh skill install ./my-skill.skill             # Install from .skill file`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			input := args[0]
			var result *InstallResult

			// Determine installation source based on input format
			if strings.HasSuffix(input, ".skill") {
				// Install from .skill file
				result, err = client.InstallSkillFromFile(botID, input)
			} else if strings.Contains(input, "@") {
				// Install from skill.sh marketplace (owner/repo@skill-name format)
				result, err = client.InstallSkillFromMarket(botID, input)
			} else {
				// Install from built-in templates by name
				result, err = client.InstallSkillFromTemplate(botID, input)
			}

			if err != nil {
				return err
			}

			if result.Success {
				fmt.Printf("✓ Skill '%s' installed successfully\n", result.SkillName)
			} else {
				fmt.Printf("✗ Failed to install skill: %s\n", result.Message)
			}
			return nil
		},
	}

	// uninstall command
	uninstallCmd := &cobra.Command{
		Use:     "uninstall <skill-name>",
		Aliases: []string{"remove", "rm"},
		Short:   "Uninstall a skill",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			if err := client.UninstallSkill(botID, args[0]); err != nil {
				return err
			}

			fmt.Printf("✓ Skill '%s' uninstalled\n", args[0])
			return nil
		},
	}

	// enable command
	enableCmd := &cobra.Command{
		Use:   "enable <skill-name>",
		Short: "Enable a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			if err := client.EnableSkill(botID, args[0], true); err != nil {
				return err
			}

			fmt.Printf("✓ Skill '%s' enabled\n", args[0])
			return nil
		},
	}

	// disable command
	disableCmd := &cobra.Command{
		Use:   "disable <skill-name>",
		Short: "Disable a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			if err := client.EnableSkill(botID, args[0], false); err != nil {
				return err
			}

			fmt.Printf("✓ Skill '%s' disabled\n", args[0])
			return nil
		},
	}

	// export command
	exportCmd := &cobra.Command{
		Use:   "export <skill-name> [output-path]",
		Short: "Export a skill to a .skill file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := checkBotID(); err != nil {
				return err
			}
			client, err := getClient()
			if err != nil {
				return err
			}

			skillName := args[0]
			outputPath := skillName + ".skill"
			if len(args) > 1 {
				outputPath = args[1]
			}

			if err := client.ExportSkill(botID, skillName, outputPath); err != nil {
				return err
			}

			fmt.Printf("✓ Skill '%s' exported to %s\n", skillName, outputPath)
			return nil
		},
	}

	rootCmd.AddCommand(listCmd, getCmd, installCmd, uninstallCmd, enableCmd, disableCmd, exportCmd)

	return rootCmd.Execute()
}
