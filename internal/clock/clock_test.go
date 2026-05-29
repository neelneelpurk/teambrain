package clock

import (
	"testing"
	"time"
)

func TestFakeNowIsStable(t *testing.T) {
	t.Parallel()

	pinned := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)
	c := NewFake(pinned)

	if !c.Now().Equal(pinned) {
		t.Fatalf("Now() = %v, want %v", c.Now(), pinned)
	}
	// A second read returns the same instant (unlike the system clock).
	if !c.Now().Equal(c.Now()) {
		t.Fatalf("Fake clock advanced on its own")
	}
}

func TestFakeAdvance(t *testing.T) {
	t.Parallel()

	c := NewFake(time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC))
	c.Advance(90 * time.Minute)

	want := time.Date(2026, time.May, 29, 13, 30, 0, 0, time.UTC)
	if !c.Now().Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", c.Now(), want)
	}
}

func TestSystemImplementsClock(t *testing.T) {
	t.Parallel()

	var _ Clock = System{}
	var _ Clock = (*Fake)(nil)

	if (System{}).Now().IsZero() {
		t.Fatal("System.Now() returned the zero time")
	}
}
