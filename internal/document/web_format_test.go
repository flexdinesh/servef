package document

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

func formatWith(t *testing.T, name, source string) string {
	t.Helper()
	original := []byte(source)
	formatted, err := Builtins().Lookup(name).Format(original)
	if err != nil {
		t.Fatalf("format %s: %v", name, err)
	}
	if string(original) != source {
		t.Fatal("formatting mutated source")
	}
	return string(formatted)
}

func withoutSpace(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func TestCSSFormatReindentsWithoutChangingTokens(t *testing.T) {
	source := `@import url(a.css) screen;/* lead */a:hover,b>c  .d{color:RED;background:url(data:image/png;base64,AA==) no-repeat;margin:0 auto /* trailing */}
@media (min-width:600px){.x::before{content:"a;{b}"}.y{}}
.card{&:hover{color:blue}& .x:focus{a:b}}`
	want := `@import url(a.css) screen; /* lead */
a:hover,b>c .d {
  color: RED;
  background: url(data:image/png;base64,AA==) no-repeat;
  margin: 0 auto /* trailing */
}

@media (min-width:600px) {
  .x::before {
    content: "a;{b}"
  }
  .y {}
}

.card {
  &:hover {
    color: blue
  }
  & .x:focus {
    a: b
  }
}
`
	got := formatWith(t, "site.css", source)
	if got != want {
		t.Fatalf("css format:\n%s", got)
	}
	if withoutSpace(got) != withoutSpace(source) {
		t.Fatal("css format changed tokens")
	}
}

func TestJavaScriptFormatPreservesTokensAndLineBreaks(t *testing.T) {
	source := "#!/usr/bin/env node\n// lead\nconst a='x',b=0xFF;/* inner */function f(){return a+b}\n" +
		"let re=/a\\/b{2}/g,d=a/2/1,t=`x${a}y${ {k:1}.k }\n  kept`;\n" +
		"if(a){x()}else{y()}\ntry{q()}catch(e){r()}\nfor(let i=0;i<3;i++){continue}\n" +
		"switch(k){case 1:one();break;default:two()}\nx = a\n++b\nc\n/re/g.test(z)\nreturn\nvalue\n"
	want := "#!/usr/bin/env node\n// lead\nconst a='x', b=0xFF; /* inner */\nfunction f() {\n  return a+b\n}\n" +
		"let re=/a\\/b{2}/g, d=a/2/1, t=`x${a}y${ {k:1}.k }\n  kept`;\n" +
		"if(a) {\n  x()\n} else {\n  y()\n}\ntry {\n  q()\n} catch(e) {\n  r()\n}\nfor(let i=0; i<3; i++) {\n  continue\n}\n" +
		"switch(k) {\n  case 1:\n    one();\n    break;\n  default:\n    two()\n}\nx = a\n++b\nc\n/re/g.test(z)\nreturn\nvalue\n"
	got := formatWith(t, "app.js", source)
	if got != want {
		t.Fatalf("js format:\n%s", got)
	}
	if withoutSpace(got) != withoutSpace(source) {
		t.Fatal("js format changed tokens")
	}
}

func TestJavaScriptFormatRejectsJSXAndMalformedInput(t *testing.T) {
	family := Builtins().Lookup("app.mjs")
	if _, err := family.Format([]byte("const el = <div>hi there</div>")); !errors.Is(err, errJSXNotFormattable) {
		t.Fatalf("jsx format error = %v", err)
	}
	source := "const s = 'searchable-js"
	if _, err := family.Format([]byte(source)); err == nil {
		t.Fatal("malformed js format succeeded")
	}
}

func TestHTMLFormatIndentsBlocksAndKeepsVerbatimContent(t *testing.T) {
	source := "<!DOCTYPE html><html lang=en><head><meta charset=\"utf-8\"><title>Demo &amp; test</title>" +
		"<script>if(a<b){x('</p>')}</script></head><body class='x y'\n  hidden><!-- note --><div><h1>Hello <b>bold</b>world</h1>" +
		"<p>First\n      second <a title=\"multi\nline\">link</a>.<p>Next<ul><li>One<li>Two</ul>" +
		"<pre>  keep\n   <b>this</b></pre><textarea> t </textarea></div></body></html>"
	want := "<!DOCTYPE html>\n<html lang=en>\n  <head>\n    <meta charset=\"utf-8\">\n    <title>Demo &amp; test</title>\n" +
		"    <script>if(a<b){x('</p>')}</script>\n  </head>\n  <body class='x y' hidden>\n    <!-- note -->\n    <div>\n" +
		"      <h1>Hello <b>bold</b>world</h1>\n      <p>\n        First\n        second <a title=\"multi\nline\">link</a>.\n" +
		"      <p>Next\n      <ul>\n        <li>One\n        <li>Two\n      </ul>\n" +
		"      <pre>  keep\n   <b>this</b></pre>\n      <textarea> t </textarea>\n    </div>\n  </body>\n</html>\n"
	if got := formatWith(t, "index.html", source); got != want {
		t.Fatalf("html format:\n%s", got)
	}
}

func TestHTMLFormatLowercasesOnlyNames(t *testing.T) {
	got := formatWith(t, "page.htm", `<DIV ID="Keep-Case">Text <SPAN>Inline</SPAN></DIV>`)
	if got != "<div id=\"Keep-Case\">Text <span>Inline</span></div>\n" {
		t.Fatalf("html names: %s", got)
	}
}
