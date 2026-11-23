<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import StoryList from '../components/StoryList.svelte';
  import type { FeedInfo, Jurisdiction } from '../lib/api';
  import { getJSON } from '../lib/api';
  import type { PageProps } from '../lib/page';

  let { query }: Partial<PageProps> = $props();

  // The feed (PRD §6.6), phone first. The filters live in the address, so
  // a filtered view can be bookmarked; changing one is not a navigation, so
  // focus stays on the control.
  document.title = 'News · Eunomia';
  const seed = untrack(() => query ?? new URLSearchParams());
  let feed = $state(seed.get('feed') ?? '');
  let stateCode = $state(seed.get('state') ?? '');
  let person = $state(seed.get('person') ?? '');
  let unread = $state(seed.get('unread') === '1');
  let saved = $state(seed.get('saved') === '1');

  let feeds = $state<FeedInfo[]>([]);
  let states = $state<Jurisdiction[]>([]);
  const ctrl = new AbortController();
  getJSON<{ feeds: FeedInfo[] }>('/api/stories?limit=1', ctrl.signal)
    .then((r) => (feeds = r.feeds))
    .catch(() => {});
  getJSON<{ states: Jurisdiction[] }>('/api/members', ctrl.signal)
    .then((r) => (states = r.states))
    .catch(() => {});
  onDestroy(() => ctrl.abort());

  const filter = $derived.by(() => {
    const q = new URLSearchParams();
    if (feed) q.set('feed', feed);
    if (stateCode) q.set('state', stateCode);
    if (person) q.set('person', person);
    if (unread) q.set('unread', '1');
    if (saved) q.set('saved', '1');
    return q.toString();
  });

  $effect(() => {
    const url = new URL(location.href);
    url.search = filter;
    history.replaceState(history.state, '', url.pathname + (filter ? `?${filter}` : ''));
  });
</script>

<h1 tabindex="-1">News</h1>
<p class="lede muted">The latest from the sources you chose, each named, with whom each story mentions.</p>

<form class="filters" role="search" onsubmit={(e) => e.preventDefault()}>
  <label>
    <span>Source</span>
    <select bind:value={feed}>
      <option value="">All</option>
      {#each feeds as f (f.id)}
        <option value={f.id}>{f.name}</option>
      {/each}
    </select>
  </label>
  <label>
    <span>State</span>
    <select bind:value={stateCode}>
      <option value="">All</option>
      {#each states as s (s.code)}
        <option value={s.code}>{s.name}</option>
      {/each}
    </select>
  </label>
  <label class="check"><input type="checkbox" bind:checked={unread} /> Unread only</label>
  <label class="check"><input type="checkbox" bind:checked={saved} /> Saved only</label>
  {#if person}
    <p class="person">
      Stories naming <a href="/people/{person}">this member</a>
      <button type="button" onclick={() => (person = '')}>Show all</button>
    </p>
  {/if}
</form>

<StoryList {filter} empty={unread || saved || feed || stateCode || person ? 'No story matches these filters.' : 'No stories yet; the news job runs every half hour.'} />

<style>
  .lede {
    margin-block: 0.5rem 1.25rem;
  }
  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem 1.25rem;
    align-items: end;
    margin-block-end: 0.5rem;
  }
  label {
    display: grid;
    gap: 0.25rem;
    min-inline-size: 0;
    font-weight: 600;
    font-size: 0.875rem;
  }
  .check {
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
    min-block-size: 2.5rem;
    font-weight: 400;
    font-size: 1rem;
  }
  .check input {
    inline-size: 1.1rem;
    block-size: 1.1rem;
    accent-color: var(--accent);
  }
  .person {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    flex-basis: 100%;
  }
  .person button {
    min-block-size: 2rem;
    padding-inline: 0.75rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--panel);
    color: var(--fg);
    cursor: pointer;
  }
  @media (pointer: coarse) {
    .check,
    .person button {
      min-block-size: 2.75rem;
    }
  }
</style>
