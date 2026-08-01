package faker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var subjects = []string{
	"Artificial intelligence", "Machine learning", "Deep neural networks",
	"Reinforcement learning", "Natural language processing", "Computer vision",
	"Generative models", "Transformer architectures", "Attention mechanisms",
	"Transfer learning", "The system architecture", "The training pipeline",
	"Data preprocessing", "Model evaluation", "Hyperparameter tuning",
}

var verbs = []string{
	"transforms", "processes", "analyzes", "optimizes", "generates",
	"predicts", "classifies", "extracts", "maps", "encodes",
	"represents a fundamental paradigm shift in", "enables efficient",
	"provides robust", "ensures reliable", "facilitates scalable",
}

var objects = []string{
	"raw input data into meaningful representations",
	"complex patterns across diverse datasets",
	"high-dimensional feature spaces with precision",
	"contextual embeddings from sequential inputs",
	"probabilistic distributions over latent variables",
	"attention weights for contextual understanding",
	"semantic representations of unstructured text",
	"hierarchical features from multi-modal inputs",
	"gradients through differentiable computation graphs",
	"token-level predictions with state-of-the-art accuracy",
}

var conclusions = []string{
	"leading to significant improvements in benchmark performance.",
	"resulting in more robust and generalizable models.",
	"enabling faster convergence during the training process.",
	"reducing computational overhead while maintaining quality.",
	"achieving new state-of-the-art results across multiple tasks.",
	"demonstrating the power of modern deep learning techniques.",
	"paving the way for the next generation of AI systems.",
	"unlocking new capabilities previously thought impossible.",
	"bridging the gap between research and production deployment.",
	"validating the effectiveness of the proposed methodology.",
}

var streamingWords = []string{
	"The", "key", "insight", "is", "that", "modern", "AI", "systems",
	"leverage", "massive", "datasets", "to", "learn", "complex", "representations",
	"through", "iterative", "optimization", "of", "differentiable", "objectives",
	"using", "stochastic", "gradient", "descent", "and", "backpropagation",
	"enabling", "unprecedented", "performance", "across", "diverse", "tasks",
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      msgResp `json:"message,omitempty"`
	Delta        *delta  `json:"delta,omitempty"`
	FinishReason string  `json:"finish_reason"`
}

type msgResp struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type streamChunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
}

func GenerateResponse(body []byte) ([]byte, error) {
	req, err := parseRequest(body)
	if err != nil {
		return nil, fmt.Errorf("parsing request: %w", err)
	}

	seed := hashBody(body)
	rng := rand.New(rand.NewSource(seed))

	promptTokens := countTokens(req, rng)
	responseText := generateParagraph(rng, seed)
	completionTokens := approxTokens(responseText)

	resp := chatResponse{
		ID:      fmt.Sprintf("chatcmpl-%x", seed),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   pickModel(req, rng),
		Choices: []choice{{
			Index: 0,
			Message: msgResp{
				Role:    "assistant",
				Content: responseText,
			},
			FinishReason: "stop",
		}},
		Usage: usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	time.Sleep(delay(rng))
	return json.Marshal(resp)
}

func StreamResponse(body []byte, out chan<- []byte, done chan<- struct{}) error {
	req, err := parseRequest(body)
	if err != nil {
		return fmt.Errorf("parsing request: %w", err)
	}

	seed := hashBody(body)
	rng := rand.New(rand.NewSource(seed))
	chunkID := fmt.Sprintf("chatcmpl-%x", seed)
	now := time.Now().Unix()
	model := pickModel(req, rng)
	words := generateWordList(rng, 25+rng.Intn(35))

	go func() {
		defer func() { done <- struct{}{} }()

		first := streamChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: now,
			Model:   model,
			Choices: []choice{{
				Index: 0,
				Delta: &delta{
					Role: "assistant",
				},
				FinishReason: "",
			}},
		}
		chunk, _ := json.Marshal(first)
		out <- chunk
		time.Sleep(delay(rng))

		for i, word := range words {
			text := word
			if i < len(words)-1 {
				text += " "
			}
			finish := ""
			if i == len(words)-1 {
				finish = "stop"
			}
			chunk := streamChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: now,
				Model:   model,
				Choices: []choice{{
					Index: 0,
					Delta: &delta{
						Content: text,
					},
					FinishReason: finish,
				}},
			}
			data, _ := json.Marshal(chunk)
			out <- data
			time.Sleep(delay(rng))
		}
	}()
	return nil
}

func parseRequest(body []byte) (chatRequest, error) {
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	return req, nil
}

func hashBody(body []byte) int64 {
	h := sha256.Sum256(body)
	var seed int64
	for i := 0; i < 8; i++ {
		seed = (seed << 8) | int64(h[i])
	}
	if seed < 0 {
		seed = -seed
	}
	return seed
}

func generateParagraph(rng *rand.Rand, seed int64) string {
	numSentences := 3 + rng.Intn(4)
	var sentences []string
	for i := 0; i < numSentences; i++ {
		sub := subjects[rng.Intn(len(subjects))]
		vrb := verbs[rng.Intn(len(verbs))]
		obj := objects[rng.Intn(len(objects))]
		conc := conclusions[rng.Intn(len(conclusions))]
		sentences = append(sentences, fmt.Sprintf("%s %s %s, %s", sub, vrb, obj, conc))
	}
	return strings.Join(sentences, " ")
}

func generateWordList(rng *rand.Rand, count int) []string {
	words := make([]string, count)
	for i := 0; i < count; i++ {
		words[i] = streamingWords[rng.Intn(len(streamingWords))]
	}
	return words
}

func countTokens(req chatRequest, rng *rand.Rand) int {
	total := 0
	for _, msg := range req.Messages {
		total += len(strings.Fields(msg.Content))
	}
	return max(total, 1)
}

func approxTokens(text string) int {
	return len(strings.Fields(text))
}

func pickModel(req chatRequest, rng *rand.Rand) string {
	if req.Model != "" {
		return req.Model
	}
	models := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	return models[rng.Intn(len(models))]
}

func delay(rng *rand.Rand) time.Duration {
	min := 200
	max := 600
	return time.Duration(min+rng.Intn(max-min)) * time.Millisecond
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
