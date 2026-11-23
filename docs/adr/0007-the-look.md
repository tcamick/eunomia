# ADR 0007 — The look

**Date:** 2026-11-23 · **Status:** accepted · **Relates to:** PRD §3 (even-handedness), `docs/web-conventions.md`

## Context
Eunomia is read in short sessions to catch up on Congress, mostly on a phone or laptop, in light and dark. Its pages carry facts with sources, so the look should read like a well-set public record, not a campaign site or a news app. Party colour is the one place where the look touches the content: it must follow the convention readers already know (Democrats blue, Republicans red) without making either look better, and it must never be the only way to tell parties apart.

## Decision
- **One face, Public Sans** (`@fontsource-variable/public-sans`, self-hosted, latin subset by `unicode-range`). It was made for US government sites, reads well small, and has a variable weight axis, so headings carry weight rather than colour. Monospace is the system's, for job names and versions only.
- **Tokens, not values, in components.** `web/src/app.css` defines the role tokens (`--bg`, `--panel`, `--sunk`, `--fg`, `--muted`, `--line`, `--accent`, `--ok`, `--bad`, the radii) in `oklch`, once for light, and again for dark under `prefers-color-scheme` and `[data-theme='dark']`. The palette is map paper and indigo ink with a brass accent for links and the current section; nothing in the chrome is blue or red, so neither party's colour is the app's own.
- **Party colour is a token and a shape, always together** (`lib/party.ts`): Democratic `--party-dem` circle, Republican `--party-rep` square, Independent `--party-ind` diamond, any other party `--party-other` triangle, a vacant seat `--vacant` ring. The party's name is beside the mark or, where space forbids, in its accessible name. Text is always in ink, never in a party colour.
- **The party colours pass the `dataviz` palette checks in both themes**, against the page background, on all pairs (not only neighbours), checked with the skill's `validate_palette.js` on 2026-11-23:

  | | Democratic | Independent | Other | Republican | Worst CVD ΔE | Worst normal ΔE |
  |---|---|---|---|---|---|---|
  | light (on `#f2f7fb`) | `oklch(47% .15 262)` | `oklch(64% .11 60)` | `oklch(64% .11 330)` | `oklch(49% .17 27)` | 14.3 (protan) | 15.6 |
  | dark (on `#121724`) | `oklch(62% .13 262)` | `oklch(52% .11 60)` | `oklch(52% .11 330)` | `oklch(66% .15 27)` | 12.1 (protan) | 15.5 |

  Every colour has at least 3:1 contrast with the page; the vacant gray has 3.4:1 (light) and 4.2:1 (dark). Tritanopia separation is 5.1 for the closest pair, below the 8 target, which the validator allows only with a second encoding; the shapes are that encoding.
- **The signature element is the hemicycle** (`components/Hemicycle.svelte`): each chamber's seats in concentric half rings, seated Democrats, their caucus, others, the Republicans' caucus, Republicans, left to right. That order is the drawing convention used for Congress, not a ranking; the legend lists parties by size. Each filled seat links to its member for the pointer; keyboard and screen-reader users reach the same members through the lists below the chart, and the chart itself is one image with a text summary of the seats by party.
- **The shell** is a top bar with the four sections (Congress, Members, Committees, Search), a skip link, and Status in the footer, since it is for the owner's checks rather than for reading.

## Rejected
- **The first party palette:** Independent purple (`oklch(48% .13 315)`) and Other green (`oklch(47% .09 170)`). Purple sat 12.7 from Democratic blue for normal vision (floor 15) and 5.2 under protanopia, the green read as gray (chroma 0.09, floor 0.10), and the dark set was lighter than the dark band. Replaced by amber and plum, one hue per party in both themes.
- **Purple for Independents, as many charts use:** too close to Democratic blue for colour-blind readers at any lightness that also passes contrast.
- **Gray for parties other than the two largest:** gray is the vacant seat, and a party must not look like an empty chair.

## Consequences
- `--party-rep` and `--bad` are both red. Status uses `--bad` only with "✗ Failed" in words, and party marks always carry a shape and a name, so neither is read by colour alone. A future status colour must not be blue, amber or plum.
- Amber Independents sit near the brass accent. The accent is used for text links and the current-section underline only; seats and marks never use it.
- A new party token, or a change to one, reruns the palette checks in both themes and updates the table above.
- A user theme (the owner's choice of looks, if it ever comes) is another token block; components do not change.
