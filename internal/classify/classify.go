// Package classify decides what kind of input the user passed to `wtree add`.
package classify

import (
	"regexp"
	"strconv"
	"strings"
)

// Kind enumerates the recognized input shapes.
type Kind int

const (
	KindText   Kind = iota // free-form branch / new-branch name
	KindNumber             // bare issue/PR number, with or without leading "#"
	KindPR                 // GitHub PR URL
	KindIssue              // GitHub issue URL
)

// Result is the outcome of classifying a single input string.
type Result struct {
	Kind   Kind
	Number int    // populated for KindNumber, KindPR, KindIssue
	NWO    string // "owner/repo" — populated for KindPR, KindIssue
	Text   string // original input, for KindText
}

var (
	prURL    = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/pull/(\d+)`)
	issueURL = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/issues/(\d+)`)
	number   = regexp.MustCompile(`^\d+$`)
)

// Classify inspects input and returns its kind plus parsed components.
func Classify(input string) Result {
	if m := prURL.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[2])
		return Result{Kind: KindPR, NWO: m[1], Number: n}
	}
	if m := issueURL.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[2])
		return Result{Kind: KindIssue, NWO: m[1], Number: n}
	}
	stripped := strings.TrimPrefix(input, "#")
	if number.MatchString(stripped) {
		n, _ := strconv.Atoi(stripped)
		return Result{Kind: KindNumber, Number: n}
	}
	return Result{Kind: KindText, Text: input}
}
