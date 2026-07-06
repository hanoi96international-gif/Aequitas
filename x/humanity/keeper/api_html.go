package keeper

import _ "embed"

// The Explorer UI used to live entirely inside this file as one ~6800-line
// Go raw string literal (explorerHTML). That made the file unwieldy to
// navigate, gave editors no HTML/CSS/JS syntax highlighting, and made a
// stray backtick anywhere in the content (including inside a comment)
// silently break Go compilation by prematurely terminating the string —
// hit twice during the 2026-07-05 beta-launch audit work.
//
// Split into three real, appropriately-typed static files under assets/,
// embedded verbatim (byte-for-byte diffed against the original content
// before this split — see git history) and served as-is: explorer.html
// references the other two via <link rel="stylesheet"> / <script src=">
// instead of inlining them.
//
//go:embed assets/explorer.html
var explorerHTML string

//go:embed assets/explorer.css
var explorerCSS string

//go:embed assets/explorer.js
var explorerJS string
