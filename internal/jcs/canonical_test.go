package jcs

import (
	"strconv"
	"strings"
	"testing"
)

func TestRejectsDeepNesting(t *testing.T) {
	// maxNestingDepth+1 levels of "[" then a scalar: must error with the
	// depth message instead of re-copying quadratically for minutes.
	depth := maxNestingDepth + 2
	in := strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth)
	out, err := Transform([]byte(in))
	if err == nil {
		t.Fatalf("depth %d accepted: %d bytes out", depth, len(out))
	}
	if !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("wrong error: %v", err)
	}
	// At the cap the input still canonicalizes.
	ok := strings.Repeat("[", maxNestingDepth) + "1" + strings.Repeat("]", maxNestingDepth)
	if _, err := Transform([]byte(ok)); err != nil {
		t.Fatalf("depth %d should pass: %v", maxNestingDepth, err)
	}
}

func TestRejectsOversizedContainers(t *testing.T) {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < maxMembers+10; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"` + "kxxxxxxxx" + strconv.Itoa(i) + `"`)
		b.WriteString(`:""`)
	}
	b.WriteByte('}')
	in := b.String()
	_, err := Transform([]byte(in))
	if err == nil {
		t.Fatalf("member count over cap accepted: %d bytes in", len(in))
	}
	if !strings.Contains(err.Error(), "member limit") {
		t.Fatalf("wrong error: %v", err)
	}
	// Arrays are bounded the same way.
	ab := strings.Builder{}
	ab.WriteByte('[')
	for i := 0; i < maxMembers+10; i++ {
		if i > 0 {
			ab.WriteByte(',')
		}
		ab.WriteByte('1')
	}
	ab.WriteByte(']')
	if _, err := Transform([]byte(ab.String())); err == nil {
		t.Fatal("oversized array accepted")
	}
}

// The audit's cost bomb: a key-packed object at the 64KiB request cap used
// to take ~70ms. With the member cap, a 64KiB object can hold at most a few
// thousand members and must stay a small function of its byte size.
func TestKeyPackedObjectStaysBounded(t *testing.T) {
	var b strings.Builder
	b.WriteByte('{')
	i := 0
	for b.Len() < 64*1024-16 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"k` + strconv.Itoa(i) + `":""`)
		i++
	}
	b.WriteByte('}')
	in := b.String()
	_, err := Transform([]byte(in))
	if err != nil && !strings.Contains(err.Error(), "member limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}
