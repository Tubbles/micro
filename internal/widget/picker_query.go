package widget

import (
	"sort"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

// Query grammar (fzf "extended-search mode" style):
//
//   query     := and_group ( WS and_group )*
//   and_group := atom ( '|' atom )*
//   atom      := [ '!' ] [ "'" | '^' ] text [ '$' ]
//
// Tokens separate on unescaped whitespace; a literal `|` token unions
// the next atom into the current AND-group. Backslash escapes the
// next character so users can type literal spaces, pipes, or operator
// characters (e.g. `\ ` for a literal space, `\|` for a literal `|`).
//
// Atom kinds:
//
//   foo     fuzzy substring (sahilm/fuzzy)
//   'foo    exact substring
//   ^foo    literal prefix
//   foo$    literal suffix
//   ^foo$   literal whole-label equality
//
// A leading `!` negates the atom in any of the above forms.
//
// Smart-case (fzf convention) applies to the operator atoms: if the
// atom text contains any uppercase character the match is
// case-sensitive, otherwise case-insensitive. Fuzzy atoms are always
// case-insensitive — that is sahilm/fuzzy's only mode and matches
// micro's prior picker behaviour.

type atomKind int

const (
	atomFuzzy atomKind = iota
	atomExact
	atomPrefix
	atomSuffix
	atomEqual
)

type queryAtom struct {
	kind   atomKind
	text   string
	negate bool
}

type orGroup []queryAtom

type parsedQuery []orGroup

// parseQuery splits s into AND-groups of OR-alternatives. Empty atoms
// (lone `!`, `^`, `$`, or stray `|`) are dropped silently. A query
// that yields zero atoms returns an empty parsedQuery; matchQuery
// treats that as "no filter".
func parseQuery(s string) parsedQuery {
	tokens := tokenizeQuery(s)
	var out parsedQuery
	pendingOr := false
	for _, tok := range tokens {
		if tok.pipe {
			if len(out) > 0 {
				pendingOr = true
			}
			continue
		}
		atom, ok := classifyAtom(tok.text)
		if !ok {
			pendingOr = false
			continue
		}
		if pendingOr && len(out) > 0 {
			out[len(out)-1] = append(out[len(out)-1], atom)
		} else {
			out = append(out, orGroup{atom})
		}
		pendingOr = false
	}
	return out
}

// rawToken is the tokenizer's output. text is the segment with
// backslash escapes applied; pipe is true iff the segment was exactly
// one unescaped `|` (the OR marker per fzf).
type rawToken struct {
	text string
	pipe bool
}

// tokenizeQuery splits s into whitespace-delimited segments. Within a
// segment, backslash escapes the next rune (`\ ` for a literal space,
// `\|` for a literal pipe). A segment that consists of exactly the
// single unescaped character `|` is the OR marker (rawToken.pipe =
// true); any other `|` is just a literal character within its
// segment. This matches fzf's extended-search lexing: `foo|bar` is a
// single fuzzy term whose text contains a literal `|`; `foo | bar`
// is two terms ORed together. The two behaviours differ.
func tokenizeQuery(s string) []rawToken {
	var tokens []rawToken
	var cur strings.Builder
	bare := false
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, rawToken{text: cur.String(), pipe: bare})
			cur.Reset()
			bare = false
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) {
			cur.WriteRune(runes[i+1])
			bare = false
			i++
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		if r == '|' && cur.Len() == 0 {
			cur.WriteRune('|')
			bare = true
			continue
		}
		cur.WriteRune(r)
		bare = false
	}
	flush()
	return tokens
}

// classifyAtom peels off `!`, `'`, `^`, and trailing `$` to figure
// out atom kind. Returns false if no text is left (e.g. `^` alone).
func classifyAtom(tok string) (queryAtom, bool) {
	negate := false
	if strings.HasPrefix(tok, "!") {
		negate = true
		tok = tok[1:]
	}
	prefix := strings.HasPrefix(tok, "^")
	if prefix {
		tok = tok[1:]
	}
	exact := !prefix && strings.HasPrefix(tok, "'")
	if exact {
		tok = tok[1:]
	}
	suffix := strings.HasSuffix(tok, "$")
	if suffix {
		tok = tok[:len(tok)-1]
	}
	if tok == "" {
		return queryAtom{}, false
	}
	var kind atomKind
	switch {
	case prefix && suffix:
		kind = atomEqual
	case prefix:
		kind = atomPrefix
	case suffix:
		kind = atomSuffix
	case exact:
		kind = atomExact
	case negate:
		// fzf semantics: bare-negation defaults to exact substring,
		// not fuzzy. `!foo` means "does not contain the literal foo".
		// Without this, a query like `'main !test` would negate the
		// fuzzy-match of "test" instead of its literal substring,
		// which is rarely what users intend.
		kind = atomExact
	default:
		kind = atomFuzzy
	}
	return queryAtom{kind: kind, text: tok, negate: negate}, true
}

