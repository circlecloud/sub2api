// Package openai provides helpers and types for OpenAI API integration.
package openai

import _ "embed"

// Model represents an OpenAI model
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// DefaultModels OpenAI 完整模型目录（主要给 API Key 账号的默认候选集使用）。
var DefaultModels = []Model{
	{ID: "gpt-5.4", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4"},
	{ID: "gpt-5.4-mini", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4 Mini"},
	{ID: "gpt-5.4-nano", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4 Nano"},
	{ID: "gpt-5.3-codex", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex"},
	{ID: "gpt-5.3-codex-spark", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex Spark"},
	{ID: "gpt-5.2", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.2"},
	{ID: "gpt-5.2-codex", Object: "model", Created: 1733011200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.2 Codex"},
	{ID: "gpt-5.1-codex-max", Object: "model", Created: 1730419200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.1 Codex Max"},
	{ID: "gpt-5.1-codex", Object: "model", Created: 1730419200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.1 Codex"},
	{ID: "gpt-5.1", Object: "model", Created: 1731456000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.1"},
	{ID: "gpt-5.1-codex-mini", Object: "model", Created: 1730419200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.1 Codex Mini"},
	{ID: "gpt-5", Object: "model", Created: 1722988800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5"},
}

// DefaultOAuthModels OpenAI OAuth 默认模型列表。
var DefaultOAuthModels = []Model{
	{ID: "gpt-5.4", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4"},
	{ID: "gpt-5.4-mini", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4 Mini"},
	{ID: "gpt-5.3-codex", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex"},
	{ID: "gpt-5.2", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.2"},
}

func modelIDs(models []Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

// DefaultModelIDs returns the default model ID list
func DefaultModelIDs() []string {
	return modelIDs(DefaultModels)
}

// DefaultOAuthModelIDs returns the curated OpenAI OAuth model ID list.
func DefaultOAuthModelIDs() []string {
	return modelIDs(DefaultOAuthModels)
}

// DefaultTestModel default model for testing OpenAI accounts
const DefaultTestModel = "gpt-5.4-mini"

// DefaultInstructions default instructions for non-Codex CLI requests
// Content loaded from instructions.txt at compile time
//
//go:embed instructions.txt
var DefaultInstructions string
