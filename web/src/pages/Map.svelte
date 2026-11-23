<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import MapDefs from '../components/MapDefs.svelte';
  import Sources from '../components/Sources.svelte';
  import StateMap from '../components/StateMap.svelte';
  import Swatch from '../components/Swatch.svelte';
  import type { StatesResponse } from '../lib/api';
  import { Load } from '../lib/load.svelte';
  import { delegationOf, fillFor, legend, modes, shortLabel, spoken, type Mode } from '../lib/mapmode';
  import type { PageProps } from '../lib/page';

  let { query }: Partial<PageProps> = $props();

  document.title = 'Map · Eunomia';
  const load = new Load<StatesResponse>();
  load.get('/api/states');
  onDestroy(() => load.stop());

  let mode = $state<Mode>(untrack(() => (query?.get('mode') === 'house' ? 'house' : 'senate')));
  const modeLabel = $derived(modes.find((m) => m.id === mode)?.label ?? '');
  const entries = $derived(load.data ? legend(load.data.states, mode) : []);

  // The mode is in the address, so a view can be bookmarked; changing it is
  // not a navigation, so focus stays on the control.
  function choose(m: Mode): void {
    mode = m;
    const url = new URL(location.href);
    if (m === 'senate') url.searchParams.delete('mode');
    else url.searchParams.set('mode', m);
    history.replaceState(history.state, '', url.pathname + url.search);
  }
</script>

<h1 tabindex="-1">The map</h1>
<p class="lede muted">Who each state sends to Congress. Select a state, or a name in the list, to see its members.</p>

{#if load.error}
  <p role="alert">Could not load the states: {load.error}.</p>
{:else if !load.data}
  <p aria-busy="true">Loading the map…</p>
{:else}
  {@const data = load.data}
  <MapDefs />
  <div class="controls">
    <fieldset>
      <legend>Colour by</legend>
      {#each modes as m (m.id)}
        <label>
          <input type="radio" name="mode" value={m.id} checked={mode === m.id} onchange={() => choose(m.id)} />
          {m.label}
        </label>
      {/each}
    </fieldset>
    <ul class="key" aria-label="Key">
      {#each entries as e (e.key)}
        <li><Swatch fill={e.fill} /> {e.label}</li>
      {/each}
    </ul>
  </div>

  <figure class="figure">
    <StateMap states={data.states} {mode} labelledby="map-caption" />
    <figcaption id="map-caption" class="muted small">
      The states, DC and the territories, coloured by their {modeLabel}: the party with the most seats, stripes where two
      tie. Letters give the seats: D Democratic, R Republican, I Independent, O another party.
    </figcaption>
  </figure>

  <section aria-labelledby="h-list">
    <h2 id="h-list">Every state</h2>
    <div class="table-wrap">
      <table>
        <caption class="visually-hidden">Each state's delegation in both chambers</caption>
        <thead>
          <tr>
            <th scope="col">State</th>
            <th scope="col">Senate</th>
            <th scope="col">House</th>
          </tr>
        </thead>
        <tbody>
          {#each data.states as s (s.code)}
            <tr>
              <th scope="row"><a href="/states/{s.code}">{s.name}</a></th>
              {#each ['senate', 'house'] as const as m (m)}
                {@const d = delegationOf(s, m)}
                <td class:current={m === mode}>
                  {#if d.seats === 0}
                    <span class="muted">none</span>
                  {:else}
                    <span class="num" aria-hidden="true"><Swatch fill={fillFor(d)} /> {shortLabel(d)}</span><span
                      class="visually-hidden"
                      >{spoken(s, m)}</span
                    >{#if m === 'house' && !s.voting}{' '}<span class="muted" aria-hidden="true">(non-voting)</span>{/if}
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <Sources sources={data.sources} />
{/if}

<style>
  .lede {
    margin-block: 0.5rem 1.5rem;
  }
  .controls {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem 2.5rem;
    align-items: start;
    margin-block-end: 1rem;
  }
  fieldset {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1.25rem;
    margin: 0;
    padding: 0;
    border: none;
  }
  legend {
    font-weight: 600;
    font-size: 0.875rem;
    margin-block-end: 0.25rem;
    padding: 0;
  }
  fieldset label {
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
    min-block-size: 2rem;
  }
  input[type='radio'] {
    inline-size: 1.1rem;
    block-size: 1.1rem;
    accent-color: var(--accent);
  }
  @media (pointer: coarse) {
    fieldset label {
      min-block-size: 2.75rem;
    }
  }
  .key {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1.25rem;
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 0.9375rem;
    align-self: end;
  }
  .key li {
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
  }
  .figure {
    margin: 0 0 2.5rem;
  }
  figcaption {
    margin-block-start: 0.5rem;
    max-inline-size: var(--measure);
  }
  .small {
    font-size: 0.875rem;
  }
  h2 {
    margin-block-end: 0.75rem;
  }
  .table-wrap {
    overflow-x: auto;
    max-inline-size: 44rem;
  }
  table {
    inline-size: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: start;
    padding: 0.375rem 0.75rem 0.375rem 0;
    border-block-end: 1px solid var(--line);
    vertical-align: top;
  }
  td .num {
    white-space: nowrap;
  }
  thead th {
    color: var(--muted);
    font-weight: 600;
  }
  tbody th {
    font-weight: 400;
    white-space: normal;
  }
  td.current {
    font-weight: 650;
  }
</style>
