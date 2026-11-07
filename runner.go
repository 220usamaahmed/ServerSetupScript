package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Status int

const (
	Pending Status = iota
	Running
	Done
	Already
	Skipped
	Failed
)

func (s Status) String() string {
	return [...]string{"pending", "running", "done", "already done", "skipped", "failed"}[s]
}

// OK reports whether the step's effect is in place (so dependents may run).
func (s Status) OK() bool { return s == Done || s == Already }

const (
	exitAlready = 10
	exitNA      = 11
)

// Item is a planned step plus its progress, owned by whoever consumes events.
type Item struct {
	Step    *Step
	Status  Status
	Detail  string   // one-line outcome: last output line, or the skip reason
	Tail    []string // last output lines, kept for failures
	Last    string   // latest output line while running
	Started time.Time
	Elapsed time.Duration
	Log     string
}

type EventKind int

const (
	EvStart EventKind = iota
	EvLine
	EvFinish
	EvAllDone
)

type Event struct {
	Kind   EventKind
	ID     string
	Line   string
	Status Status
	Detail string
	Tail   []string
	Log    string
}

type Runner struct {
	Remote *Remote
	Common string
	LogDir string
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// lineWriter splits streamed output into lines, logs them, and reports them.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	log  *os.File
	emit func(string)
	tail []string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.log != nil {
		w.log.Write(p)
	}
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexAny(string(w.buf), "\r\n")
		if i < 0 {
			break
		}
		line := strings.TrimSpace(ansiRe.ReplaceAllString(string(w.buf[:i]), ""))
		w.buf = w.buf[i+1:]
		if line != "" {
			w.tail = append(w.tail, line)
			if len(w.tail) > 8 {
				w.tail = w.tail[1:]
			}
			w.emit(line)
		}
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if line := strings.TrimSpace(ansiRe.ReplaceAllString(string(w.buf), "")); line != "" {
		w.tail = append(w.tail, line)
		w.emit(line)
	}
	w.buf = nil
}

func (w *lineWriter) lastTail() ([]string, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := append([]string(nil), w.tail...)
	last := ""
	if len(t) > 0 {
		last = t[len(t)-1]
	}
	return t, last
}

// Run executes todo in order. prior holds the statuses of steps outside todo
// (already finished in an earlier pass) for requirement checks. A failed or
// skipped step never stops the others; only its dependents are skipped.
func (r *Runner) Run(ctx context.Context, todo []*Step, prior map[string]Status, emit func(Event)) {
	status := map[string]Status{}
	for k, v := range prior {
		status[k] = v
	}
	os.MkdirAll(r.LogDir, 0o755)
	for _, s := range todo {
		if ctx.Err() != nil {
			emit(Event{Kind: EvFinish, ID: s.ID, Status: Skipped, Detail: "interrupted"})
			status[s.ID] = Skipped
			continue
		}
		if reason := r.skipReason(s, status); reason != "" {
			status[s.ID] = Skipped
			emit(Event{Kind: EvFinish, ID: s.ID, Status: Skipped, Detail: reason})
			continue
		}
		emit(Event{Kind: EvStart, ID: s.ID})
		res := r.runOne(ctx, s, emit)
		status[s.ID] = res.Status
		emit(res)
	}
	emit(Event{Kind: EvAllDone})
}

func (r *Runner) skipReason(s *Step, status map[string]Status) string {
	if s.NeedsRoot && r.Remote.Info.Root == "none" {
		return "needs root or passwordless sudo, which this login doesn't have"
	}
	for _, req := range s.Requires {
		if st := status[req]; !st.OK() {
			why := "was skipped"
			if st == Failed {
				why = "failed"
			}
			return fmt.Sprintf("requires %s, which %s", req, why)
		}
	}
	return ""
}

func (r *Runner) runOne(ctx context.Context, s *Step, emit func(Event)) Event {
	logPath := filepath.Join(r.LogDir, fmt.Sprintf("%02d-%s.log", s.Order, s.ID))
	lf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if lf != nil {
		defer lf.Close()
	}
	w := &lineWriter{log: lf, emit: func(l string) { emit(Event{Kind: EvLine, ID: s.ID, Line: l}) }}
	code, err := r.Remote.RunStep(ctx, s, r.Common, w)
	w.flush()
	tail, last := w.lastTail()
	ev := Event{Kind: EvFinish, ID: s.ID, Tail: tail, Detail: last, Log: logPath}
	switch {
	case err != nil:
		ev.Status, ev.Detail = Failed, "could not run: "+err.Error()
	case ctx.Err() != nil:
		ev.Status, ev.Detail = Failed, "interrupted"
	case code == 0:
		ev.Status = Done
		if last == "" {
			ev.Detail = "done"
		}
	case code == exitAlready:
		ev.Status = Already
	case code == exitNA:
		ev.Status = Skipped
	case code == 124 || code == 137:
		ev.Status, ev.Detail = Failed, fmt.Sprintf("timed out after %s", s.Timeout)
	case code == 255:
		ev.Status, ev.Detail = Failed, "ssh connection lost: "+last
	default:
		ev.Status = Failed
		if last == "" {
			ev.Detail = fmt.Sprintf("exited with status %d", code)
		}
	}
	return ev
}
