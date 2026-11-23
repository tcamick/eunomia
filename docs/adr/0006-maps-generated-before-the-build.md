# ADR 0006 — Maps generated before the build

**Date:** 2026-11-23 · **Status:** accepted (owner, 2026-11-23: go-ahead for the build-time packages) · **Relates to:** PRD §6.2, §7, §9; `docs/web-conventions.md`, The map

## Context
The map page draws the 50 states and DC; each state's page draws its congressional districts. The shapes change rarely: state lines almost never, district lines after a redistricting. The page is read on a laptop and a phone, and the CSP keeps every request on Eunomia's own origin.

## Decision
- **Shapes are generated before the build and committed,** as TypeScript modules of SVG path data under `web/src/lib/geo/`, by `scripts/geo/generate.mjs`, run by `make geo` as the Dockerfile's `geo` stage (Node 22 in the same pinned image as the web build): the inputs are copied into the image, the packages are installed with their install scripts off, nothing from the host is mounted (AGENTS.md, Operating on neptune), and only the output is copied back. The browser gets plain `<path>` data; it loads no mapping library and no tiles.
- **Inputs, pinned:**
  - the states: `us-atlas` 3.0.1's `states-albers-10m.json`, Census boundaries already projected with `d3.geoAlbersUsa` into 975 × 610 (Alaska and Hawaii as insets). It has the 50 states and DC; the territories are drawn as labelled boxes;
  - the districts: the Census Bureau's cartographic boundary file `cb_2024_us_cd119_500k` (the 119th Congress), downloaded by `make geo` and refused unless its SHA-256 matches the Makefile's.
  The script's six packages (`us-atlas`, `shapefile`, `d3-geo`, `topojson-server`, `topojson-simplify`, `topojson-client`) are pinned exactly in `scripts/geo/package.json` and its lockfile; they never reach the browser.
- **One module per state's districts,** imported dynamically, so a state's district map loads only on its page. Each state gets its own equal-area conic projection, centred on its longitude and fitted to a frame 600 units wide, so Rhode Island keeps as much detail as Texas, and Alaska's Aleutians stay in one piece across the antimeridian.
- **Simplified as a topology,** after projection, dropping points whose triangle is under 0.6 square units: neighbouring districts keep one shared border, so no gaps or slivers open between them. Coordinates are rounded to a tenth of a unit.
- **Tests hold the shapes to the facts:** a Go test (`internal/geo/shapes_test.go`) checks the generated manifest against the 2020 apportionment in both directions; a web test checks each module against the manifest.
- **The Census Bureau is credited** wherever its boundaries are drawn, as its terms ask ("acknowledgement must be given to the U.S. Census Bureau as the source"), and each district map says whose lines it draws: "Districts of the 119th Congress".

## Rejected
- **A mapping library in the browser** (d3-geo with TopoJSON at run time, or Leaflet and tiles): more code and more requests for shapes that never change, and tiles would come from another origin.
- **One file of every district:** about 1 MB; every state page would load all of it.
- **The `us-atlas` projection for district maps:** small states would be a few units wide, and the inset Alaska distorted.
- **Simplifying each district on its own:** neighbours simplify differently, leaving gaps along their shared borders.

## Consequences
- When the Census Bureau publishes the 120th Congress's districts (roadmap, gate G2), `make geo` is rerun with the new file's URL and checksum, and the label changes; the tests then fail until the manifest's Congress and the apportionment agree.
- The Census file is a display product, meant for 1:500,000 or smaller; the maps are drawn at page size, well within that.
- Dense urban districts are hard to tell apart on a state's map at page size (Los Angeles, New York City). The list beside the map is the way to them; a zoomed inset is not planned.
