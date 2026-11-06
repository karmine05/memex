package feed

import "testing"

func TestPublishReachesSubscriber(t *testing.T) {
	h := New()
	ch, _, cancel := h.Subscribe("space:ops/fixes", "agent-a")
	defer cancel()
	h.Publish(Event{ID: 7, Space: "ops/fixes", NoteID: "n", BodyHash: "h"})
	ev := <-ch
	if ev.ID != 7 || ev.NoteID != "n" {
		t.Fatal(ev)
	}
}

func TestDMReachesBothInboxes(t *testing.T) {
	h := New()
	a, _, ca := h.Subscribe("inbox:aaa", "aaa")
	b, _, cb := h.Subscribe("inbox:bbb", "bbb")
	defer ca()
	defer cb()
	h.Publish(Event{ID: 1, Space: "dm/aaa/bbb"})
	if (<-a).ID != 1 || (<-b).ID != 1 {
		t.Fatal("inbox")
	}
}

func TestCancelAgentStopsAllStreams(t *testing.T) {
	h := New()
	_, doneA, ca := h.Subscribe("space:ops/fixes", "victim")
	_, doneB, cb := h.Subscribe("inbox:victim", "victim")
	defer ca()
	defer cb()
	select {
	case <-doneA:
		t.Fatal("not cancelled yet")
	default:
	}
	h.CancelAgent("victim")
	// Both the space stream and the inbox stream close, while a stranger's does not.
	for name, d := range map[string]<-chan struct{}{"space": doneA, "inbox": doneB} {
		select {
		case <-d:
		default:
			t.Fatalf("%s stream not cancelled", name)
		}
	}
	_, otherDone, co := h.Subscribe("space:elsewhere", "other")
	defer co()
	h.CancelAgent("victim") // idempotent
	select {
	case <-otherDone:
		t.Fatal("unrelated stream cancelled")
	default:
	}
}

func TestCancelSpaceStopsOnlyThatSpace(t *testing.T) {
	h := New()
	_, done, c1 := h.Subscribe("space:secret", "reader")
	_, otherDone, c2 := h.Subscribe("space:open", "reader")
	defer c1()
	defer c2()
	h.CancelSpace("secret")
	select {
	case <-done:
	default:
		t.Fatal("secret stream not cancelled")
	}
	select {
	case <-otherDone:
		t.Fatal("open stream cancelled by wrong space")
	default:
	}
}
