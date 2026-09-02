package dataacquire

import (
	"io"
	"sync"
	"time"
)

// progressInterval throttles progress callbacks. The SSE stream samples on its own
// schedule; reporting on every 32 KiB chunk would just burn CPU updating a value
// nobody reads between ticks.
const progressInterval = 500 * time.Millisecond

// progressWriter counts bytes as they pass through and reports rate and ETA.
// It implements io.Writer so it can sit in the same MultiWriter as the hasher,
// which means the count it reports is exactly the count that is hashed.
type progressWriter struct {
	total    int64
	written  int64
	started  time.Time
	last     time.Time
	interval time.Duration
	report   func(Progress)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if p.report == nil {
		return len(b), nil
	}
	now := time.Now()
	if now.Sub(p.last) < p.interval {
		return len(b), nil
	}
	p.last = now
	p.report(p.sample(now))
	return len(b), nil
}

// flush emits a final sample so the last update is the completed one rather than
// whatever the throttle happened to allow.
func (p *progressWriter) flush() {
	if p.report != nil {
		p.report(p.sample(time.Now()))
	}
}

// sample builds the progress reading at a moment.
func (p *progressWriter) sample(now time.Time) Progress {
	out := Progress{Downloaded: p.written, Total: p.total}
	elapsed := now.Sub(p.started).Seconds()
	if elapsed <= 0 || p.written == 0 {
		return out
	}
	rate := float64(p.written) / elapsed
	out.BytesPerSec = int64(rate)
	if p.total > p.written && rate > 0 {
		eta := int64(float64(p.total-p.written) / rate)
		out.ETASec = &eta
	}
	return out
}

var _ io.Writer = (*progressWriter)(nil)

// slot holds the live state of one in-flight data step.
//
// Fetch and extract run in goroutines inside go-app, so unlike a training subprocess
// they can simply publish into memory — the same shape a trainer's progress file uses for
// its per-format status. One slot per step kind is enough because the data lane
// admits one step at a time; the lane, not this variable, is what enforces that.
type slot[T any] struct {
	mu       sync.Mutex
	active   bool
	progress T
	source   string
}

func (s *slot[T]) publish(source string, p T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.progress = p
	s.source = source
}

// clear marks the step finished. Callers defer it so a failed or cancelled step
// cannot leave the console showing work that is not running.
func (s *slot[T]) clear() {
	var zero T
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	s.progress = zero
	s.source = ""
}

func (s *slot[T]) status() (T, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress, s.source, s.active
}

var (
	liveFetch   slot[Progress]
	liveExtract slot[ExtractProgress]
)

// PublishProgress records the current download state for the ops console.
func PublishProgress(source string, p Progress) { liveFetch.publish(source, p) }

// ClearProgress marks the download finished.
func ClearProgress() { liveFetch.clear() }

// Status is the live download state, or ok=false when nothing is downloading.
func Status() (Progress, string, bool) { return liveFetch.status() }

// PublishExtractProgress records the current extraction state for the ops console.
func PublishExtractProgress(source string, p ExtractProgress) { liveExtract.publish(source, p) }

// ClearExtractProgress marks the extraction finished.
func ClearExtractProgress() { liveExtract.clear() }

// ExtractStatus is the live extraction state, or ok=false when nothing is extracting.
func ExtractStatus() (ExtractProgress, string, bool) { return liveExtract.status() }
