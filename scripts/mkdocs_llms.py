"""Generate LLM-friendly text files from rendered MkDocs pages."""

from __future__ import annotations

from html.parser import HTMLParser
from pathlib import Path
import re
from urllib.parse import urljoin

_PAGES: list[tuple[str, str, str]] = []


class _ContentText(HTMLParser):
    """Convert rendered page content to readable Markdown-like text."""

    def __init__(self, page_url: str, site_url: str) -> None:
        super().__init__(convert_charrefs=True)
        self.page_url = page_url
        self.site_url = site_url
        self.parts: list[str] = []
        self.links: list[tuple[str, int]] = []
        self.in_pre = False
        self.ignored = 0

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if self.ignored:
            self.ignored += 1
            return
        if tag in {"script", "style", "svg"}:
            self.ignored = 1
            return

        self._start_element(tag, dict(attrs))

    def _start_element(self, tag: str, attributes: dict[str, str | None]) -> None:
        if self._start_heading(tag):
            return
        if self._start_block(tag, attributes):
            return

        if tag == "code":
            self._start_code(attributes)
        elif tag == "a":
            self._start_link(attributes)
        elif tag == "img":
            alt = attributes.get("alt")
            if alt:
                self.parts.append(alt)

    def _start_code(self, attributes: dict[str, str | None]) -> None:
        if not self.in_pre:
            self.parts.append("`")
            return
        language = next(
            (
                name.removeprefix("language-")
                for name in attributes.get("class", "").split()
                if name.startswith("language-")
            ),
            "",
        )
        if language and self.parts and self.parts[-1] == "```\n":
            self.parts[-1] = f"```{language}\n"

    def _start_link(self, attributes: dict[str, str | None]) -> None:
        href = attributes.get("href")
        if href:
            self.links.append((href, len(self.parts)))

    def _start_heading(self, tag: str) -> bool:
        if tag in {"h1", "h2", "h3", "h4", "h5", "h6"}:
            self._line_break()
            self.parts.append("#" * int(tag[1]) + " ")
            return True
        return False

    def _start_block(self, tag: str, attributes: dict[str, str | None]) -> bool:
        if tag in {"p", "div", "blockquote", "ul", "ol", "table"}:
            self._line_break()
        elif tag == "li":
            self._line_break()
            self.parts.append("- ")
        elif tag == "pre":
            self._line_break()
            language = next(
                (
                    name.removeprefix("language-")
                    for name in attributes.get("class", "").split()
                    if name.startswith("language-")
                ),
                "",
            )
            self.parts.append(f"```{language}\n")
            self.in_pre = True
        elif tag == "br":
            self.parts.append("\n")
        elif tag == "hr":
            self._line_break()
            self.parts.append("---\n")
        elif tag in {"td", "th"}:
            if self.parts and not self.parts[-1].endswith((" ", "\n")):
                self.parts.append(" | ")
        else:
            return False
        return True

    def handle_endtag(self, tag: str) -> None:
        if self.ignored:
            self.ignored -= 1
            return
        if tag in {"h1", "h2", "h3", "h4", "h5", "h6", "p", "div", "li", "tr"}:
            self._line_break()
        elif tag == "pre":
            self._line_break()
            self.parts.append("```\n")
            self.in_pre = False
        elif tag == "code" and not self.in_pre:
            self.parts.append("`")
        elif tag == "a" and self.links:
            href, start = self.links.pop()
            label = "".join(self.parts[start:]).strip()
            target = urljoin(urljoin(self.site_url, self.page_url), href)
            if target != label:
                self.parts.append(f" ({target})")
        elif tag in {"td", "th"}:
            self.parts.append(" | ")

    def handle_data(self, data: str) -> None:
        if not self.ignored:
            self.parts.append(data)

    def _line_break(self) -> None:
        if self.parts and not self.parts[-1].endswith("\n"):
            self.parts.append("\n")

    def text(self) -> str:
        self.close()
        normalized: list[str] = []
        in_fence = False
        for line in "".join(self.parts).splitlines():
            stripped = line.strip()
            if stripped.startswith("```"):
                in_fence = not in_fence
                normalized.append(stripped)
            elif in_fence:
                normalized.append(line.rstrip())
            elif stripped:
                normalized.append(re.sub(r"\s+", " ", stripped))
            elif normalized and normalized[-1]:
                normalized.append("")
        return "\n".join(normalized).strip()


def _absolute_url(site_url: str, page_url: str) -> str:
    return urljoin(site_url.rstrip("/") + "/", page_url.lstrip("/"))


def _write_outputs(
    pages: list[tuple[str, str, str]], site_dir: Path, site_url: str
) -> None:
    base_url = site_url.rstrip("/") + "/"
    ordered_pages = sorted(pages, key=lambda page: (page[1] != "", page[1].casefold()))
    index = [
        "# rotari",
        "",
        "> Documentation for rotari, an execution manager for experiment batches.",
        "",
        "## Documentation",
        "",
    ]
    full = [
        "# rotari documentation (full)",
        "",
        "> Generated from the rendered MkDocs pages.",
        "",
    ]

    for title, page_url, content in ordered_pages:
        page_link = _absolute_url(base_url, page_url)
        index.append(f"- [{title}]({page_link})")
        rendered_text = _ContentText(page_url, base_url)
        rendered_text.feed(content)
        full.extend(
            [
                f"## {title}",
                "",
                f"Source: {page_link}",
                "",
                rendered_text.text(),
                "",
                "---",
                "",
            ]
        )

    site_dir.mkdir(parents=True, exist_ok=True)
    (site_dir / "llms.txt").write_text("\n".join(index).rstrip() + "\n", encoding="utf-8")
    (site_dir / "llms-full.txt").write_text(
        "\n".join(full).rstrip() + "\n", encoding="utf-8"
    )


def on_pre_build(**kwargs) -> None:
    """Reset collected pages before each MkDocs build."""
    _PAGES.clear()


def on_post_page(output, page, **kwargs):
    """Collect rendered Markdown content for each generated page."""
    title = page.title or page.file.src_uri
    _PAGES.append((title, page.url, page.content))
    return output


def on_post_build(config, **kwargs) -> None:
    """Write the LLM files into the configured MkDocs site directory."""
    _write_outputs(_PAGES, Path(config["site_dir"]), config["site_url"])