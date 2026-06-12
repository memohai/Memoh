# File icon assets

The file explorer's type icons are the **Seti** icon set, vendored from the
`theme-seti` extension of the VS Code repository.

- `seti.woff` — the icon font.
- `vs-seti-icon-theme.json` — the file-association → glyph + color mapping
  (`fileNames`, `fileExtensions`, `languageIds`, and `light` overrides).

## Provenance

Generated from [jesseweed/seti-ui](https://github.com/jesseweed/seti-ui):

- Icon glyphs: `styles/_fonts/seti.less`
- Icon colors: `styles/ui-variables.less`
- File associations: `styles/components/icons/mapping.less`

Bundled via Microsoft's `microsoft/vscode` `extensions/theme-seti/icons/`.

## License

Seti UI is distributed under the **MIT License** (© Jesse Weed). The
`theme-seti` packaging in `microsoft/vscode` is likewise MIT (© Microsoft).
Both permit redistribution and commercial use with attribution, satisfied by
this file. The two assets above are vendored verbatim; update them by copying
the upstream files again rather than hand-editing.
