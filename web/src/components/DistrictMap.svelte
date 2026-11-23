<script lang="ts">
  import Shape from './Shape.svelte';
  import type { Member } from '../lib/api';
  import type { DistrictMap } from '../lib/geo-types';
  import { ordinal } from '../lib/format';
  import { partyLabel, partyStyle, vacantStyle } from '../lib/party';

  // A state's congressional districts (ADR 0006). Its shapes are one
  // module per state, imported here on demand, so they load only on that
  // state's page. Each district is filled with its member's party colour,
  // and carries the party's shape as a marker, so party is never colour
  // alone; a vacant district is hatched with the vacant ring.
  let { code, house, congress }: { code: string; house: Member[]; congress: number } = $props();

  const modules = import.meta.glob<{ default: DistrictMap }>('../lib/geo/districts/*.ts');

  let map = $state<DistrictMap | null>(null);
  let failed = $state(false);
  $effect(() => {
    const load = modules[`../lib/geo/districts/${code}.ts`];
    map = null;
    failed = false;
    if (!load) {
      failed = true;
      return;
    }
    let live = true;
    load()
      .then((m) => {
        if (live) map = m.default;
      })
      .catch(() => {
        if (live) failed = true;
      });
    return () => {
      live = false;
    };
  });

  const byDistrict = $derived(new Map(house.map((m) => [m.district ?? 0, m])));
  const dense = $derived((map?.districts.length ?? 0) > 20);
  const name = (n: number) => (n === 0 ? 'At large' : `${ordinal(n)} district`);
</script>

<figure class="districts">
  {#if failed}
    <p class="muted">The district map could not be loaded; the list below has every seat.</p>
  {:else if !map}
    <div class="placeholder" aria-busy="true"></div>
  {:else}
    <svg viewBox={map.viewBox} role="group" aria-labelledby="districts-caption">
      {#each map.districts as d (d.n)}
        {@const m = byDistrict.get(d.n)}
        {@const st = m ? partyStyle(m.partyCode) : vacantStyle}
        {#if m}
          <a href="/people/{m.id}" aria-label="{name(d.n)}: {m.name}, {partyLabel(m.party ?? '', m.caucus)}">
            <path d={d.d} style:fill="var({st.token})" />
          </a>
        {:else}
          <g aria-label="{name(d.n)}: vacant" role="img">
            <path d={d.d} style:fill="url(#vacant)" />
          </g>
        {/if}
      {/each}
      <g class="marks" aria-hidden="true">
        {#each map.districts as d (d.n)}
          {@const m = byDistrict.get(d.n)}
          {@const st = m ? partyStyle(m.partyCode) : vacantStyle}
          <Shape shape={st.shape} cx={d.label[0]} cy={d.label[1]} r={dense ? 3.5 : 6} />
          {#if !dense}
            <text x={d.label[0]} y={d.label[1] - 10} text-anchor="middle">{d.n || 'AL'}</text>
          {/if}
        {/each}
      </g>
    </svg>
  {/if}
  <figcaption id="districts-caption">
    Districts of the {ordinal(congress)} Congress{#if map && dense}; select a district, or use the list{/if}.
  </figcaption>
</figure>

<style>
  .districts {
    margin: 0;
  }
  svg {
    display: block;
    inline-size: 100%;
    block-size: auto;
    max-block-size: 70vh;
  }
  .placeholder {
    aspect-ratio: 4 / 3;
    background: var(--sunk);
    border-radius: var(--radius-m);
  }
  path {
    stroke: var(--bg);
    stroke-width: 1;
    stroke-linejoin: round;
  }
  a:hover path {
    stroke: var(--fg);
    stroke-width: 2;
  }
  a:focus-visible path {
    stroke: var(--accent);
    stroke-width: 3.5;
  }
  .marks {
    pointer-events: none;
  }
  /* Party markers are solid; the vacant ring and the party-unknown box are
     hollow, so no hollow shape can be read as a party's. */
  .marks :global(circle),
  .marks :global(rect),
  .marks :global(polygon) {
    fill: var(--fg);
    stroke: var(--bg);
    stroke-width: 1.2;
  }
  .marks :global([fill='none']) {
    fill: none;
    stroke: var(--fg);
    stroke-width: 2;
  }
  text {
    font-size: 12px;
    font-weight: 700;
    fill: var(--fg);
    paint-order: stroke;
    stroke: var(--bg);
    stroke-width: 3px;
  }
  figcaption {
    margin-block-start: 0.5rem;
    color: var(--muted);
    font-size: 0.875rem;
  }
</style>
