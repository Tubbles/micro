package action

import (
	"reflect"
	"testing"
)

func TestDisambiguateTitles(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "TODO example: one collision, two distinct dirs",
			in:   []string{"a/b/d", "a/c/d", "a/e"},
			want: []string{"b/d", "c/d", "e"},
		},
		{
			name: "singleton stays at basename",
			in:   []string{"foo/bar"},
			want: []string{"bar"},
		},
		{
			name: "deep collision walks up multiple levels",
			in:   []string{"x/a/b/d", "y/a/b/d", "a/e"},
			want: []string{"x/a/b/d", "y/a/b/d", "e"},
		},
		{
			name: "identical paths only: shared title at basename",
			in:   []string{"a/b", "a/b"},
			want: []string{"b", "b"},
		},
		{
			name: "same-file pair disambiguates against a third tab",
			in:   []string{"foo/bar.go", "foo/bar.go", "baz/bar.go"},
			want: []string{"foo/bar.go", "foo/bar.go", "baz/bar.go"},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "no collisions: every path keeps its basename",
			in:   []string{"a/x", "b/y", "c/z"},
			want: []string{"x", "y", "z"},
		},
		{
			name: "absolute paths",
			in:   []string{"/home/u/a/file", "/home/u/b/file"},
			want: []string{"a/file", "b/file"},
		},
		{
			name: "saturation: shorter path can't expand, longer one does",
			in:   []string{"foo", "a/foo"},
			want: []string{"foo", "a/foo"},
		},
		{
			name: "duplicate-same-path among three: pair stays grouped, third disambiguates",
			in:   []string{"a/x", "b/x", "a/x"},
			want: []string{"a/x", "b/x", "a/x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := disambiguateTitles(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("disambiguateTitles(%v) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLastNComponents(t *testing.T) {
	cases := []struct {
		path string
		n    int
		want string
	}{
		{"/usr/local/bin/foo", 1, "foo"},
		{"/usr/local/bin/foo", 2, "bin/foo"},
		{"/usr/local/bin/foo", 4, "usr/local/bin/foo"},
		{"/usr/local/bin/foo", 100, "usr/local/bin/foo"},
		{"foo", 1, "foo"},
		{"foo/bar", 1, "bar"},
		{"foo/bar", 2, "foo/bar"},
		{"", 1, ""},
		{"a/b/", 1, "b"},
		{"/foo", 1, "foo"},
	}
	for _, tc := range cases {
		got := lastNComponents(tc.path, tc.n)
		if got != tc.want {
			t.Errorf("lastNComponents(%q, %d) = %q; want %q", tc.path, tc.n, got, tc.want)
		}
	}
}
