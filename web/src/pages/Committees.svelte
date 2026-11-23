<script lang="ts">
  import { onDestroy } from 'svelte';
  import Sources from '../components/Sources.svelte';
  import type { CommitteeSummary, SourceLine } from '../lib/api';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';

  let {}: Partial<PageProps> = $props();

  document.title = 'Committees · Eunomia';
  const load = new Load<{ committees: CommitteeSummary[]; sources: SourceLine[] }>();
  load.get('/api/committees');
  onDestroy(() => load.stop());

  const chambers = [
    { id: 'senate', name: 'Senate' },
    { id: 'house', name: 'House' },
    { id: 'joint', name: 'Joint' },
  ] as const;
  let name = $state('');

  const top = $derived(
    (load.data?.committees ?? [])
      .filter((c) => !c.parent)
      .filter((c) => !name || c.name.toLowerCase().includes(name.trim().toLowerCase()))
      .sort((a, b) => a.name.localeCompare(b.name)),
  );
</script>

<h1 tabindex="-1">Committees</h1>
<p class="lede muted">
  Most of Congress's work on a bill happens in committee. Each chamber has its own; joint committees have members from both.
</p>

{#if load.error}
  <p role="alert">Could not load the committees: {load.error}.</p>
{:else if !load.data}
  <p aria-busy="true">Loading the committees…</p>
{:else if load.data.committees.length === 0}
  <p>The roster has not been loaded yet. It fills in after the roster job's first run.</p>
{:else}
  <form class="filters" role="search" onsubmit={(e) => e.preventDefault()}>
    <label>
      <span>Name</span>
      <input type="search" bind:value={name} autocomplete="off" spellcheck="false" />
    </label>
  </form>

  <div class="chambers">
    {#each chambers as ch (ch.id)}
      {@const list = top.filter((c) => c.chamber === ch.id)}
      {#if list.length > 0}
        <section aria-labelledby="h-{ch.id}">
          <h2 id="h-{ch.id}">{ch.name} <span class="count muted num">{list.length}</span></h2>
          <ul>
            {#each list as c (c.id)}
              <li>
                <a href="/committees/{c.id}">{c.name}</a>
                <span class="muted small">
                  {c.members} members{#if c.subcommittees > 0}, {c.subcommittees} subcommittees{/if}{#if c.chair}; chair
                    {c.chair}{/if}
                </span>
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    {/each}
  </div>
  {#if top.length === 0}
    <p>No committee matches “{name}”.</p>
  {/if}
  <Sources sources={load.data.sources} />
{/if}

<style>
  .lede {
    margin-block: 0.5rem 1.5rem;
  }
  .filters {
    max-inline-size: 22rem;
    margin-block-end: 2rem;
  }
  label {
    display: grid;
    gap: 0.25rem;
    font-weight: 600;
    font-size: 0.875rem;
  }
  .chambers {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
    gap: 2.5rem 3rem;
    align-items: start;
  }
  h2 {
    margin-block-end: 0.75rem;
  }
  .count {
    font-size: 1rem;
    font-weight: 600;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: grid;
    padding-block: 0.5rem;
    border-block-end: 1px solid var(--line);
  }
  li a {
    font-weight: 620;
  }
  .small {
    font-size: 0.875rem;
  }
</style>
