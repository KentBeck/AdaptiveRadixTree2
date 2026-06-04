#!/usr/bin/env python3
"""Concatenate the per-chapter manuscript markdown into one book.

Outputs (gitignored):
  tutorial/_book/adaptive-radix-tree.md
  tutorial/_book/adaptive-radix-tree.html

Usage:
  python3 tutorial/build_book.py

Re-run any time after editing the manuscript. The output directory is
in .gitignore so the build artifact never gets committed.

Requires the `markdown` Python package (pip install markdown).
"""
import os
import re
import sys

import markdown

HERE = os.path.dirname(os.path.abspath(__file__))
OUT_DIR = os.path.join(HERE, "_book")
MANIFEST = os.path.join(HERE, "book.yml")


def read(path):
    with open(os.path.join(HERE, path), encoding="utf-8") as f:
        return f.read()


def load_manifest():
    manifest = {"parts": []}
    current_part = None
    with open(MANIFEST, encoding="utf-8") as f:
        for lineno, raw in enumerate(f, 1):
            line = raw.rstrip("\n")
            stripped = line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            if stripped == "parts:":
                continue
            if line.startswith("  - "):
                current_part = {}
                manifest["parts"].append(current_part)
                key, value = parse_key_value(stripped[2:], lineno)
                current_part[key] = value
                continue
            if line.startswith("    "):
                if current_part is None:
                    raise ValueError(f"{MANIFEST}:{lineno}: part field before part")
                key, value = parse_key_value(stripped, lineno)
                current_part[key] = value
                continue
            key, value = parse_key_value(stripped, lineno)
            manifest[key] = value

    for key in ("title", "subtitle", "output_base"):
        if not manifest.get(key):
            raise ValueError(f"{MANIFEST}: missing {key}")
    if not manifest["parts"]:
        raise ValueError(f"{MANIFEST}: missing parts")
    for part in manifest["parts"]:
        if not part.get("path") or not part.get("title"):
            raise ValueError(f"{MANIFEST}: every part needs path and title")
    return manifest


def parse_key_value(text, lineno):
    if ":" not in text:
        raise ValueError(f"{MANIFEST}:{lineno}: expected key: value")
    key, value = text.split(":", 1)
    return key.strip(), value.strip()


def assemble_md(manifest):
    out = []
    out.append(f"# {manifest['title']}\n")
    out.append(f"*{manifest['subtitle']}*\n")
    out.append("\n## Table of contents\n")
    for part in manifest["parts"]:
        title = part["title"]
        anchor = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")
        out.append(f"- [{title}](#{anchor})")
    out.append("\n")

    for part in manifest["parts"]:
        path = part["path"]
        title = part["title"]
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
<title>__TITLE__</title>
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


def assemble_html(manifest, md_text):
    md = markdown.Markdown(
        extensions=["fenced_code", "tables", "toc"],
        output_format="html5",
    )
    body = md.convert(md_text)
    return HTML_TEMPLATE.replace("__TITLE__", manifest["title"]).replace("__BODY__", body)


def main():
    manifest = load_manifest()
    os.makedirs(OUT_DIR, exist_ok=True)
    md_text = assemble_md(manifest)
    md_path = os.path.join(OUT_DIR, f"{manifest['output_base']}.md")
    with open(md_path, "w", encoding="utf-8") as f:
        f.write(md_text)
    print(f"wrote {md_path}  ({len(md_text):,} bytes, {md_text.count(chr(10)):,} lines)")

    html_text = assemble_html(manifest, md_text)
    html_path = os.path.join(OUT_DIR, f"{manifest['output_base']}.html")
    with open(html_path, "w", encoding="utf-8") as f:
        f.write(html_text)
    print(f"wrote {html_path} ({len(html_text):,} bytes)")


if __name__ == "__main__":
    sys.exit(main())
