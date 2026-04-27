// Package slug converts free-form text (branch names, issue titles) into
// filesystem-safe, compact slugs.
package slug

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultIssueWordLimit  = 4
	defaultIssueSlugMaxLen = 36
)

var (
	nonAlnumDash = regexp.MustCompile(`[^a-z0-9-]+`)
	dashRun      = regexp.MustCompile(`-+`)

	stopwords = map[string]bool{
		"a": true, "an": true, "and": true, "at": true, "by": true,
		"for": true, "from": true, "in": true, "into": true, "of": true,
		"on": true, "or": true, "the": true, "to": true, "with": true,
	}
)

func Sanitize(s string) string {
	s = strings.ToLower(s)
	s = nonAlnumDash.ReplaceAllString(s, "-")
	s = dashRun.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// Trim caps slug at maxLen, preferring a word boundary (last dash) over a
// hard truncation so worktree names don't end mid-word.
func Trim(slug string, maxLen int) string {
	if len(slug) <= maxLen {
		return slug
	}
	cut := slug[:maxLen]
	if i := strings.LastIndex(cut, "-"); i > 0 {
		cut = cut[:i]
	} else {
		cut = slug[:maxLen]
	}
	return strings.TrimRight(cut, "-")
}

// IssueSlug builds "<num>-<compacted-title>". Stopwords drop, "number of"
// collapses to "count", capped by WTREE_ISSUE_WORD_LIMIT and WTREE_ISSUE_SLUG_MAX_LEN.
func IssueSlug(num int, title string) string {
	maxWords := envInt("WTREE_ISSUE_WORD_LIMIT", defaultIssueWordLimit)
	maxLen := envInt("WTREE_ISSUE_SLUG_MAX_LEN", defaultIssueSlugMaxLen)

	sanitized := Sanitize(title)
	if sanitized == "" {
		sanitized = "issue"
	}

	words := strings.Split(sanitized, "-")
	kept := make([]string, 0, maxWords)

	for i := 0; i < len(words); i++ {
		w := words[i]
		if stopwords[w] {
			continue
		}
		if w == "number" && i+1 < len(words) && words[i+1] == "of" {
			kept = append(kept, "count")
			i++
		} else {
			kept = append(kept, w)
		}
		if len(kept) >= maxWords {
			break
		}
	}

	if len(kept) == 0 {
		end := maxWords
		if end > len(words) {
			end = len(words)
		}
		kept = append(kept, words[:end]...)
	}

	compact := Trim(strings.Join(kept, "-"), maxLen)
	if compact == "" {
		compact = Trim(sanitized, maxLen)
	}
	return strconv.Itoa(num) + "-" + compact
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
