# Phase E1b — The map and state pages acceptance

Branch: `feat/phase-e1b-map` · Started: 2026-11-23 · Merged:

Roadmap `docs/roadmap.md`, E1b; plan `docs/implementation-plan.md` §4, E1b; PRD §3, §6.2, §7, §9.

## Done-when checklist
- [ ] Clicking any state, or choosing it from the list, opens its page with both senators and every House member, with photos and parties — evidence:
- [ ] Each colour mode's map agrees with the list beside it for every state — evidence:
- [ ] Every state can be reached and opened with the keyboard alone, and by touch at 360 px through the list — evidence:
- [ ] A state's district map loads only on that state's page (the bundle check shows it), and is labelled with its Congress — evidence:
- [ ] A vacant seat is shown as vacant — evidence:

## Sources (checked 2026-11-23)
- **District shapes:** the Census Bureau's cartographic boundary file `cb_2024_us_cd119_500k.zip` (`https://www2.census.gov/geo/tiger/GENZ2024/shp/`), 7,045,981 bytes, SHA-256 `2b4238ae…b031a` (pinned in the Makefile). 441 districts: 435 seats, the six at-large states coded `00`, DC and five territories coded `98`. Terms, from its metadata: "free to use in a product or publication, however acknowledgement must be given to the U.S. Census Bureau as the source"; display at 1:500,000 or smaller; no access constraints. Fetched once by `make geo`, not at run time.
- **State shapes:** `us-atlas` 3.0.1 (npm, ISC licence), `states-albers-10m.json`, derived from Census boundaries; pinned by the lockfile and named, with its SHA-256, in the generated files' headers.
- **Build-time packages:** the owner's go-ahead, 2026-11-23, for `us-atlas`, `shapefile`, `d3-geo`, `topojson-server`, `topojson-simplify`, `topojson-client`, pinned exactly in `scripts/geo/`.

## Open questions found during the phase
- PRD §6.2 asks for a vacant seat's "reason and the date of any special election". The roster source does not give either for a vacancy. Until a source that does is added, the page shows the seat as vacant, and, where the stored roster has it, who last held it and when the source stopped listing them.

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
