package ioengine

import (
	"bytes"
	"strings"
	"testing"
)

func TestArchiveRejectsUnsafeNamesAndOversizeContent(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"../x", "/x", "a/../b", "a\\b"} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			a := NewArchive(&b, 10)
			if e := a.Add(name, strings.NewReader("x")); e == nil {
				t.Fatal("unsafe name accepted")
			}
			_ = a.Close()
		})
	}
	var b bytes.Buffer
	a := NewArchive(&b, 3)
	if e := a.Add("file", strings.NewReader("four")); e == nil {
		t.Fatal("oversize accepted")
	}
	_ = a.Close()
}
