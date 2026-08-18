package keeper

import (
	_ "embed"
	"fmt"
	"hash/crc32"
	"strings"
)

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

// explorerCSSVersion/explorerJSVersion fingerprint the embedded content so
// browsers never serve a stale cached copy after a deploy.
//
// FIX (2026-07-21): handleExplorerCSS/handleExplorerJS set a 1-hour browser
// cache on plain, unversioned "/explorer.css"/"/explorer.js" URLs. A visitor
// whose browser had already cached either file within the last hour kept
// serving that stale copy for the rest of the hour — completely independent
// of how many times the server redeployed in between — while handleUI's
// always-fresh, no-cache HTML rendered immediately. Confirmed live: DAG-view
// and Consensus-tab changes landed in explorerHTML but stayed invisible to
// an already-open browser tab because the JS/CSS it had cached was still the
// pre-deploy version. explorerHTML now links to "/explorer.css?v=<hash>" and
// "/explorer.js?v=<hash>" (see explorerHTMLVersioned below) instead of the
// bare paths — a content change produces a different hash and therefore a
// different URL, which a browser has never cached, so the cache no longer
// needs to be short-lived to stay correct (see handleExplorerCSS/JS's own
// comment for the resulting long, safe cache lifetime).
var explorerCSSVersion = fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(explorerCSS)))
var explorerJSVersion = fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(explorerJS)))

// explorerHTMLVersioned is explorerHTML with its stylesheet/script tags
// pointed at content-hashed URLs — computed once at startup (not per
// request; the embedded content, and therefore the hashes, never change for
// the lifetime of a running binary). handleUI serves this instead of the
// raw explorerHTML.
var explorerHTMLVersioned = strings.NewReplacer(
	`href="/explorer.css"`, `href="/explorer.css?v=`+explorerCSSVersion+`"`,
	`src="/explorer.js"`, `src="/explorer.js?v=`+explorerJSVersion+`"`,
).Replace(explorerHTML)

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

// vendorWalletConnectJS replaces the WebAuthn "register via browser" flow
// (device-bound, so a person with two devices could register twice — see
// the AUDIT_2026-07-12 write-up) with WalletConnect on the Register and Swap
// pages: a proper wallet-signature connection instead of a browser
// credential standing in for one. Same self-hosting rationale as
// vendorEthersJS above — this is @walletconnect/ethereum-provider (see
// package.json), bundled to a single self-contained browser script with
// esbuild since the package only ships bundler-oriented builds, not a
// dependency-free vendor bundle.
//
//go:embed assets/vendor/walletconnect-ethereum-provider.min.js
var vendorWalletConnectJS string

// FIX (Monster Audit 2026-07-12 follow-up, P1): handleNodeBinding and
// handleLanding used to inline their <script> blocks directly into the HTML,
// which forced 'unsafe-inline' into script-src on both pages' CSP — a page
// that runs a compromised/injected script has full access to window.ethereum
// (handleNodeBinding signs a validator-binding message; handleLanding is the
// public landing page). Externalizing to same-origin files, same pattern as
// explorerJS above, lets both drop 'unsafe-inline' from script-src entirely.
//
//go:embed assets/node-binding.js
var nodeBindingJS string

//go:embed assets/landing.js
var landingJS string

// Brand assets. The favicon is an SVG so one file covers every size a browser
// asks for, and it draws the balance as paths rather than the ⚖ character —
// a glyph would render with whatever font the viewer happens to have, or not
// at all. apple-touch-icon.png exists because iOS ignores SVG icons when a
// page is added to the home screen.
//
//go:embed assets/favicon.svg
var faviconSVG string

//go:embed assets/apple-touch-icon.png
var appleTouchIcon []byte

// The card image link previews use. Until this existed, every link to the
// site posted on X or Telegram — the two channels the site itself points
// people at — rendered as a bare URL with no title, image or description.
//
//go:embed assets/og-image.png
var ogImagePNG []byte

// FIX (Monster Audit follow-up, 2026-07-12, P0/P1): aequitas-dapp.html (the
// mobile wallet SPA served by handleDapp) had NO CSP header at all — not
// even the permissive 'unsafe-inline' one the other HTML handlers used to
// carry — plus 21 onclick=/oninput= attributes, an inline <script> block,
// and CDN-loaded ethers/lightweight-charts (same supply-chain risk already
// fixed on the Explorer's register page). Externalized for the same reason
// nodeBindingJS/landingJS were: lets handleDapp set a real script-src 'self'
// CSP with zero exceptions.
//
//go:embed assets/dapp.js
var dappJS string
