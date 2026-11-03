package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestMermaidPreviewIsOneEscapedFence(t *testing.T) {
	source := []byte("flowchart LR\n    A[\"<script>alert('x')</script> & co\"] --> B\n")
	original := bytes.Clone(source)
	preview, err := Builtins().Lookup("flow.mmd").Preview(source, Context{Path: "flow.mmd"})
	if err != nil {
		t.Fatal(err)
	}
	want := "<pre><code class=\"language-mermaid\">flowchart LR\n    A[&quot;&lt;script&gt;alert('x')&lt;/script&gt; &amp; co&quot;] --&gt; B\n</code></pre>\n"
	if preview != want {
		t.Fatalf("preview = %q, want %q", preview, want)
	}
	if strings.Contains(preview, "<script>") {
		t.Fatal("Mermaid source was rendered as raw HTML")
	}
	if !bytes.Equal(source, original) {
		t.Fatal("preview changed the source bytes")
	}
	if got := strings.TrimSuffix(plainHTML(preview), "\n"); got != string(source) {
		t.Fatalf("preview text = %q, want original source", got)
	}
}
