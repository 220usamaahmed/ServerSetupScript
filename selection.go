package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "sss")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "sss")
}

func selectionPath() string { return filepath.Join(configDir(), "selection") }

// LoadSelection returns the starting checklist: saved choices where they exist,
// each step's header default for steps not seen before.
func LoadSelection(steps []*Step) map[string]bool {
	sel := map[string]bool{}
	for _, s := range steps {
		sel[s.ID] = s.Default
	}
	f, err := os.Open(selectionPath())
	if err != nil {
		return sel
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		id, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		if _, known := sel[id]; known {
			sel[id] = parseBool(v)
		}
	}
	return sel
}

func SaveSelection(steps []*Step, sel map[string]bool) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, s := range steps {
		if s.Hidden {
			continue
		}
		v := "off"
		if sel[s.ID] {
			v = "on"
		}
		fmt.Fprintf(&b, "%s=%s\n", s.ID, v)
	}
	return os.WriteFile(selectionPath(), []byte(b.String()), 0o644)
}
