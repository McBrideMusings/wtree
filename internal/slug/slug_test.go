package slug

import "testing"

func TestSanitize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"Hello World", "hello-world"},
		{"Foo/Bar:Baz", "foo-bar-baz"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"weird---runs____here", "weird-runs-here"},
		{"keep-dashes-already", "keep-dashes-already"},
		{"UPPER-Case-Title 42", "upper-case-title-42"},
		{"!!!", ""},
		{"don't \"quote\" me, please.", "don-t-quote-me-please"},
	}
	for _, c := range cases {
		got := Sanitize(c.in)
		if got != c.want {
			t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrim(t *testing.T) {
	cases := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"this-is-a-longer-slug-string", 12, "this-is-a"},
		{"this-is-a-longer-slug-string", 6, "this"},
		{"nodashes-but-very-long-string-here", 6, "nodash"},
		{"-leadingdash", 5, "-lead"},
	}
	for _, c := range cases {
		got := Trim(c.in, c.maxLen)
		if got != c.want {
			t.Errorf("Trim(%q, %d) = %q, want %q", c.in, c.maxLen, got, c.want)
		}
	}
}

func TestIssueSlug(t *testing.T) {
	t.Setenv("WTREE_ISSUE_WORD_LIMIT", "")
	t.Setenv("WTREE_ISSUE_SLUG_MAX_LEN", "")
	cases := []struct {
		num   int
		title string
		want  string
	}{
		{42, "Add a feature for the user", "42-add-feature-user"},
		{7, "Number of retries before giving up", "7-count-retries-before-giving"},
		{1, "the and of to", "1-the-and-of-to"},
		{99, "", "99-issue"},
		{
			3,
			"Implement an extremely-long descriptive title that should be truncated nicely",
			"3-implement-extremely-long-descriptive",
		},
		{
			5,
			"Implement extra extremely descriptive title that should be done",
			"5-implement-extra-extremely",
		},
		{12, "Refactor parser", "12-refactor-parser"},
	}
	for _, c := range cases {
		got := IssueSlug(c.num, c.title)
		if got != c.want {
			t.Errorf("IssueSlug(%d, %q) = %q, want %q", c.num, c.title, got, c.want)
		}
	}
}

func TestIssueSlugEnvOverrides(t *testing.T) {
	t.Setenv("WTREE_ISSUE_WORD_LIMIT", "2")
	t.Setenv("WTREE_ISSUE_SLUG_MAX_LEN", "20")
	got := IssueSlug(101, "Add a brand new authentication system")
	want := "101-add-brand"
	if got != want {
		t.Errorf("IssueSlug = %q, want %q", got, want)
	}
}
