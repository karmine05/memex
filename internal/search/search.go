package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Healthy(ctx context.Context) error
}

type None struct{}

func (None) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embedder disabled")
}

func (None) Healthy(context.Context) error { return errors.New("embedder disabled") }

type Ollama struct {
	URL    string
	Model  string
	Dim    int
	Client *http.Client
}

type OpenAI struct {
	URL    string
	Model  string
	APIKey string
	Dim    int
	Client *http.Client
}

func client() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	payload, _ := json.Marshal(map[string]any{"model": o.Model, "input": texts})
	var resp struct {
		Embeddings [][]float32 `json:"embeddings"`
		Embedding  []float32   `json:"embedding"`
	}
	if err := postJSON(ctx, o.http(), strings.TrimRight(o.URL, "/")+"/api/embed", "", payload, &resp); err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 && len(resp.Embedding) > 0 {
		resp.Embeddings = [][]float32{resp.Embedding}
	}
	return checkDim(o.Dim, texts, resp.Embeddings)
}

func (o *Ollama) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.URL, "/")+"/api/tags", nil)
	if err != nil {
		return err
	}
	res, err := o.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("ollama status %d", res.StatusCode)
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return err
	}
	want := strings.Split(o.Model, ":")[0]
	for _, m := range out.Models {
		if strings.Split(m.Name, ":")[0] == want {
			return nil
		}
	}
	return fmt.Errorf("model %s not loaded", o.Model)
}

func (o *Ollama) http() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return client()
}

func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	payload, _ := json.Marshal(map[string]any{"model": o.Model, "input": texts})
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	base := strings.TrimRight(o.URL, "/")
	if err := postJSON(ctx, o.http(), base+"/v1/embeddings", o.APIKey, payload, &resp); err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, errors.New("embedding index out of range")
		}
		out[d.Index] = d.Embedding
	}
	return checkDim(o.Dim, texts, out)
}

func (o *OpenAI) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.URL, "/")+"/v1/models", nil)
	if err != nil {
		return err
	}
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	res, err := o.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("openai status %d", res.StatusCode)
	}
	return nil
}

func (o *OpenAI) http() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return client()
}

func postJSON(ctx context.Context, c *http.Client, url, key string, payload []byte, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("embed status %d", res.StatusCode)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("embed decode: %w", err)
	}
	return nil
}

func checkDim(dim int, texts []string, vecs [][]float32) ([][]float32, error) {
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("embed count %d want %d", len(vecs), len(texts))
	}
	for i, v := range vecs {
		if len(v) != dim {
			return nil, fmt.Errorf("embed dim %d want %d", len(v), dim)
		}
		if v == nil {
			return nil, fmt.Errorf("embed %d empty", i)
		}
	}
	return vecs, nil
}

// VectorLiteral renders a pgvector text literal. Values come from the embedder, not the client.
func VectorLiteral(v []float32) (string, error) {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return "", errors.New("non-finite embedding")
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String(), nil
}

// EmbedInput prefixes the topic and prefers the answer field when the body is a note object.
func EmbedInput(body string) string {
	var doc struct {
		Topic  string          `json:"topic"`
		Answer json.RawMessage `json:"answer"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil || doc.Topic == "" {
		return body
	}
	answer := strings.TrimSpace(string(doc.Answer))
	if len(doc.Answer) == 0 || answer == "null" {
		return "topic: " + doc.Topic + "\n" + body
	}
	var s string
	if json.Unmarshal(doc.Answer, &s) == nil {
		return "topic: " + doc.Topic + "\n" + s
	}
	return "topic: " + doc.Topic + "\n" + answer
}
