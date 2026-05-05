package addnode4

// build_test.go is the chapter-4 prototype of the tutorial build
// step. It does three things, all driven by `go test ./04-add-node4/`:
//
//  1. TestProseMatchesCode verifies that every Go code block in
//     tutorial.md tagged with `{src=PATH}` appears as a contiguous
//     token subsequence of PATH (Go file, this chapter or another).
//     Blank-line-separated groups inside one block are matched
//     independently, so a block can quote two struct decls that sit
//     apart in the source.
//
//  2. TestProseRegions verifies HTML-comment-delimited regions in
//     tutorial.md against measurements taken at test time
//     (unsafe.Sizeof of each inner-node type). Run with
//     `-update-prose` to rewrite a stale region; without the flag a
//     mismatch fails the test.
//
//  3. The chapter's existing tests run alongside, so "art.go passes
//     the tests" comes for free.
//
// Bench numbers are not yet wired in -- this prototype demonstrates
// the marker shape on the simplest measurement (per-type sizes).
// Adding put/get/all/range tables follows the same pattern.

import (
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

var updateProse = flag.Bool("update-prose", false, "rewrite generated regions in tutorial.md")

// ---- (1) prose-vs-code check -----------------------------------------------

type proseBlock struct {
	line    int    // 1-based line of the opening fence
	srcPath string // resolved path relative to the chapter dir
	body    string
}

var (
	codeFenceRE = regexp.MustCompile("^```go\\b(.*)$")
	srcAttrRE   = regexp.MustCompile(`src=([^\s}]+)`)
)

func TestProseMatchesCode(t *testing.T) {
	md, err := os.ReadFile("tutorial.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := extractTaggedGoBlocks(string(md))
	if len(blocks) == 0 {
		t.Fatal("no {src=...}-tagged Go blocks found in tutorial.md")
	}
	for _, blk := range blocks {
		src, err := os.ReadFile(blk.srcPath)
		if err != nil {
			t.Errorf("tutorial.md:%d: cannot read %s: %v", blk.line, blk.srcPath, err)
			continue
		}
		srcToks := tokenize(src)
		for _, group := range splitOnBlankLines(blk.body) {
			proseToks := tokenize([]byte(group))
			if len(proseToks) == 0 {
				continue
			}
			if !containsSubseq(srcToks, proseToks) {
				t.Errorf("tutorial.md:%d: code block does not appear in %s\n--- prose snippet ---\n%s",
					blk.line, blk.srcPath, group)
			}
		}
	}
}

func extractTaggedGoBlocks(md string) []proseBlock {
	var out []proseBlock
	lines := strings.Split(md, "\n")
	i := 0
	for i < len(lines) {
		m := codeFenceRE.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}
		open := i
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
			j++
		}
		attr := srcAttrRE.FindStringSubmatch(m[1])
		if attr != nil {
			out = append(out, proseBlock{
				line:    open + 1,
				srcPath: filepath.Clean(attr[1]),
				body:    strings.Join(lines[open+1:j], "\n"),
			})
		}
		i = j + 1
	}
	return out
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

type tokInfo struct {
	tok token.Token
	lit string
}

func tokenize(src []byte) []tokInfo {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	s.Init(file, src, nil, 0) // mode 0: skip comments, keep auto-semis
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

// ---- (2) bench-driven region check -----------------------------------------

func TestProseRegions(t *testing.T) {
	for _, region := range proseRegions() {
		enforceRegion(t, "tutorial.md", region.name, region.render())
	}
}

type proseRegion struct {
	name   string
	render func() string
}

func proseRegions() []proseRegion {
	return []proseRegion{
		{name: "nodesizes", render: renderNodeSizesTable},
	}
}

func renderNodeSizesTable() string {
	n4 := int(unsafe.Sizeof(node4[int]{}))
	n256 := int(unsafe.Sizeof(node256[int]{}))
	lf := int(unsafe.Sizeof(leaf[int]{}))
	return fmt.Sprintf("```\n"+
		"Type      Bytes    What it holds\n"+
		"node4   %7d    prefix slice, 4 sorted keys, 4 child slots, terminal, count\n"+
		"node256 %7d    prefix slice, 256 child slots, terminal, count\n"+
		"leaf    %7d    key slice header, value (V == int)\n"+
		"ratio   %6dx    node256 / node4\n"+
		"```", n4, n256, lf, n256/n4)
}

func enforceRegion(t *testing.T, path, name, expected string) {
	t.Helper()
	startMarker := fmt.Sprintf("<!-- bench:%s:start -->", name)
	endMarker := fmt.Sprintf("<!-- bench:%s:end -->", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	si := strings.Index(body, startMarker)
	ei := strings.Index(body, endMarker)
	if si < 0 || ei < 0 {
		t.Errorf("%s: missing markers %q / %q", path, startMarker, endMarker)
		return
	}
	if si > ei {
		t.Errorf("%s: markers reversed for region %q", path, name)
		return
	}
	currentInner := body[si+len(startMarker) : ei]
	wantInner := "\n" + expected + "\n"
	if currentInner == wantInner {
		return
	}
	if !*updateProse {
		t.Errorf("%s: region %q is stale (rerun with -update-prose to refresh)\n--- want ---\n%s\n--- got ---\n%s",
			path, name, wantInner, currentInner)
		return
	}
	newBody := body[:si+len(startMarker)] + wantInner + body[ei:]
	if err := os.WriteFile(path, []byte(newBody), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: rewrote region %q", path, name)
}
