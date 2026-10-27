package types

import "encoding/json"

// OpenAI Responses API types.
// Reference: https://platform.openai.com/docs/api-reference/responses

// ResponsesRequest represents a request to the OpenAI Responses API.
type ResponsesRequest struct {
	Model             string              `json:"model"`
	Input             []ResponsesInput    `json:"input"`
	Stream            bool                `json:"stream,omitempty"`
	Tools             []ResponsesTool     `json:"tools,omitempty"`
	Reasoning         *ResponsesReasoning `json:"reasoning,omitempty"`
	MaxOutputTokens   int                 `json:"max_output_tokens,omitempty"`
	Temperature       *float64            `json:"temperature,omitempty"`
	TopP              *float64            `json:"top_p,omitempty"`
	ToolChoice        json.RawMessage     `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool               `json:"parallel_tool_calls,omitempty"`
	Text              json.RawMessage     `json:"text,omitempty"`
	PromptCacheKey    string              `json:"prompt_cache_key,omitempty"`
}

// ResponsesInput represents a single input item in the Responses request.
type ResponsesInput struct {
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
}

// ResponsesTool represents a tool definition for the Responses API.
type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ResponsesReasoning controls reasoning effort.
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// ResponsesResponse represents a response from the OpenAI Responses API.
type ResponsesResponse struct {
	Error             json.RawMessage             `json:"error,omitempty"`
	ID                string                      `json:"id"`
	Object            string                      `json:"object"`
	Created           int64                       `json:"created"`
	Model             string                      `json:"model"`
	Output            []ResponsesOutput           `json:"output"`
	Usage             *ResponsesUsage             `json:"usage,omitempty"`
	Status            string                      `json:"status,omitempty"`
	IncompleteDetails *ResponsesIncompleteDetails `json:"incomplete_details,omitempty"`
}

type ResponsesIncompleteDetails struct {
	Reason string `json:"reason"`
}

// ResponsesOutput represents a single output item.
type ResponsesOutput struct {
	Type             string             `json:"type"`
	Status           string             `json:"status,omitempty"`
	ID               string             `json:"id,omitempty"`
	Role             string             `json:"role,omitempty"`
	Content          []ResponsesContent `json:"content,omitempty"`
	CallID           string             `json:"call_id,omitempty"`
	Name             string             `json:"name,omitempty"`
	Arguments        string             `json:"arguments,omitempty"`
	Summary          []ResponsesContent `json:"summary,omitempty"`
	EncryptedContent string             `json:"encrypted_content,omitempty"`
}

// ResponsesContent represents content within an output item.
type ResponsesContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

// ResponsesUsage represents token usage in a Responses response.
type ResponsesUsage struct {
	InputTokens         int                      `json:"input_tokens"`
	OutputTokens        int                      `json:"output_tokens"`
	InputTokensDetails  *PromptTokensDetails     `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *CompletionTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponsesChunk represents a streaming chunk from the Responses API.
type ResponsesChunk struct {
	Error        json.RawMessage    `json:"error,omitempty"`
	Type         string             `json:"type"`
	ID           string             `json:"id,omitempty"`
	Delta        string             `json:"delta,omitempty"`
	Text         string             `json:"text,omitempty"`
	SummaryIndex int                `json:"summary_index,omitempty"`
	ContentIndex int                `json:"content_index,omitempty"`
	Refusal      string             `json:"refusal,omitempty"`
	Part         *ResponsesContent  `json:"part,omitempty"`
	Output       []ResponsesOutput  `json:"output,omitempty"`
	Usage        *ResponsesUsage    `json:"usage,omitempty"`
	ItemID       string             `json:"item_id,omitempty"`
	Item         *ResponsesOutput   `json:"item,omitempty"`
	Response     *ResponsesResponse `json:"response,omitempty"`
}

// Google Gemini API types.
// Reference: https://ai.google.dev/api/generate-content

// GeminiRequest represents a request to the Gemini API.
type GeminiRequest struct {
	Contents          []GeminiContent         `json:"contents"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []GeminiTool            `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"toolConfig,omitempty"`
	Stream            bool                    `json:"stream,omitempty"`
}

// GeminiContent represents a single content item.
type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of a content item.
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *GeminiBlob             `json:"inlineData,omitempty"`
	FileData         *GeminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiBlob struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiFileData struct {
	MIMEType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type GeminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type GeminiFunctionResponse struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig GeminiFunctionCallingConfig `json:"functionCallingConfig"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GeminiGenerationConfig controls generation parameters.
type GeminiGenerationConfig struct {
	Temperature        *float64        `json:"temperature,omitempty"`
	TopP               *float64        `json:"topP,omitempty"`
	StopSequences      []string        `json:"stopSequences,omitempty"`
	ResponseMIMEType   string          `json:"responseMimeType,omitempty"`
	ResponseJSONSchema json.RawMessage `json:"responseJsonSchema,omitempty"`
	MaxOutputTokens    int             `json:"maxOutputTokens,omitempty"`
	// ThinkingConfig carries Gemini's own reasoning control. It was absent, so
	// a request that asked for any thinking level reached Gemini with nothing
	// in the body about reasoning and the model used its default.
	ThinkingConfig *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// GeminiThinkingConfig mirrors generationConfig.thinkingConfig. ThinkingBudget
// is a token cap; Gemini treats 0 as "do not think" and -1 as "decide per
// request", which is why it is a signed integer rather than a level.
type GeminiThinkingConfig struct {
	// ThinkingBudget is the token cap for models that take one (Gemini 2.5).
	ThinkingBudget *int `json:"thinkingBudget,omitempty"`
	// ThinkingLevel is the discrete rung for models that take one (Gemini 3+).
	// The two are mutually exclusive: Gemini answers 400 when both arrive.
	ThinkingLevel *string `json:"thinkingLevel,omitempty"`
	// IncludeThoughts asks Gemini to return the thought summaries. Without it
	// the model may think and return nothing about the thinking, which reads
	// downstream as "this model produced no reasoning" rather than "we did not
	// ask for it".
	IncludeThoughts *bool `json:"includeThoughts,omitempty"`
}

// GeminiTool represents a tool definition.
type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// GeminiFunctionDeclaration represents a function declaration.
type GeminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// GeminiResponse represents a response from the Gemini API.
type GeminiResponse struct {
	ModelVersion   string                `json:"modelVersion,omitempty"`
	Error          json.RawMessage       `json:"error,omitempty"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
	Candidates     []GeminiCandidate     `json:"candidates"`
	UsageMetadata  *GeminiUsage          `json:"usageMetadata,omitempty"`
}

// GeminiCandidate represents a response candidate.
type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type GeminiPromptFeedback struct {
	BlockReason string `json:"blockReason,omitempty"`
}

// GeminiUsage represents token usage in a Gemini response.
type GeminiUsage struct {
	PromptTokenCount        *int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    *int `json:"candidatesTokenCount,omitempty"`
	ThoughtsTokenCount      *int `json:"thoughtsTokenCount,omitempty"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount,omitempty"`
	ToolUsePromptTokenCount *int `json:"toolUsePromptTokenCount,omitempty"`
	TotalTokenCount         int  `json:"totalTokenCount"`
}

// GeminiStreamChunk represents a streaming chunk from the Gemini API.
type GeminiStreamChunk struct {
	ModelVersion   string                `json:"modelVersion,omitempty"`
	Error          json.RawMessage       `json:"error,omitempty"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
	Candidates     []GeminiCandidate     `json:"candidates"`
	UsageMetadata  *GeminiUsage          `json:"usageMetadata,omitempty"`
}
