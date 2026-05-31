package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/UnitVectorY-Labs/invdex/internal/config"
	"github.com/UnitVectorY-Labs/invdex/internal/models"
)

// Client provides LLM capabilities for inventory management.
type Client struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

// New creates a new LLM client.
func New(cfg *config.Config) *Client {
	return &Client{
		endpoint:   cfg.LLMEndpoint,
		apiKey:     cfg.LLMAPIKey,
		model:      cfg.LLMModel,
		httpClient: &http.Client{},
	}
}

// ChatMessage represents a message in the chat completion API.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest represents a chat completion request.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// chatResponse represents a chat completion response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// SuggestFromImage analyzes an uploaded image and suggests item details.
func (c *Client) SuggestFromImage(ctx context.Context, imageDescription string) (*models.LLMSuggestion, error) {
	prompt := fmt.Sprintf(`You are an inventory management assistant for collectables. Based on the following description of an uploaded image, suggest appropriate inventory details.

Image description: %s

Respond with a JSON object containing:
- "title": A concise, descriptive title for this collectable item
- "description": A detailed description of the item including condition, notable features, and any relevant details for a collector
- "tags": An array of relevant tags/categories for this item (e.g., "trading cards", "vintage", "sports", "pokemon", etc.)

Respond ONLY with the JSON object, no other text.`, imageDescription)

	return c.getSuggestion(ctx, prompt)
}

// SuggestFromTitle suggests item details based on a title.
func (c *Client) SuggestFromTitle(ctx context.Context, title string) (*models.LLMSuggestion, error) {
	prompt := fmt.Sprintf(`You are an inventory management assistant for collectables. Based on the following item title, suggest appropriate inventory details.

Item title: %s

Respond with a JSON object containing:
- "title": The refined/corrected title for this collectable item
- "description": A detailed description of the item including what it likely is, typical condition notes, and any relevant details for a collector
- "tags": An array of relevant tags/categories for this item (e.g., "trading cards", "vintage", "sports", "pokemon", "coins", "stamps", "figurines", etc.)

Respond ONLY with the JSON object, no other text.`, title)

	return c.getSuggestion(ctx, prompt)
}

// Chat provides a conversational interface for inventory management assistance.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	systemMsg := ChatMessage{
		Role: "system",
		Content: `You are an inventory management assistant for collectables. You help users:
- Identify and categorize collectable items
- Suggest appropriate tags and descriptions
- Provide information about items (value estimates, rarity, condition grading)
- Help organize their collection
- Answer questions about collectables

Be helpful, concise, and knowledgeable about various types of collectables including trading cards, coins, stamps, figurines, vintage items, sports memorabilia, and more.`,
	}

	allMessages := append([]ChatMessage{systemMsg}, messages...)

	reqBody := chatRequest{
		Model:    c.model,
		Messages: allMessages,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (c *Client) getSuggestion(ctx context.Context, prompt string) (*models.LLMSuggestion, error) {
	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := c.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	// Clean up the response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var suggestion models.LLMSuggestion
	if err := json.Unmarshal([]byte(response), &suggestion); err != nil {
		return nil, fmt.Errorf("failed to parse LLM suggestion: %w", err)
	}

	return &suggestion, nil
}
