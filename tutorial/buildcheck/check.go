// Package buildcheck implements the tutorial's "self-healing" prose
// build step. Each chapter's build_test.go invokes Check, which:
//
//  1. Reads tutorial.md.
//  2. Walks every fenced Go code block whose info string carries a
//     {src=PATH ...} attribute. Two modes:
//     - {src=PATH decls=A,B,C} (or decl=A): extract those top-level
//       declarations from PATH via go/parser+go/printer and treat
//       them as the canonical content of the block. Mismatch fails
//       the test; -update-prose rewrites the prose block from source.
//     - {src=PATH} (no decls): verify-only fragment. The block's Go
//       tokens (from go/scanner, comments and semicolons stripped)
//       must appear as a contiguous subsequence of PATH's tokens, in
//       blank-line-separated groups so two snippets in one block can
//       sit at different points in the source. Cannot self-heal.
//  3. Walks every region of the form
//     <!-- bench:Name:start --> ... <!-- bench:Name:end -->
//     and verifies / regenerates it from the matching Region.Render
//     in the chapter's renderer list.
//
// Without -update-prose the test fails on any drift. With it, the
// file is rewritten in place. Both code blocks and bench regions
// flow through the same pass so a single -update-prose run heals
// the document end-to-end.
package buildcheck

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// UpdateProse, when set via -update-prose on the test command line,
// causes Check to rewrite tutorial.md in place rather than fail.
var UpdateProse = flag.Bool("update-prose", false, "rewrite tutorial.md code blocks and bench regions in place")

// Region is a bench-driven section of tutorial.md. Render is invoked
// at test time; the resulting text replaces (or is verified against)
// the body between <!-- bench:Name:start --> and <!-- bench:Name:end -->.
type Region struct {
	Name   string
	Render func() string
}

// Check verifies (and, with -update-prose, rewrites) tutorial.md at
// mdPath. The chapter directory is mdPath's parent; {src=PATH} attrs
// are resolved against that directory.
func Check(t *testing.T, mdPath string, regions []Region) {
	t.Helper()
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	body = checkCodeBlocks(t, mdPath, body)
	body = checkRegions(t, mdPath, body, regions)
	if *UpdateProse && body != string(raw) {
		if err := os.WriteFile(mdPath, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: rewrote prose", mdPath)
	}
}

// ---- code blocks ---------------------------------------------------------

var (
	codeFenceRE = regexp.MustCompile("^(```+)go\\b(.*)$")
	attrRE      = regexp.MustCompile(`(\w+)=([^\s}]+)`)
)

func checkCodeBlocks(t *testing.T, mdPath, body string) string {
	t.Helper()
	chDir := filepath.Dir(mdPath)
	lines := strings.Split(body, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		m := codeFenceRE.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, lines[i])
			i++
			continue
		}
		fence := m[1]
		attrs := parseAttrs(m[2])
		// Find the closing fence of the same length.
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], fence) {
			j++
		}
		if j >= len(lines) {
			out = append(out, lines[i])
			i++
			continue
		}
		openIdx := i
		closeIdx := j
		bodyLines := lines[openIdx+1 : closeIdx]
		blockBody := strings.Join(bodyLines, "\n")

		newBody := blockBody
		if src := attrs["src"]; src != "" {
			srcRel := src
			srcPath := filepath.Join(chDir, srcRel)
			declList := attrs["decls"]
			if declList == "" {
				declList = attrs["decl"]
			}
			switch {
			case declList != "":
				extracted, err := extractDecls(srcPath, splitNames(declList))
				if err != nil {
					t.Errorf("%s:%d: %v", mdPath, openIdx+1, err)
				} else if extracted != blockBody {
					if !*UpdateProse {
						t.Errorf("%s:%d: code block stale vs %s decls=%s (rerun with -update-prose to refresh)\n--- want ---\n%s\n--- got ---\n%s",
							mdPath, openIdx+1, srcRel, declList, extracted, blockBody)
					}
					newBody = extracted
				}
			default:
				// Fragment: verify-only.
				srcRaw, err := os.ReadFile(srcPath)
				if err != nil {
					t.Errorf("%s:%d: cannot read %s: %v", mdPath, openIdx+1, srcPath, err)
				} else {
					srcToks := tokenize(srcRaw)
					for _, group := range splitOnBlankLines(blockBody) {
						proseToks := tokenize([]byte(group))
						if len(proseToks) == 0 {
							continue
						}
						if !containsSubseq(srcToks, proseToks) {
							t.Errorf("%s:%d: fragment does not appear in %s\n--- snippet ---\n%s",
								mdPath, openIdx+1, srcRel, group)
						}
					}
				}
			}
		}

		out = append(out, lines[openIdx])
		out = append(out, strings.Split(newBody, "\n")...)
		out = append(out, lines[closeIdx])
		i = closeIdx + 1
	}
	return strings.Join(out, "\n")
}

