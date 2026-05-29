// Package clock abstracts the current time behind a small interface so that
// time-dependent behavior (commit messages, manifest timestamps) is
// deterministic under test. Production code uses System; tests use Fake.
package clock

import "time"

// Clock reports the current time. It is the only time source production code
// should read, so that tests can pin it.
type Clock interface {
	Now() time.Time
}

// System is the real clock backed by time.Now.
type System struct{}

// Now returns the current wall-clock time.
func (System) Now() time.Time { return time.Now() }

// Fake is a controllable clock for tests. The zero value reports the zero time;
// set T or use NewFake.
type Fake struct {
	T time.Time
}

// NewFake returns a Fake pinned to t.
func NewFake(t time.Time) *Fake { return &Fake{T: t} }

// Now returns the fake's current time.
func (f *Fake) Now() time.Time { return f.T }

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) { f.T = f.T.Add(d) }
