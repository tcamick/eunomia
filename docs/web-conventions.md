# Web conventions for `web/`

The frontend counterpart to `docs/go-conventions.md`. Adapted on 2026-11-22 from Plutus's `docs/web-conventions.md`, which was distilled from Google Chrome's [Modern Web Guidance](https://github.com/GoogleChrome/modern-web-guidance) (Apache-2.0), pinned at commit `4dd9683`. Removed: the rules that existed because the page showed private positions (nothing private in browser storage, the session cookie, logout clearing), money arithmetic, and Plutus's system of looks. Added: rules for the map and for showing text that strangers wrote. When a rule here conflicts with what you would do by default, follow the rule.

## Scope and browser support
- **Stack:** Svelte 5 (runes) + TypeScript, Vite. Self-hosted fonts through `@fontsource`. No CSS framework, no component library, no mapping library at runtime, no charting library until a chart needs one.
- **Targets:** a phone browser for Home and the feed, a laptop browser for the map and the Congress pages (PRD §4). Current Chrome, Firefox and Safari, desktop and mobile. Every page works at 360 CSS px wide.
- **Browser support policy:** a feature that is *Baseline Widely available* needs no fallback. A *Baseline Newly available* feature is allowed with a feature check (`@supports`, `'x' in el`) and graceful degradation, the fallback under about 20 lines. **No polyfills, and no new runtime dependency to cover a platform feature.**
- **Platform before code.** Before writing JavaScript for a UI behaviour, check whether HTML or CSS already does it: `<dialog>`, `popover`, `<details>`, form constraints, `:has()`, container queries, `@starting-style`, view transitions. Before adding an npm package, check the platform. A change to `package.json` is a dependency change and the owner's decision (`docs/development-workflow.md`, Stop and ask).

## Security (the page shows text strangers wrote)
- **Outside text is text.** Headlines, summaries, names, bill titles and vote questions go through normal Svelte interpolation. `{@html}` is used only for HTML the Go server rendered from this repository's own `learn/` markdown (goldmark, raw HTML escaped). Never `{@html}` on anything that came from a feed or an API, and never HTML built from strings in TypeScript.
- **Same origin only.** The server sends a `Content-Security-Policy` that keeps every fetch on this origin (`default-src 'self'`; images `'self' data:`; no inline script; inline style attributes allowed, because Svelte's `style:` sets them). No CDN scripts or styles, no web fonts from another origin, no analytics, no third-party embeds, no remote images. Portraits are mirrored by a job and served from this origin. If a feature needs another origin, the owner decides, and the CSP changes in the same commit with a test.
- **Companion headers** (server, `internal/httpapi`): `X-Content-Type-Options: nosniff` with a correct `Content-Type` on every response; `Referrer-Policy: no-referrer`, so a publisher does not learn the app's tailnet name; `frame-ancestors 'none'` with `X-Frame-Options: DENY`; `Cross-Origin-Opener-Policy: same-origin`; `Cross-Origin-Resource-Policy: same-origin`; a `Permissions-Policy` that turns off camera, microphone and geolocation.
- **External links** use `rel="noopener noreferrer"`. A URL from data is rendered as an `href` only if the server marked it `https`; anything else is shown as plain text.
- **Browser storage** holds per-device preferences only (a theme, a collapsed panel, a chosen colour mode), wrapped in `try/catch`. What the owner has read or saved lives on the server, so it follows them between phone and laptop. The service worker caches the app shell only, never `/api/*`.

## Semantics and accessibility
- **Structure:** one `<main>`; one `<h1>` per page (the page title), headings in order with no skipped levels; landmarks `<header>`, `<nav aria-label>`, `<main>`, `<aside>`. Each route sets `document.title` to `"<Page> · Eunomia"`.
- **Native elements:** `<button>` for actions and `<a href>` for navigation. Never `onclick` on a `<div>` or `<span>`. Every `<button>` outside a form has `type="button"`. `<details>`/`<summary>` for disclosure. A native `<dialog>` with `showModal()` for anything modal, with a visible close button; no hand-made focus trap.
- **Focus:** style `:focus-visible`, not `:focus`. Never `outline: none` without an equally visible replacement; keep a real `outline` so the ring survives `forced-colors`. When content opens from a user action, move focus into it; when it closes, return focus to the control that opened it.
- **Targets:** interactive elements are at least 24×24 CSS px, rising to 44px under `@media (pointer: coarse)`. Size with padding or `min-block-size`, not a fixed height.
- **Names:** every control has a visible `<label for>` or, for an icon-only button, an `aria-label`. A placeholder is never the label. `title=` is never the only way to reach information. Decorative SVGs get `aria-hidden="true"`.
- **Tables:** `<table>` with `<caption>` (may be visually hidden), `<th scope="col">`, and `<th scope="row">` for the person or state. Sticky headers when a table scrolls. Container queries for narrow widths.
- **Live updates:** one polite `aria-live` region per page for results of the owner's actions (saved, marked read, error). `role="alert"` only for failures that block. Do not announce "Loading…"; use `<progress>` or `aria-busy`.

## The map, seat charts and party colour
- **The map is inline SVG** from shapes generated at build time (`scripts/`, committed output). Each state is an `<a href>` inside the SVG with an accessible name that carries what the colour shows ("Nevada: governor Republican, senators 2 Democratic"). It is reachable by Tab in a sensible order (alphabetical, not drawing order) and has a visible focus ring.
- **Every map has a list twin** on the same page with the same data: the accessible equivalent, and the control on a phone, where states like Rhode Island cannot be tapped. Small eastern states also get the usual call-out boxes beside the map.
- **Party is never shown by colour alone.** A letter, a word or a pattern goes with it: in the legend, in every table cell, in the accessible name. A split or vacant seat has its own pattern, not a blend.
- **Party colours are tokens** (`--party-dem`, `--party-rep`, `--party-ind`, `--party-other`, `--vacant`), defined in `oklch`, checked for contrast against both themes and for colour-blind separation with the dataviz palette checks. The same token is used on the map, the seat chart, a person's page and a card. Components never name a colour.
- **Seat charts and other charts** are hand-written SVG with a text alternative: an `aria-label` or `<figcaption>` that states the totals, and the numbers as a table where they matter.
- **Portraits** have `width` and `height`, `loading="lazy"` below the fold, and `alt` set to the person's name. A missing portrait shows initials, not a broken image or a stranger's face.

## CSS, type and layout
- **Respect text scaling.** Do not set the root font size. Size text in `rem` or `em`, unitless `line-height`, no fixed heights on containers that hold text. Form controls use `font-size: max(1rem, 1em)` so iOS Safari does not zoom on focus. Never turn off pinch zoom.
- **Layout:** flexbox for one axis and grid for two (`repeat(auto-fit, minmax(min(100%, 16rem), 1fr))` for grids of people). Container queries for components; media queries for page-level breakpoints only. Logical properties (`margin-inline`, `padding-block`). `env(safe-area-inset-*)` for edges when installed to a home screen.
- **Tokens and theme:** components read role tokens only (`--bg`, `--panel`, `--fg`, `--muted`, `--line`, `--accent`, the party tokens, the radii, the type faces), never a colour, radius or face of their own. Light is the default block on `:root`; dark is one block under `@media (prefers-color-scheme: dark)` and the same values under `[data-theme='dark']` for a manual choice. New colours in `oklch`. Per-element state reaches CSS as a custom property through `style:--x`.
- **Selectors:** `:has()` instead of mirroring child state in a class; `:is()` and `:where()` instead of duplicated rules; `:user-invalid`, never bare `:invalid`. No global reset beyond `box-sizing`.
- **Motion:** animate only `opacity` and `transform`. Non-essential motion sits inside `@media (prefers-reduced-motion: no-preference)`, and an element whose animation is off still ends in its final state. A card flip (PRD §6.9) has a reduced-motion form that swaps faces without turning. Nothing flashes.

## Forms and input
- Native constraints first (`required`, `minlength`, `maxlength`, `pattern`, `type`). Validate on blur or submit, clear an error on `input`, link the error with `aria-describedby` and `aria-invalid`.
- The search box is a real `<form role="search">` with a labelled `<input type="search">`; results are links; it works without JavaScript's help for Enter and Esc.
- An Enter-to-send input checks `!e.isComposing` so input-method composition is not cut off.

## Performance
- No main-thread work over 50 ms in an event handler. Split longer work with `scheduler.yield()` where available and a `setTimeout(0)` fallback.
- The map's shapes load with the pages that draw them, by dynamic import, not in the bundle of Home or the feed.
- Fonts are self-hosted with `font-display: swap`; only the subsets and weights in use.
- `content-visibility: auto` with `contain-intrinsic-size` for long lists (the feed, a roll call's 435 positions, the roster).

## Dates and numbers
- **An election day, a term's start and a vote's date are calendar dates, not instants.** Never call `new Date('YYYY-MM-DD')` and format it in local time: west of UTC the date shifts back a day, and an election appears on the wrong day. Keep date parsing and formatting in `lib/format.ts`, built with `timeZone: 'UTC'` on `Intl.DateTimeFormat`, until `Temporal.PlainDate` is Baseline.
- A story's publication time is an instant and is shown in the viewer's local time, as a relative time ("3 hours ago") with the full date in a `<time datetime>`.
- Counts and percentages through `Intl.NumberFormat`. The server computes rates (party-line voting, margins); the browser does not redo the arithmetic.

## Consulting the source guides
Each rule above is a summary. When you build or change a specific pattern, read the matching guide first, pinned and read-only:

```bash
curl -fsSL https://raw.githubusercontent.com/GoogleChrome/modern-web-guidance/4dd968349e54ca45a05dd5c818fb035c9054b022/skills/modern-web-guidance/guides/<area>/<id>.md
```

| Area | Guide IDs |
|---|---|
| security | `security`, `sanitize-untrusted-html`, `restrict-outbound-connections` |
| accessibility | `accessibility`, `respect-os-text-scale`, `accessible-error-announcement` |
| ui-behaviors | `light-dismiss-a-dialog`, `platform-controls-dismiss-dialog`, `animate-to-from-top-layer`, `motion` |
| ui-atoms | `responsive-table`, `icons` |
| css | `css`, `css-layout`, `responsive-design` |
| forms | `forms`, `validate-input-after-interaction`, `ime-safe-enter-submit` |
| html | `html` |
| performance | `performance`, `break-up-long-tasks` |
| pwa | `install-web-app` |
| js | `capture-location-agnostic-data` (dates) |

Do not run `npx modern-web-guidance@latest` from an agent session: it downloads and executes an unpinned package on each call.

## Design
The first phase that draws a page (E1a) sets the look: the type faces, the tokens, the page shell and the navigation, with the `frontend-design` skill, and records the choices in an ADR. Later phases use those tokens and do not restyle. Charts and the party palette are checked with the `dataviz` skill's palette checks.

## Definition of done (frontend)
`make web-docker` is clean: tests, `svelte-check` and the build. Every changed page has been opened and looked at (`docs/development-workflow.md` §2): at 360 px and at laptop width, in light and dark, with the keyboard alone (Tab, Shift+Tab, Enter, Space, Esc) and focus always visible, at 200% zoom with nothing clipped, and with `prefers-reduced-motion` on. Checks that need the owner's own phone are recorded as manual in the acceptance file.
