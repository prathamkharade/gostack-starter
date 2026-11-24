package config

import "testing"

func TestStrTreatsCommentsAndBlanksAsUnset(t *testing.T) {
	cases := map[string]string{
		"":                     "def",
		"   ":                  "def",
		"#":                    "def",
		"# e.g. https://cdn.x": "def",
		" https://cdn.x ":      "https://cdn.x",
	}
	for in, want := range cases {
		t.Setenv("GOSTACK_TEST_STR", in)
		if got := str("GOSTACK_TEST_STR", "def"); got != want {
			t.Errorf("str(%q) = %q, want %q", in, got, want)
		}
	}
	if got := str("GOSTACK_TEST_UNSET_KEY", "def"); got != "def" {
		t.Errorf("unset key = %q", got)
	}
}
