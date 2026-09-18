package workspace

import (
	"reflect"
	"testing"
)

// sampleState builds a multi-tab State with a nested split layout and
// cursors, exercising the shapes the round-trip test cares about: a
// single-leaf tab, and a tab with a vsplit whose second child is
// itself an hsplit (so decoding must recurse, not just handle one
// level).
func sampleState() *State {
	return &State{
		Version:   CurrentVersion,
		Dir:       "/home/user/project",
		Cwd:       "/home/user/project/src",
		ActiveTab: 1,
		Tabs: []TabState{
			{
				ActivePane: 0,
				Layout: &Node{
					Kind:   "leaf",
					Path:   "main.go",
					Cursor: &CursorLoc{X: 4, Y: 10},
				},
			},
			{
				ActivePane: 1,
				Layout: &Node{
					Kind: "vsplit",
					Children: []*Node{
						{
							Kind:       "leaf",
							Proportion: 0.3,
							Path:       "README.md",
							Cursor:     &CursorLoc{X: 0, Y: 0},
						},
						{
							Kind:       "hsplit",
							Proportion: 0.7,
							Children: []*Node{
								{
									Kind:       "leaf",
									Proportion: 0.5,
									Path:       "internal/pkg/foo.go",
									Cursor:     &CursorLoc{X: 12, Y: 40},
								},
								{
									Kind:       "leaf",
									Proportion: 0.5,
									Path:       "/etc/hosts",
									Cursor:     &CursorLoc{X: 0, Y: 3},
								},
							},
						},
					},
				},
			},
		},
	}
}

// TestStateRoundTripFixpoint proves the schema is stable under
// encode/decode: encoding a decoded state must reproduce the exact
// same bytes (and the decoded value must deep-equal the original),
// so a saved workspace read back by a later version of this package
// (with no schema change) is byte-identical on the next save.
func TestStateRoundTripFixpoint(t *testing.T) {
	original := sampleState()

	data1, err := Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(data1)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("decoded state does not match original:\ngot  %+v\nwant %+v", decoded, original)
	}

	data2, err := Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}

	if string(data1) != string(data2) {
		t.Fatalf("encode/decode is not a fixpoint:\n--- first ---\n%s\n--- second ---\n%s", data1, data2)
	}
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	if _, err := Decode([]byte("not json")); err == nil {
		t.Fatal("expected an error decoding invalid JSON")
	}
}

// TestStateRoundTripWithScratchContent proves the schema extension
// v3 adds for the persistent scratch workspace (D-55): a leaf with no
// Path (a scratch, unnamed/unsaved buffer) carries its text inline in
// Content, and that text survives Encode/Decode intact, while a leaf
// with a real Path keeps storing only the path, exactly as v1 does.
func TestStateRoundTripWithScratchContent(t *testing.T) {
	original := &State{
		Version:   CurrentVersion,
		ActiveTab: 0,
		Tabs: []TabState{
			{
				ActivePane: 0,
				Layout: &Node{
					Kind: "vsplit",
					Children: []*Node{
						{
							Kind:       "leaf",
							Proportion: 0.5,
							Path:       "",
							Content:    "unsaved scratch text\nsecond line\n",
							Cursor:     &CursorLoc{X: 3, Y: 1},
						},
						{
							Kind:       "leaf",
							Proportion: 0.5,
							Path:       "/home/user/real.txt",
							Cursor:     &CursorLoc{X: 0, Y: 0},
						},
					},
				},
			},
		},
	}

	data, err := Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("decoded state does not match original:\ngot  %+v\nwant %+v", decoded, original)
	}

	scratchLeaf := decoded.Tabs[0].Layout.Children[0]
	if scratchLeaf.Path != "" {
		t.Fatalf("scratch leaf Path = %q, want empty", scratchLeaf.Path)
	}
	if scratchLeaf.Content != "unsaved scratch text\nsecond line\n" {
		t.Fatalf("scratch leaf Content = %q, want the original text", scratchLeaf.Content)
	}

	realLeaf := decoded.Tabs[0].Layout.Children[1]
	if realLeaf.Path != "/home/user/real.txt" {
		t.Fatalf("real-file leaf Path = %q, want /home/user/real.txt", realLeaf.Path)
	}
	if realLeaf.Content != "" {
		t.Fatalf("real-file leaf Content = %q, want empty (paths only)", realLeaf.Content)
	}
}
