# Web UI fonts

IBM Plex Sans (Regular, Medium, SemiBold) and IBM Plex Mono (Regular, Medium),
the Latin-1 split WOFF2 files exactly as IBM publishes them in the npm packages
`@ibm/plex-sans` 1.1.0 and `@ibm/plex-mono` 2.5.0 (`fonts/split/woff2/`).
They are licensed under the SIL Open Font License 1.1 with the Reserved Font
Name "Plex"; see [LICENSE.txt](LICENSE.txt). The files are not modified or
subset further, so they keep their names.

`internal/webui/fonts.go` embeds them, serves them under `/fonts/`, and copies
them into the static export; characters outside Latin-1 fall back to the
system font.
