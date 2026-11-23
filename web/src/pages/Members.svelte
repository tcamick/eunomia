<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import Portrait from '../components/Portrait.svelte';
  import Sources from '../components/Sources.svelte';
  import type { MembersResponse } from '../lib/api';
  import { calendarDate, seatName } from '../lib/format';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';

  let { query }: Partial<PageProps> = $props();

  document.title = 'Members · Eunomia';
  const load = new Load<MembersResponse>();
  load.get('/api/members');
  onDestroy(() => load.stop());

  // A link such as /members?state=CA (from search) opens with that filter set.
  const seed = untrack(() => ({ chamber: query?.get('chamber') ?? '', state: query?.get('state')?.toUpperCase() ?? '' }));
  let chamber = $state(['senate', 'house'].includes(seed.chamber) ? seed.chamber : '');
  let stateCode = $state(seed.state);
  let party = $state('');
  let name = $state('');

  const parties = $derived([...new Set((load.data?.members ?? []).map((m) => m.party ?? ''))].sort());
  const shown = $derived(
    (load.data?.members ?? [])
      .filter((m) => !chamber || m.body === chamber)
      .filter((m) => !stateCode || m.state === stateCode)
      .filter((m) => !party || (m.party ?? '') === party)
      .filter((m) => !name || m.name.toLowerCase().includes(name.trim().toLowerCase()))
      .sort((a, b) => a.lastName.localeCompare(b.lastName) || a.name.localeCompare(b.name)),
  );
</script>

<h1 tabindex="-1">Members</h1>

{#if load.error}
  <p role="alert">Could not load the members: {load.error}.</p>
{:else if !load.data}
  <p aria-busy="true">Loading the members…</p>
{:else}
  <form class="filters" role="search" onsubmit={(e) => e.preventDefault()}>
    <label>
      <span>Name</span>
      <input type="search" bind:value={name} autocomplete="off" spellcheck="false" />
    </label>
    <label>
      <span>Chamber</span>
      <select bind:value={chamber}>
        <option value="">Both</option>
        <option value="senate">Senate</option>
        <option value="house">House</option>
      </select>
    </label>
    <label>
      <span>State</span>
      <select bind:value={stateCode}>
        <option value="">All</option>
        {#each load.data.states as s (s.code)}
          <option value={s.code}>{s.name}</option>
        {/each}
      </select>
    </label>
    <label>
      <span>Party</span>
      <select bind:value={party}>
        <option value="">All</option>
        {#each parties as p (p)}
          <option value={p}>{p || 'Unknown'}</option>
        {/each}
      </select>
    </label>
  </form>

  <p class="count" aria-live="polite">{shown.length} of {load.data.members.length} members</p>

  {#if shown.length === 0}
    <p>No member matches these filters.</p>
  {:else}
    <ul class="list">
      {#each shown as m (m.id)}
        <li>
          <Portrait id={m.id} name={m.name} has={m.portrait} size={2.75} />
          <div class="who">
            <a href="/people/{m.id}">{m.name}</a>
            <span class="detail">
              <PartyMark code={m.partyCode} party={m.party} caucus={m.caucus} />
              <span class="muted">{m.body === 'senate' ? 'Senator for' : 'Representative for'} {seatName(m)}{#if !m.voting} (non-voting){/if}</span>
            </span>
          </div>
          <span class="term muted small">until {calendarDate(m.end)}</span>
        </li>
      {/each}
    </ul>
  {/if}
  <Sources sources={load.data.sources} />
{/if}

<style>
  .filters {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr));
    gap: 0.75rem 1rem;
    margin-block: 1.5rem 1rem;
  }
  label {
    display: grid;
    min-inline-size: 0;
    gap: 0.25rem;
    font-weight: 600;
    font-size: 0.875rem;
  }
  .count {
    color: var(--muted);
    margin-block-end: 0.75rem;
  }
  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    container-type: inline-size;
  }
  .list li {
    display: flex;
    gap: 0.875rem;
    align-items: center;
    padding-block: 0.625rem;
    border-block-end: 1px solid var(--line);
    content-visibility: auto;
    contain-intrinsic-size: auto 4.5rem;
  }
  .who {
    display: grid;
    flex: 1;
    min-inline-size: 0;
  }
  .who a {
    font-weight: 650;
  }
  .detail {
    display: flex;
    flex-wrap: wrap;
    gap: 0.125rem 0.875rem;
    font-size: 0.9375rem;
  }
  .small {
    font-size: 0.875rem;
  }
  @container (max-width: 34rem) {
    .term {
      display: none;
    }
  }
</style>
