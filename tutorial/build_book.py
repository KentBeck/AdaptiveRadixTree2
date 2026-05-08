#!/usr/bin/env python3
"""Concatenate the per-chapter tutorial markdown into one book.

Outputs (gitignored):
  tutorial/_book/art-tutorial.md
  tutorial/_book/art-tutorial.html

Usage:
  python3 tutorial/build_book.py

Re-run any time after editing the tutorial. The output directory is
in .gitignore so the build artefact never gets committed.

Requires the `markdown` Python package (pip install markdown).
"""
import os
import re
import sys

import markdown

HERE = os.path.dirname(os.path.abspath(__file__))
OUT_DIR = os.path.join(HERE, "_book")

# Reading order. The README is the front matter; chapter dirs are
# numbered; BENCHMARKS.md is the appendix.
PARTS = [
    ("README.md", "Front matter"),
    ("00-what-is-a-trie/tutorial.md", "Chapter 0 — What's a trie?"),
    # ("01-test-harness/tutorial.md", "Chapter 1 — Test harness"),  # prose pending
    ("02-node256-only/tutorial.md", "Chapter 2 — A node256-only tree"),
    ("03-lazy-expansion/tutorial.md", "Chapter 3 — Lazy expansion"),
    ("04-path-compression/tutorial.md", "Chapter 4 — Path compression"),
    ("05-add-node4/tutorial.md", "Chapter 5 — Adding node4"),
    ("06-introduce-polymorphism/tutorial.md", "Chapter 6 — Introduce polymorphism"),
    ("07-add-node16/tutorial.md", "Chapter 7 — Adding node16"),
    ("08-add-node48/tutorial.md", "Chapter 8 — Adding node48"),
    ("09-polish/tutorial.md", "Chapter 9 — Polish + reading guide"),
    ("BENCHMARKS.md", "Appendix — Scaling annex"),
]


def read(path):
    with open(os.path.join(HERE, path), encoding="utf-8") as f:
        return f.read()


def assemble_md():
    out = []
    out.append("# Adaptive Radix Tree — the literate tutorial\n")
    out.append("*Single-file build assembled from the per-chapter markdown.*\n")
    out.append("\n## Table of contents\n")
    for path, title in PARTS:
        anchor = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")
        out.append(f"- [{title}](#{anchor})")
    out.append("\n")

    for path, title in PARTS:
        out.append("\n\n---\n")
        out.append(f"\n# {title}\n\n")
        out.append(f"*Source: `tutorial/{path}`*\n\n")
        # Demote the chapter's headings by one level so the file's H1
        # becomes an H2 under our chapter title; otherwise we'd have
        # two competing H1s and the TOC anchors would collide.
        body = read(path)
        body = re.sub(r"^(#{1,5}) ", lambda m: "#" + m.group(1) + " ", body, flags=re.M)
        out.append(body)
        out.append("\n")
    return "".join(out)


HTML_TEMPLATE = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Adaptive Radix Tree — the literate tutorial</title>
<style>
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI",
                 Helvetica, Arial, sans-serif;
    max-width: 820px;
    margin: 2em auto;
    padding: 0 1em 4em;
    line-height: 1.55;
    color: #111;
    background: #fafafa;
  }
  h1, h2, h3, h4 { line-height: 1.25; }
  h1 { font-size: 2.0em; border-bottom: 2px solid #333; padding-bottom: 0.2em; }
  h2 { font-size: 1.45em; border-bottom: 1px solid #ccc; padding-bottom: 0.15em; margin-top: 1.8em; }
  h3 { font-size: 1.15em; margin-top: 1.5em; }
  hr { border: none; border-top: 2px dashed #aaa; margin: 3em 0; }
  code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    background: #eee;
    padding: 1px 4px;
    border-radius: 3px;
    font-size: 0.92em;
  }
  pre {
    background: #1f1f1f;
    color: #f0f0f0;
    padding: 0.9em 1em;
    overflow-x: auto;
    border-radius: 5px;
    font-size: 0.88em;
  }
  pre code { background: none; padding: 0; color: inherit; font-size: 1em; }
  table { border-collapse: collapse; margin: 1em 0; font-size: 0.92em; }
  th, td { border: 1px solid #ccc; padding: 5px 9px; text-align: left; }
  th { background: #f0f0f0; }
  td:nth-child(n+2) { text-align: right; }
  blockquote {
    border-left: 4px solid #888;
    padding-left: 1em;
    color: #444;
    background: #f4f4f4;
    margin: 1em 0;
  }
  a { color: #1a55a3; }
  em { color: #444; }
  ul li a { text-decoration: none; }
  ul li a:hover { text-decoration: underline; }
</style>
</head>
<body>
__BODY__
</body>
</html>
"""


def assemble_html(md_text):
    md = markdown.Markdown(
        extensions=["fenced_code", "tables", "toc"],
        output_format="html5",
    )
    body = md.convert(md_text)
    return HTML_TEMPLATE.replace("__BODY__", body)


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    md_text = assemble_md()
    md_path = os.path.join(OUT_DIR, "art-tutorial.md")
    with open(md_path, "w", encoding="utf-8") as f:
        f.write(md_text)
    print(f"wrote {md_path}  ({len(md_text):,} bytes, {md_text.count(chr(10)):,} lines)")

    html_text = assemble_html(md_text)
    html_path = os.path.join(OUT_DIR, "art-tutorial.html")
    with open(html_path, "w", encoding="utf-8") as f:
        f.write(html_text)
    print(f"wrote {html_path} ({len(html_text):,} bytes)")


if __name__ == "__main__":
    sys.exit(main())
