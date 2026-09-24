#!/usr/bin/env python3
"""Generate a PEP 503 "simple" package index for rotari wheels.

Wheels are not stored anywhere separately: this script reads the assets of
every GitHub release and links directly to their `browser_download_url`, so
the generated index always reflects exactly what has been released.
"""

from __future__ import annotations

import argparse
import html
import json
import os
import pathlib
import re
import urllib.error
import urllib.request


PROJECT_NAME = "rotari"
GITHUB_API_ROOT = "https://api.github.com"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="GitHub 'owner/name' slug")
    parser.add_argument("--output-dir", required=True, type=pathlib.Path)
    return parser.parse_args()


def github_get(url: str) -> list:
    request = urllib.request.Request(url)
    request.add_header("Accept", "application/vnd.github+json")
    request.add_header("User-Agent", "rotari-pypi-index-generator")
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(request) as response:
        return json.load(response)


def fetch_wheel_assets(repo: str) -> list[dict]:
    assets = []
    page = 1
    while True:
        url = f"{GITHUB_API_ROOT}/repos/{repo}/releases?per_page=100&page={page}"
        try:
            releases = github_get(url)
        except urllib.error.HTTPError as exc:
            raise SystemExit(f"failed to list releases for {repo}: {exc}") from exc
        if not releases:
            break
        for release in releases:
            for asset in release.get("assets", []):
                if asset["name"].endswith(".whl"):
                    assets.append(asset)
        page += 1
    return assets


def wheel_href(asset: dict) -> str:
    url = html.escape(asset["browser_download_url"])
    digest = asset.get("digest") or ""
    match = re.fullmatch(r"sha256:([0-9a-f]{64})", digest)
    if match:
        return f"{url}#sha256={match.group(1)}"
    return url


def write_index(output_dir: pathlib.Path, assets: list[dict]) -> None:
    simple_dir = output_dir / "simple"
    project_dir = simple_dir / PROJECT_NAME
    project_dir.mkdir(parents=True, exist_ok=True)

    simple_dir.joinpath("index.html").write_text(
        "<!DOCTYPE html>\n"
        "<html>\n"
        '  <head><meta name="pypi:repository-version" content="1.0"></head>\n'
        "  <body>\n"
        f'    <a href="{PROJECT_NAME}/">{PROJECT_NAME}</a>\n'
        "  </body>\n"
        "</html>\n",
        encoding="utf-8",
    )

    links = "\n".join(
        f'    <a href="{wheel_href(asset)}">{html.escape(asset["name"])}</a><br>'
        for asset in sorted(assets, key=lambda asset: asset["name"])
    )
    project_dir.joinpath("index.html").write_text(
        "<!DOCTYPE html>\n"
        "<html>\n"
        '  <head><meta name="pypi:repository-version" content="1.0"></head>\n'
        "  <body>\n"
        f"{links}\n"
        "  </body>\n"
        "</html>\n",
        encoding="utf-8",
    )


def main() -> None:
    args = parse_args()
    assets = fetch_wheel_assets(args.repo)
    write_index(args.output_dir, assets)


if __name__ == "__main__":
    main()
