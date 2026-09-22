package api

import (
	"testing"
	"time"
)

func TestLimiterIsolatesKeys(t *testing.T) {
	now := time.Now()
	l := NewLimiter()
	l.now = func() time.Time { return now }
	if !l.Allow("a", 1) || l.Allow("a", 1) {
		t.Fatal("a should exhaust at 1/min")
	}
	if !l.Allow("b", 1) {
		t.Fatal("b is a different bucket")
	}
	now = now.Add(time.Minute)
	if !l.Allow("a", 1) {
		t.Fatal("a should refill after a minute")
	}
}
