<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import type { SearchResult } from '../lib/api';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';
  import { router } from '../lib/router.svelte';

  let { query }: Partial<PageProps> = $props();

  // The query lives in the URL (/search?q=…), so a search can be bookmarked
  // and the back button returns to the previous one.
  document.title = 'Search · Eunomia';
  const load = new Load<{ q: string; results: SearchResult[] }>();
  onDestroy(() => load.stop());

  const q = $derived(query?.get('q')?.trim() ?? '');
  let text = $state(untrack(() => query?.get('q') ?? ''));

  $effect(() => {
    if (q) load.get(`/api/search?q=${encodeURIComponent(q)}`);
  });

  const groups = $derived.by(() => {
    const rs = load.data?.q === q ? load.data.results : [];
    return [
      { kind: 'state', label: 'States and territories', items: rs.filter((r) => r.kind === 'state') },
      { kind: 'person', label: 'Members', items: rs.filter((r) => r.kind === 'person') },
      { kind: 'committee', label: 'Committees', items: rs.filter((r) => r.kind === 'committee') },
    ].filter((g) => g.items.length > 0);
  });

  const announce = $derived.by(() => {
    if (!q || !load.data || load.data.q !== q) return '';
    const n = load.data.results.length;
    return n === 0 ? `Nothing matches ${q}.` : `${n} ${n === 1 ? 'result' : 'results'} for ${q}.`;
  });

  function href(r: SearchResult): string {
    if (r.kind === 'person') return `/people/${r.id}`;
    if (r.kind === 'committee') return `/committees/${r.id}`;
    return `/members?state=${encodeURIComponent(r.id)}`;
  }

  function submit(e: SubmitEvent): void {
    e.preventDefault();
    const t = text.trim();
    router.go(t ? `/search?q=${encodeURIComponent(t)}` : '/search');
  }
</script>

<h1 tabindex="-1">Search</h1>

<form role="search" onsubmit={submit}>
  <label for="q">Name of a member, state or committee</label>
  <div class="row">
    <input id="q" type="search" name="q" bind:value={text} autocomplete="off" spellcheck="false" enterkeyhint="search" />
    <button type="submit">Search</button>
  </div>
</form>

<!-- One polite announcement per finished search: the count, never "Searching…". -->
<p class="visually-hidden" aria-live="polite">{announce}</p>

<div aria-busy={!!q && !load.error && (!load.data || load.data.q !== q)}>
  {#if !q}
    <p class="muted">Try a last name, a state such as “Ohio” or “OH”, or a word like “Judiciary”.</p>
  {:else if load.error}
    <p role="alert">Could not search: {load.error}.</p>
  {:else if !load.data || load.data.q !== q}
    <p class="muted">Searching…</p>
  {:else if groups.length === 0}
    <p>Nothing matches “{q}”.</p>
  {:else}
    {#each groups as g (g.kind)}
      <section aria-labelledby="h-{g.kind}">
        <h2 id="h-{g.kind}">{g.label}</h2>
        <ul>
          {#each g.items as r (r.kind + r.id)}
            <li>
              <a href={href(r)}>{r.label}</a>
              {#if r.kind === 'person'}
                {@const [party, ...seat] = r.detail.split(' · ')}
                <span class="detail small">
                  <PartyMark code={r.partyCode ?? ''} party={party === 'party unknown' ? '' : party} />
                  <span class="muted">{seat.join(' · ')}</span>
                </span>
              {:else if r.kind === 'state'}
                <span class="muted small">Its members of Congress</span>
              {:else}
                <span class="muted small">{r.detail}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  {/if}
</div>

<style>
  form {
    margin-block: 1rem 2rem;
    max-inline-size: 36rem;
  }
  label {
    display: block;
    font-weight: 600;
    margin-block-end: 0.375rem;
  }
  .row {
    display: flex;
    gap: 0.5rem;
  }
  input {
    flex: 1;
    min-inline-size: 0;
  }
  button {
    min-block-size: 2.5rem;
    padding-inline: 1rem;
    background: var(--accent);
    color: var(--bg);
    border: none;
    border-radius: var(--radius-s);
    font-weight: 650;
    cursor: pointer;
  }
  @media (pointer: coarse) {
    button {
      min-block-size: 2.75rem;
    }
  }
  section {
    margin-block-end: 2rem;
  }
  h2 {
    margin-block-end: 0.5rem;
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
  .detail {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.75rem;
  }
  .small {
    font-size: 0.875rem;
  }
</style>
