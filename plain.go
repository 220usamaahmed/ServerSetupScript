package main

import (
	"context"
	"fmt"
	"io"
)

// runPlain runs the plan with line-by-line output (for --yes or no TTY).
func runPlain(out io.Writer, runner *Runner, plan []*Step, verbose bool) int {
	byID := map[string]*Step{}
	idx := map[string]int{}
	for i, s := range plan {
		byID[s.ID], idx[s.ID] = s, i+1
	}
	results := map[string]Event{}
	runner.Run(context.Background(), plan, nil, func(e Event) {
		switch e.Kind {
		case EvStart:
			fmt.Fprintf(out, "[%d/%d] %s ...\n", idx[e.ID], len(plan), byID[e.ID].Name)
		case EvLine:
			if verbose {
				fmt.Fprintf(out, "        %s\n", e.Line)
			}
		case EvFinish:
			results[e.ID] = e
			fmt.Fprintf(out, "[%d/%d] %s: %s", idx[e.ID], len(plan), byID[e.ID].Name, e.Status)
			if e.Detail != "" {
				fmt.Fprintf(out, " - %s", e.Detail)
			}
			fmt.Fprintln(out)
		}
	})
	failed := 0
	fmt.Fprintln(out, "\nSummary:")
	for _, s := range plan {
		e := results[s.ID]
		fmt.Fprintf(out, "  %-20s %-13s %s\n", s.ID, e.Status, e.Detail)
		if e.Status == Failed {
			failed++
			for _, l := range lastN(e.Tail, 5) {
				fmt.Fprintf(out, "      | %s\n", l)
			}
		}
	}
	fmt.Fprintf(out, "\nlogs: %s\n", runner.LogDir)
	if failed > 0 {
		return 1
	}
	return 0
}

func lastN(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
