package feed

import "testing"

func TestPublishReachesSubscriber(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("space:ops/fixes")
	defer cancel()
	h.Publish(Event{ID: 7, Space: "ops/fixes", NoteID: "n", BodyHash: "h"})
	ev := <-ch
	if ev.ID != 7 || ev.NoteID != "n" {
		t.Fatal(ev)
	}
}

func TestDMReachesBothInboxes(t *testing.T) {
	h := New()
	a, ca := h.Subscribe("inbox:aaa")
	b, cb := h.Subscribe("inbox:bbb")
	defer ca()
	defer cb()
	h.Publish(Event{ID: 1, Space: "dm/aaa/bbb"})
	if (<-a).ID != 1 || (<-b).ID != 1 {
		t.Fatal("inbox")
	}
}
