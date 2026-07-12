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

// FIX (Monster Audit 2026-07-12, P1): the register/wallet page used to load
// ethers and the price-chart library straight from cdnjs.cloudflare.com /
// unpkg.com — a compromised CDN build of either would run with full access
// to this page's wallet-connect and signing flow. Self-hosting exact,
// version-pinned copies (same versions the CDN tags named: ethers 6.13.0,
// lightweight-charts 4.1.3) removes that third-party supply-chain risk
// entirely; the files themselves come from this repo's own npm
// dependencies (package.json), not hand-downloaded.
//
//go:embed assets/vendor/ethers.min.js
var vendorEthersJS string

//go:embed assets/vendor/lightweight-charts.min.js
var vendorLightweightChartsJS string
