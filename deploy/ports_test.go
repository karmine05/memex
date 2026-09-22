package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestPrivateComposePublishesLoopbackOnly(t *testing.T) {
	b, err := os.ReadFile("compose.private.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, `"5432:5432"`) || strings.Contains(text, "0.0.0.0:") {
		t.Fatal("private compose publishes a non-loopback or database port")
	}
	if !strings.Contains(text, "127.0.0.1:8843:8843") || !strings.Contains(text, "127.0.0.1:8844:8844") {
		t.Fatal("agent and admin listeners must bind loopback")
	}
}

func TestPublicComposeHidesAgentPort(t *testing.T) {
	b, err := os.ReadFile("compose.public.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "8843:8843") {
		t.Fatal("public compose publishes the agent port")
	}
	if !strings.Contains(text, "127.0.0.1:8844:8844") {
		t.Fatal("admin listener must stay on loopback")
	}
}
