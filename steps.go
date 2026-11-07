package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Step is one install/config action, backed by a shell script in the steps dir.
type Step struct {
	ID          string // filename without order prefix and .sh
	Order       int    // numeric filename prefix
	Path        string
	Name        string
	Description string
	Default     bool
	Requires    []string
	NeedsRoot   bool
	Hidden      bool // not shown in the checklist; pulled in only via requires
	Timeout     time.Duration
}

var (
	fileRe   = regexp.MustCompile(`^(\d+)-([a-z0-9][a-z0-9-]*)\.sh$`)
	headerRe = regexp.MustCompile(`^#\s*sss-([a-z-]+):\s*(.*?)\s*$`)
)

func parseBool(v string) bool {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true
	}
	return false
}

// LoadSteps reads every NN-name.sh in dir and parses its "# sss-key: value" header.
func LoadSteps(dir string) ([]*Step, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading steps dir: %w", err)
	}
	var steps []*Step
	seen := map[string]string{}
	for _, e := range entries {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		order, _ := strconv.Atoi(m[1])
		s := &Step{ID: m[2], Order: order, Path: filepath.Join(dir, e.Name()),
			Default: true, Timeout: 10 * time.Minute}
		s.Name = s.ID
		if prev, dup := seen[s.ID]; dup {
			return nil, fmt.Errorf("duplicate step id %q (%s and %s)", s.ID, prev, e.Name())
		}
		seen[s.ID] = e.Name()
		f, err := os.Open(s.Path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for i := 0; sc.Scan() && i < 60; i++ {
			h := headerRe.FindStringSubmatch(sc.Text())
			if h == nil {
				continue
			}
			k, v := h[1], h[2]
			switch k {
			case "name":
				s.Name = v
			case "description":
				s.Description = v
			case "default":
				s.Default = parseBool(v)
			case "requires":
				for _, r := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
					s.Requires = append(s.Requires, r)
				}
			case "needs-root":
				s.NeedsRoot = parseBool(v)
			case "hidden":
				s.Hidden = parseBool(v)
			case "timeout":
				secs, err := strconv.Atoi(v)
				if err != nil || secs <= 0 {
					f.Close()
					return nil, fmt.Errorf("%s: bad sss-timeout %q (seconds expected)", e.Name(), v)
				}
				s.Timeout = time.Duration(secs) * time.Second
			default:
				f.Close()
				return nil, fmt.Errorf("%s: unknown header sss-%s", e.Name(), k)
			}
		}
		f.Close()
		steps = append(steps, s)
	}
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Order != steps[j].Order {
			return steps[i].Order < steps[j].Order
		}
		return steps[i].ID < steps[j].ID
	})
	byID := stepIndex(steps)
	for _, s := range steps {
		for _, r := range s.Requires {
			if byID[r] == nil {
				return nil, fmt.Errorf("step %q requires unknown step %q", s.ID, r)
			}
		}
	}
	if _, err := Plan(steps, allOn(steps)); err != nil {
		return nil, err
	}
	return steps, nil
}

func stepIndex(steps []*Step) map[string]*Step {
	m := make(map[string]*Step, len(steps))
	for _, s := range steps {
		m[s.ID] = s
	}
	return m
}

func allOn(steps []*Step) map[string]bool {
	m := map[string]bool{}
	for _, s := range steps {
		m[s.ID] = true
	}
	return m
}

// Plan returns the steps to run: the selected ones plus everything they
// (transitively) require, ordered so requirements come first and otherwise
// by filename prefix.
func Plan(all []*Step, selected map[string]bool) ([]*Step, error) {
	byID := stepIndex(all)
	var out []*Step
	state := map[string]int{} // 1 = visiting, 2 = done
	var visit func(s *Step, chain []string) error
	visit = func(s *Step, chain []string) error {
		switch state[s.ID] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain, " -> "), s.ID)
		}
		state[s.ID] = 1
		reqs := append([]string(nil), s.Requires...)
		sort.Slice(reqs, func(i, j int) bool { return byID[reqs[i]].Order < byID[reqs[j]].Order })
		for _, r := range reqs {
			if err := visit(byID[r], append(chain, s.ID)); err != nil {
				return err
			}
		}
		state[s.ID] = 2
		out = append(out, s)
		return nil
	}
	for _, s := range all {
		if selected[s.ID] {
			if err := visit(s, nil); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Dependents returns the ids of steps that (transitively) require id.
func Dependents(all []*Step, id string) []string {
	var out []string
	changed := true
	deps := map[string]bool{id: true}
	for changed {
		changed = false
		for _, s := range all {
			if deps[s.ID] {
				continue
			}
			for _, r := range s.Requires {
				if deps[r] {
					deps[s.ID] = true
					out = append(out, s.ID)
					changed = true
					break
				}
			}
		}
	}
	return out
}

// Requirements returns the ids that id (transitively) requires.
func Requirements(all []*Step, id string) []string {
	byID := stepIndex(all)
	var out []string
	seen := map[string]bool{}
	var walk func(s *Step)
	walk = func(s *Step) {
		for _, r := range s.Requires {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
				walk(byID[r])
			}
		}
	}
	walk(byID[id])
	return out
}