// matchQuery runs q against every label in sources and returns the
// surviving entries in score-descending order, stable on original
// index. The shape is identical to a fuzzy.Find return so the picker
// renderer keeps working unchanged.
//
// Returns:
//   - nil when q has zero atoms (no filter, show all items)
//   - non-nil empty slice when q has atoms but nothing matched
//     (picker's OnSubmit path)
//   - non-empty slice otherwise
func matchQuery(q parsedQuery, sources []string) []fuzzy.Match {
	if len(q) == 0 {
		return nil
	}
	fuzzyHits := precomputeFuzzyHits(q, sources)
	results := make([]fuzzy.Match, 0)
	for idx, label := range sources {
		lower := strings.ToLower(label)
		score, hits, ok := evalLabel(q, idx, label, lower, fuzzyHits)
		if !ok {
			continue
		}
		results = append(results, fuzzy.Match{
			Str:            label,
			Index:          idx,
			Score:          score,
			MatchedIndexes: hits,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Index < results[j].Index
	})
	return results
}

// precomputeFuzzyHits runs fuzzy.Find once per fuzzy atom so the
// per-item loop is O(items) not O(items*fuzzy_atoms).
func precomputeFuzzyHits(q parsedQuery, sources []string) [][]map[int]fuzzy.Match {
	out := make([][]map[int]fuzzy.Match, len(q))
	for gi, group := range q {
		out[gi] = make([]map[int]fuzzy.Match, len(group))
		for ai, atom := range group {
			if atom.kind != atomFuzzy {
				continue
			}
			matches := fuzzyFind(atom.text, sources)
			m := make(map[int]fuzzy.Match, len(matches))
			for _, fm := range matches {
				m[fm.Index] = fm
			}
			out[gi][ai] = m
		}
	}
	return out
}

// evalLabel walks the AND-groups, OR-ing within each. Returns the
// summed fuzzy score and the sorted-deduplicated union of byte
// indexes contributed by positive atoms.
func evalLabel(q parsedQuery, idx int, label, lower string, fuzzyHits [][]map[int]fuzzy.Match) (int, []int, bool) {
	score := 0
	var hits []int
	for gi, group := range q {
		var altScore int
		var altHits []int
		matched := false
		for ai, atom := range group {
			ok, h, s := evalAtom(atom, idx, label, lower, fuzzyHits[gi][ai])
			if !ok {
				continue
			}
			matched = true
			altScore = s
			altHits = h
			break
		}
		if !matched {
			return 0, nil, false
		}
		score += altScore
		if len(altHits) > 0 {
			hits = append(hits, altHits...)
		}
	}
	if len(hits) > 1 {
		sort.Ints(hits)
		hits = dedupeSorted(hits)
	}
	return score, hits, true
}

// evalAtom returns (matched, hits, score) for one atom against one
// label. Negation is applied here: a negated atom that the base
// predicate would reject becomes a match with no highlight bytes and
// no score contribution.
func evalAtom(atom queryAtom, idx int, label, lower string, fuzzyMap map[int]fuzzy.Match) (bool, []int, int) {
	matched, hits, score := atomBase(atom, idx, label, lower, fuzzyMap)
	if atom.negate {
		return !matched, nil, 0
	}
	return matched, hits, score
}

func atomBase(atom queryAtom, idx int, label, lower string, fuzzyMap map[int]fuzzy.Match) (bool, []int, int) {
	switch atom.kind {
	case atomFuzzy:
		m, ok := fuzzyMap[idx]
		if !ok {
			return false, nil, 0
		}
		return true, append([]int(nil), m.MatchedIndexes...), m.Score
	case atomExact:
		hay, needle := caseFold(label, lower, atom.text)
		i := strings.Index(hay, needle)
		if i < 0 {
			return false, nil, 0
		}
		return true, byteRange(i, i+len(needle)), 0
	case atomPrefix:
		hay, needle := caseFold(label, lower, atom.text)
		if !strings.HasPrefix(hay, needle) {
			return false, nil, 0
		}
		return true, byteRange(0, len(needle)), 0
	case atomSuffix:
		hay, needle := caseFold(label, lower, atom.text)
		if !strings.HasSuffix(hay, needle) {
			return false, nil, 0
		}
		return true, byteRange(len(label)-len(needle), len(label)), 0
	case atomEqual:
		hay, needle := caseFold(label, lower, atom.text)
		if hay != needle {
			return false, nil, 0
		}
		return true, byteRange(0, len(label)), 0
	}
	return false, nil, 0
}

// caseFold picks the (haystack, needle) pair under smart-case rules.
// If the needle has any uppercase rune the comparison is
// case-sensitive (use the original label); otherwise case-insensitive
// (use the pre-lowered label). The pre-lowered label is the caller's
// per-item cache.
//
// For ASCII haystacks (the dominant filename case) the lowered byte
// layout matches the original byte-for-byte, so highlight ranges
// derived from `lower` indexes line up with the rendered original.
// Non-ASCII labels with letters whose lowercase byte-length differs
// from the original (rare) may show off-by-a-few-bytes highlight;
// the match decision itself is still correct.
func caseFold(label, lower, needle string) (string, string) {
	for _, r := range needle {
		if unicode.IsUpper(r) {
			return label, needle
		}
	}
	return lower, needle
}

func byteRange(start, end int) []int {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	out := make([]int, end-start)
	for i := range out {
		out[i] = start + i
	}
	return out
}

func dedupeSorted(s []int) []int {
	n := 0
	for i, v := range s {
		if i == 0 || v != s[n-1] {
			s[n] = v
			n++
		}
	}
	return s[:n]
}
