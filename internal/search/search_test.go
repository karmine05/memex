package search

import "testing"

func TestEmbedInputUsesTopicAndAnswer(t *testing.T) {
	got := EmbedInput(`{"topic":"k8s/oom","answer":"set swap.max","status":"answer"}`)
	if got != "topic: k8s/oom\nset swap.max" {
		t.Fatal(got)
	}
	if EmbedInput("plain") != "plain" {
		t.Fatal("plain")
	}
}

func TestVectorLiteral(t *testing.T) {
	got, err := VectorLiteral([]float32{1, -0.5})
	if err != nil {
		t.Fatal(err)
	}
	if got != "[1,-0.5]" {
		t.Fatal(got)
	}
}
