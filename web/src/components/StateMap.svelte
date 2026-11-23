<script lang="ts">
  import type { StateSummary } from '../lib/api';
  import type { StateShape } from '../lib/geo-types';
  import { delegationOf, fillFor, paint, shortLabel, spoken, type Mode } from '../lib/mapmode';

  // The map of the states (docs/web-conventions.md, The map). Each state is
  // a link named in words; Tab visits them alphabetically. States too small
  // to point at, and DC, get a call-out box, which is their tab stop; the
  // territories, which us-atlas does not draw, are boxes along the bottom.
  let { states, mode, labelledby }: { states: StateSummary[]; mode: Mode; labelledby: string } = $props();

  // The shapes are imported on demand, so only the map page loads them
  // (docs/web-conventions.md, Performance; make bundle-check).
  let shapeOf = $state<Map<string, StateShape> | null>(null);
  let failed = $state(false);
  $effect(() => {
    let live = true;
    import('../lib/geo/states')
      .then((m) => {
        if (live) shapeOf = new Map(m.states.map((s) => [s.code, s]));
      })
      .catch(() => {
        if (live) failed = true;
      });
    return () => {
      live = false;
    };
  });
  const calloutCodes = ['VT', 'NH', 'MA', 'RI', 'CT', 'NJ', 'DE', 'MD', 'DC'];
  const territoryCodes = ['PR', 'VI', 'GU', 'AS', 'MP'];
  const BOX_W = 84;
  const BOX_H = 36;
  const boxAt = (code: string): { x: number; y: number } | undefined => {
    const c = calloutCodes.indexOf(code);
    if (c >= 0) return { x: 980, y: 104 + c * 46 };
    const t = territoryCodes.indexOf(code);
    if (t >= 0) return { x: 320 + t * (BOX_W + 14), y: 630 };
    return undefined;
  };

  // Alphabetical, as the API sends them: the Tab order.
  const rows = $derived(
    states.map((s) => ({
      s,
      shape: shapeOf?.get(s.code),
      box: boxAt(s.code),
      fill: paint(fillFor(delegationOf(s, mode))),
      label: shortLabel(delegationOf(s, mode)),
      name: spoken(s, mode),
    })),
  );
</script>

{#if failed}
  <p class="muted">The map could not be loaded; the list below has every state.</p>
{:else if !shapeOf}
  <div class="placeholder" aria-busy="true"></div>
{:else}
<svg viewBox="0 0 1070 680" role="group" aria-labelledby={labelledby} class="map">
  <g class="shapes">
    {#each rows as r (r.s.code)}
      {#if r.shape && !r.box}
        <a href="/states/{r.s.code}" aria-label={r.name}>
          <path d={r.shape.d} style:fill={r.fill} />
        </a>
      {:else}
        {#if r.shape}
          <!-- Too small to point at: the box below is the tab stop. -->
          <a href="/states/{r.s.code}" tabindex="-1" aria-hidden="true">
            <path d={r.shape.d} style:fill={r.fill} />
          </a>
        {/if}
        {#if r.box}
          <a href="/states/{r.s.code}" aria-label={r.name} class="callout">
            <rect class="box" x={r.box.x} y={r.box.y} width={BOX_W} height={BOX_H} rx="4" />
            <rect x={r.box.x + 6} y={r.box.y + 7} width="22" height="22" rx="3" style:fill={r.fill} class="swatch" />
            <text x={r.box.x + 34} y={r.box.y + 16} class="code">{r.s.code}</text>
            <text x={r.box.x + 34} y={r.box.y + 29} class="count">{r.label || '—'}</text>
          </a>
        {/if}
      {/if}
    {/each}
  </g>
  <g class="labels" aria-hidden="true">
    {#each rows as r (r.s.code)}
      {#if r.shape && r.box}
        <line x1={r.shape.label[0]} y1={r.shape.label[1]} x2={r.box.x} y2={r.box.y + BOX_H / 2} />
      {:else if r.shape}
        <text x={r.shape.label[0]} y={r.shape.label[1]} class="code" text-anchor="middle">{r.s.code}</text>
        <text x={r.shape.label[0]} y={r.shape.label[1] + 11} class="count" text-anchor="middle">{r.label}</text>
      {/if}
    {/each}
  </g>
</svg>
{/if}

<style>
  .placeholder {
    aspect-ratio: 1070 / 680;
    background: var(--sunk);
    border-radius: var(--radius-m);
  }
  .map {
    display: block;
    inline-size: 100%;
    block-size: auto;
  }
  path {
    stroke: var(--bg);
    stroke-width: 0.8;
    stroke-linejoin: round;
  }
  a:hover path,
  a:hover .swatch {
    stroke: var(--fg);
    stroke-width: 1.5;
  }
  a:focus-visible path,
  a:focus-visible .box {
    stroke: var(--accent);
    stroke-width: 3;
  }
  .box {
    fill: var(--panel);
    stroke: var(--line);
  }
  .swatch {
    stroke: var(--line);
  }
  text {
    fill: var(--fg);
    pointer-events: none;
  }
  .labels text {
    paint-order: stroke;
    stroke: var(--bg);
    stroke-width: 3px;
    stroke-linejoin: round;
  }
  .code {
    font-size: 12px;
    font-weight: 760;
  }
  .count {
    font-size: 9.5px;
    font-variant-numeric: tabular-nums;
  }
  line {
    stroke: var(--muted);
    stroke-width: 0.6;
  }
</style>
