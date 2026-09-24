#!/usr/bin/env python3
"""Generate simple static HTML pages from go doc output."""

from __future__ import annotations

import argparse
import html
import os
import pathlib
import re
import shutil
import string
import subprocess
import tempfile


SECTION_HEADINGS = {
    "CONSTANTS",
    "FUNCTIONS",
    "TYPES",
    "VARIABLES",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    default_repo_dir = pathlib.Path(__file__).resolve().parents[1]
    parser.add_argument("--repo-dir", default=default_repo_dir, type=pathlib.Path)
    parser.add_argument("--output-dir", type=pathlib.Path)
    parser.add_argument("--go-binary", default=os.environ.get("GO_BINARY", "go"))
    graph = parser.add_mutually_exclusive_group()
    graph.add_argument("--dependency-graph", action="store_true")
    graph.add_argument("--dependency-graph-svg", type=pathlib.Path)
    return parser.parse_args()


def resolve_output_dir(repo_dir: pathlib.Path, output_dir: pathlib.Path | None) -> pathlib.Path:
    if output_dir is None:
        output_dir = repo_dir / "docs" / "go-api"
    output_dir = output_dir.resolve()
    if str(output_dir) in {"", "/"}:
        raise SystemExit(f"refusing to write Go docs to {output_dir}")
    return output_dir


def prepare_output_dir(output_dir: pathlib.Path) -> None:
    shutil.rmtree(output_dir, ignore_errors=True)
    output_dir.mkdir(parents=True, exist_ok=True)


def run_go(repo_dir: pathlib.Path, go_binary: str, *args: str) -> str:
    return subprocess.check_output([go_binary, *args], cwd=repo_dir, text=True)


def generate_dependency_graph(repo_dir: pathlib.Path, output_dir: pathlib.Path) -> pathlib.Path:
    with tempfile.TemporaryDirectory(prefix="rotari-dep-graph-", dir=output_dir.parent) as temp_dir:
        graph_dir = pathlib.Path(temp_dir)
        subprocess.check_call([repo_dir / "scripts" / "generate-dep-graph.sh", graph_dir], cwd=repo_dir)
        generated = graph_dir / "rotari-deps.svg"
        destination = output_dir / "rotari-deps.svg"
        shutil.copyfile(generated, destination)
    return destination


def load_template(template_dir: pathlib.Path, name: str) -> string.Template:
    return string.Template((template_dir / name).read_text(encoding="utf-8"))


def doc_target(module: str, package: str) -> str:
    relative = package.removeprefix(module + "/")
    if relative == package:
        return "."
    return "./" + relative


def package_filename(module: str, package: str) -> str:
    relative = package.removeprefix(module + "/")
    slug = relative.replace("/", "_") or "root"
    return f"{slug}.html"


def display_path(module: str, package: str) -> str:
    return package.removeprefix(module + "/")


def normalize_import_path(docs: str, package: str) -> str:
    return re.sub(
        r'^(package\s+\S+\s+// import )"\."',
        rf'\1"{package}"',
        docs,
        count=1,
    )


def render_doc_sections(docs: str) -> str:
    sections: list[tuple[str, list[str]]] = [("Overview", [])]
    for line in docs.splitlines():
        if line in SECTION_HEADINGS:
            sections.append((line.title(), []))
            continue
        sections[-1][1].append(line)
    return "\n".join(
        '<section class="doc-section">'
        f"<h2>{html.escape(title)}</h2>"
        f"<pre>{html.escape(chr(10).join(lines).strip())}</pre>"
        "</section>"
        for title, lines in sections
        if "\n".join(lines).strip()
    )


def render_package_page(
    repo_dir: pathlib.Path,
    output_dir: pathlib.Path,
    go_binary: str,
    module: str,
    package: str,
    package_template: string.Template,
) -> tuple[str, str]:
    relative = display_path(module, package)
    filename = package_filename(module, package)
    docs = run_go(repo_dir, go_binary, "doc", "-all", doc_target(module, package))
    docs = normalize_import_path(docs, package)
    rendered = package_template.substitute(
        title=html.escape(f"{relative} package"),
        package=html.escape(package),
        sections=render_doc_sections(docs),
    )
    (output_dir / filename).write_text(rendered, encoding="utf-8")
    return relative, filename


def render_index(
    output_dir: pathlib.Path,
    module: str,
    links: list[tuple[str, str]],
    index_template: string.Template,
    dependency_graph_svg: pathlib.Path | None,
) -> None:
    index_items = "\n".join(
        f'<li><a href="{html.escape(filename)}">{html.escape(relative)}</a></li>'
        for relative, filename in links
    )
    graph_section = render_dependency_graph(output_dir, dependency_graph_svg)
    index_page = index_template.substitute(
        module=html.escape(module),
        package_links=index_items,
        dependency_graph=graph_section,
    )
    (output_dir / "index.html").write_text(index_page, encoding="utf-8")


def render_dependency_graph(output_dir: pathlib.Path, source: pathlib.Path | None) -> str:
    if source is None:
        return ""
    source = source.resolve()
    if not source.is_file():
        raise SystemExit(f"dependency graph SVG not found: {source}")
    destination = output_dir / "rotari-deps.svg"
    if source != destination:
        shutil.copyfile(source, destination)
    return "\n".join(
        [
            '<section class="dependency-graph">',
            "<h2>Module dependency graph</h2>",
            '<a href="rotari-deps.svg"><img src="rotari-deps.svg" alt="Go module dependency graph"></a>',
            "</section>",
        ]
    )


def main() -> None:
    args = parse_args()
    repo_dir = args.repo_dir.resolve()
    output_dir = resolve_output_dir(repo_dir, args.output_dir)
    template_dir = repo_dir / "scripts" / "templates"

    prepare_output_dir(output_dir)

    module = run_go(repo_dir, args.go_binary, "list", "-m").strip()
    packages = [
        line
        for line in run_go(repo_dir, args.go_binary, "list", "./...").splitlines()
        if line
    ]
    index_template = load_template(template_dir, "go-docs-index.html")
    package_template = load_template(template_dir, "go-docs-package.html")
    dependency_graph_svg = args.dependency_graph_svg
    if args.dependency_graph:
        dependency_graph_svg = generate_dependency_graph(repo_dir, output_dir)

    links = [
        render_package_page(
            repo_dir,
            output_dir,
            args.go_binary,
            module,
            package,
            package_template,
        )
        for package in packages
    ]
    render_index(output_dir, module, links, index_template, dependency_graph_svg)
    print(f"generated Go docs in {output_dir}")


if __name__ == "__main__":
    main()