package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"memex/internal/config"
	"memex/internal/store"
)

func TestCreateNoteSpaceEnvelope(t *testing.T) {
	const spaceRequired = `space required: top-level field alongside body, e.g. {"space":"ops/fixes","body":{...}}`
	const invalidSpace = "invalid space: 1-200 chars, no leading dot/underscore/-, no '..', no leading/trailing slash, not dm/<a>/<b>"
	noteBody := map[string]any{"topic": "lab/unique", "status": "answer", "answer": "ok"}

	s := &Server{
		Cfg:       config.Default(),
		Limit:     NewLimiter(),
		testAgent: &store.Agent{ID: "00000000-0000-7000-8000-000000000001", Status: "active"},
	}
	ts := httptest.NewServer(s.Handler("agent"))
	t.Cleanup(ts.Close)

	cases := []struct {
		name string
		body any
		want string
	}{
		{
			name: "space nested inside body",
			body: map[string]any{"body": map[string]any{"space": "ops/fixes", "topic": "lab/unique", "status": "answer", "answer": "ok"}},
			want: spaceRequired,
		},
		{
			name: "space missing entirely",
			body: map[string]any{"body": noteBody},
			want: spaceRequired,
		},
		{
			name: "invalid space leading slash",
			body: map[string]any{"space": "/ops", "body": noteBody},
			want: invalidSpace,
		},
		{
			name: "dm space rejected",
			body: map[string]any{"space": "dm/a/b", "body": noteBody},
			want: invalidSpace,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := postNote(t, ts.URL+"/v1/notes", tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d body %s", res.StatusCode, res.Body)
			}
			var got errBody
			if err := json.Unmarshal([]byte(res.Body), &got); err != nil {
				t.Fatalf("json %s: %v", res.Body, err)
			}
			if got.Error != tc.want {
				t.Fatalf("error %q want %q", got.Error, tc.want)
			}
		})
	}
}

type noteResponse struct {
	StatusCode int
	Body       string
}

func postNote(t *testing.T, url string, body any) noteResponse {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer mxt_test")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return noteResponse{StatusCode: res.StatusCode, Body: string(raw)}
}