func parseAttrs(infoTail string) map[string]string {
	attrs := map[string]string{}
	for _, m := range attrRE.FindAllStringSubmatch(infoTail, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

func splitNames(csv string) []string {
	parts := strings.Split(csv, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// extractDecls returns the bytes from path corresponding to the named
// top-level declarations, in the order requested, separated by blank
// lines. The exact source bytes (including any leading doc comment)
// are returned so the prose matches the file byte-for-byte.
func extractDecls(path string, names []string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse %s: %v", path, err)
	}
	index := map[string]ast.Decl{}
	for _, d := range file.Decls {
		switch dd := d.(type) {
		case *ast.FuncDecl:
			if dd.Recv != nil {
				if rt := receiverType(dd.Recv); rt != "" {
					index[rt+"."+dd.Name.Name] = d
				}
			}
			// Also register the bare method name so the prose can
			// say decl=Put rather than decl=Tree.Put. Last-writer
			// wins on collision, but this codebase has none.
			index[dd.Name.Name] = d
		case *ast.GenDecl:
			for _, spec := range dd.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					index[s.Name.Name] = d
				case *ast.ValueSpec:
					for _, n := range s.Names {
						index[n.Name] = d
					}
				}
			}
		}
	}
	var parts []string
	var missing []string
	for _, n := range names {
		d, ok := index[n]
		if !ok {
			missing = append(missing, n)
			continue
		}
		// Skip any leading doc comment: source comments are
		// written for source readers, the prose for tutorial
		// readers. The two audiences want different framing.
		startOff := fset.Position(d.Pos()).Offset
		endOff := fset.Position(d.End()).Offset
		parts = append(parts, string(bytes.TrimRight(src[startOff:endOff], " \t")))
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("decl(s) not found in %s: %v", path, missing)
	}
	return strings.Join(parts, "\n\n"), nil
}

func receiverType(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	for {
		switch v := t.(type) {
		case *ast.StarExpr:
			t = v.X
		case *ast.IndexExpr:
			t = v.X
		case *ast.IndexListExpr:
			t = v.X
		case *ast.Ident:
			return v.Name
		default:
			return ""
		}
	}
}

// ---- token-subsequence verification -------------------------------------

type tokInfo struct {
	tok token.Token
	lit string
}

func tokenize(src []byte) []tokInfo {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	s.Init(file, src, nil, 0)
	var out []tokInfo
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON || tok == token.ILLEGAL || tok == token.COMMENT {
			continue
		}
		out = append(out, tokInfo{tok: tok, lit: lit})
	}
	return out
}

func containsSubseq(haystack, needle []tokInfo) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j].tok != needle[j].tok || haystack[i+j].lit != needle[j].lit {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func splitOnBlankLines(s string) []string {
	var groups []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		groups = append(groups, strings.Join(cur, "\n"))
		cur = nil
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return groups
}

// ---- bench regions ------------------------------------------------------

func checkRegions(t *testing.T, mdPath, body string, regions []Region) string {
	t.Helper()
	for _, r := range regions {
		startMarker := fmt.Sprintf("<!-- bench:%s:start -->", r.Name)
		endMarker := fmt.Sprintf("<!-- bench:%s:end -->", r.Name)
		si := strings.Index(body, startMarker)
		ei := strings.Index(body, endMarker)
		if si < 0 || ei < 0 {
			t.Errorf("%s: missing markers for region %q", mdPath, r.Name)
			continue
		}
		if si > ei {
			t.Errorf("%s: markers reversed for region %q", mdPath, r.Name)
			continue
		}
		want := "\n" + r.Render() + "\n"
		got := body[si+len(startMarker) : ei]
		if want == got {
			continue
		}
		if !*UpdateProse {
			t.Errorf("%s: region %q is stale (rerun with -update-prose to refresh)\n--- want ---\n%s\n--- got ---\n%s",
				mdPath, r.Name, want, got)
			continue
		}
		body = body[:si+len(startMarker)] + want + body[ei:]
	}
	return body
}
