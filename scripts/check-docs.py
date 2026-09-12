#!/usr/bin/env python3
"""Structural checks for every markdown file in the repo.

Why this exists: the docs advertise install commands, link across four
deliverables and are read on GitHub, where a broken relative link or a
mismatched anchor is invisible until a stranger hits it. Each check below
corresponds to a defect that actually shipped here at least once.

Checks:
  links    — every relative link resolves to a file on disk
  anchors  — every in-page `#anchor` matches a heading, using GitHub's
             own slug algorithm (lowercase, strip punctuation, spaces to
             hyphens); GitHub silently scrolls nowhere when it misses
  hr       — no `---` thematic breaks: they are parsed as YAML frontmatter
             delimiters by some renderers (macOS Quick Look) and break the
             page outright
  wrap     — no hard-wrapped prose; paragraphs must be one physical line so
             the renderer controls wrapping

Exit code is 1 if any check fails, so it can gate CI.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent

# Directories that are not hand-authored docs: dependencies, build output,
# generated SDKs and vendored code. Findings there are noise.
SKIP_DIRS = {
    "node_modules",
    "vendor",
    "dist",
    ".git",
    "sdk",
    "gen",
    ".vite",
    "build",
}

# A hard-wrapped paragraph is a prose line that ends mid-sentence. Lines
# under this length that end a paragraph are almost always intentional
# (headings, short list items), so only flag long runs.
WRAP_MIN_LEN = 200


def is_skipped(path: Path) -> bool:
    return any(part in SKIP_DIRS for part in path.parts)


def github_slug(heading: str) -> str:
    """Reproduce GitHub's heading -> anchor transform.

    Lowercase, drop anything that is not a word char / space / hyphen,
    then spaces to hyphens. Emoji and punctuation vanish; this is the
    step people get wrong when hand-writing a table of contents.
    """
    text = heading.strip().lower()
    text = re.sub(r"[^\w\s-]", "", text)
    # Each space becomes its own hyphen — GitHub does NOT collapse runs.
    # "Quick Start — Go" loses the em dash, leaving two spaces, and so
    # anchors as `quick-start--go`. Collapsing here would report every
    # correct em-dash anchor in the repo as broken.
    return text.replace(" ", "-").strip("-")


def markdown_files() -> list[Path]:
    return sorted(p for p in REPO.rglob("*.md") if not is_skipped(p.relative_to(REPO)))


def strip_frontmatter(lines: list[str]) -> list[str]:
    """Blank out a leading YAML frontmatter block.

    GitHub issue templates REQUIRE `---` delimiters at the top of the
    file. Those are structure, not thematic breaks, and flagging them
    would make the check unfixable.
    """
    if not lines or lines[0].strip() != "---":
        return lines
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            return [""] * (i + 1) + lines[i + 1 :]
    return lines


def strip_code_fences(lines: list[str]) -> list[str]:
    """Blank out fenced code so commands and YAML are not linted as prose."""
    out, in_fence = [], False
    for line in lines:
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            out.append("")
            continue
        out.append("" if in_fence else line)
    return out


LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*)$")
# An ordered-list item is its own line by definition; a long one followed
# by the next item is a list, not a wrapped paragraph.
ORDERED_ITEM_RE = re.compile(r"^\s*\d+[.)]\s")


def check_file(path: Path) -> list[str]:
    rel = path.relative_to(REPO)
    raw = path.read_text(encoding="utf-8").splitlines()
    prose = strip_code_fences(strip_frontmatter(raw))
    problems: list[str] = []

    headings = [HEADING_RE.match(ln) for ln in prose]
    slugs = {github_slug(m.group(2)) for m in headings if m}

    for num, line in enumerate(prose, 1):
        if line.strip() in {"---", "***", "___"}:
            problems.append(f"{rel}:{num}: thematic break (`---`) — use a heading instead")

        for target in LINK_RE.findall(line):
            target = target.strip()
            if target.startswith(("http://", "https://", "mailto:", "#")):
                if target.startswith("#") and github_slug(target[1:]) not in slugs:
                    problems.append(f"{rel}:{num}: anchor {target} matches no heading")
                continue
            file_part = target.split("#", 1)[0]
            if not file_part:
                continue
            resolved = (path.parent / file_part).resolve()
            if not resolved.exists():
                problems.append(f"{rel}:{num}: link target does not exist: {target}")

    # Hard-wrap: a long prose line immediately followed by more prose means
    # the author broke a paragraph across physical lines.
    for num in range(len(prose) - 1):
        cur, nxt = prose[num].rstrip(), prose[num + 1].strip()
        if (
            len(cur) >= WRAP_MIN_LEN
            and not cur.startswith(("#", ">", "|", "-", "*", " ", "\t"))
            and not ORDERED_ITEM_RE.match(cur)
            and nxt
            and not nxt.startswith(("#", ">", "|", "-", "*", "[", "!"))
            and not ORDERED_ITEM_RE.match(nxt)
        ):
            problems.append(f"{rel}:{num + 1}: hard-wrapped prose — one paragraph per line")

    return problems


def main() -> int:
    files = markdown_files()
    problems: list[str] = []
    for path in files:
        problems.extend(check_file(path))

    print(f"checked {len(files)} markdown files")
    for p in problems:
        print(p)
    if problems:
        print(f"\nFAIL: {len(problems)} problem(s)")
        return 1
    print("OK: links resolve, anchors match, no thematic breaks, no hard-wrapped prose")
    return 0


if __name__ == "__main__":
    sys.exit(main())
