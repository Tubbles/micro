package util

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringWidth(t *testing.T) {
	bytes := []byte("\tPot să \tmănânc sticlă și ea nu mă rănește.")

	n := StringWidth(bytes, 23, 4)
	assert.Equal(t, 26, n)
}

func TestSliceVisualEnd(t *testing.T) {
	s := []byte("\thello")
	slc, n, _ := SliceVisualEnd(s, 2, 4)
	assert.Equal(t, []byte("\thello"), slc)
	assert.Equal(t, 2, n)

	slc, n, _ = SliceVisualEnd(s, 1, 4)
	assert.Equal(t, []byte("\thello"), slc)
	assert.Equal(t, 1, n)

	slc, n, _ = SliceVisualEnd(s, 4, 4)
	assert.Equal(t, []byte("hello"), slc)
	assert.Equal(t, 0, n)

	slc, n, _ = SliceVisualEnd(s, 5, 4)
	assert.Equal(t, []byte("ello"), slc)
	assert.Equal(t, 0, n)
}

func TestIntListOpt(t *testing.T) {
	assert.Nil(t, IntListOpt(float64(0)))
	assert.Equal(t, []int{80}, IntListOpt(float64(80)))
	assert.Equal(t, []int{72, 80, 120}, IntListOpt([]any{float64(72), float64(80), float64(120)}))
	assert.Equal(t, []int{80}, IntListOpt([]any{float64(0), float64(80)}))
	assert.Equal(t, []int{}, IntListOpt([]any{}))
	assert.Equal(t, []int{}, IntListOpt([]any{float64(0), float64(0)}))
	assert.Nil(t, IntListOpt("80"))
	assert.Nil(t, IntListOpt(nil))
}

func TestMarshalIndentNoEscape(t *testing.T) {
	v := map[string]any{"x": "tab=>&<"}
	b, err := MarshalIndentNoEscape(v)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(b), "tab=>&<"))
	assert.False(t, strings.Contains(string(b), "\\u003e"))
	assert.False(t, strings.Contains(string(b), "\\u0026"))
	assert.False(t, strings.Contains(string(b), "\\u003c"))

	v2 := map[string]any{"a": "b", "n": float64(4)}
	got, err := MarshalIndentNoEscape(v2)
	assert.NoError(t, err)
	want, err := json.MarshalIndent(v2, "", "    ")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}
