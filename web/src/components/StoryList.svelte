<script lang="ts">
  import Sources from './Sources.svelte';
  import { getJSON, setMarks, type StoriesResponse, type Story } from '../lib/api';
  import { localDateTime, relativeTime } from '../lib/format';
  import { httpsOnly } from '../lib/url';

  // A list of stories (PRD §6.6): each with its source named and labelled
  // before the headline, since a headline is its publisher's claim, not a
  // fact (AGENTS.md, Rules for facts 6); the summary as text, never HTML;
  // whom and what it names; and the owner's read and saved marks, kept on
  // the server so they follow the owner from device to device.
  let {
    filter,
    limit = 30,
    level = 2,
    empty = 'No stories yet.',
    paged = true,
    sources = true,
  }: {
    /** The API's query: "person=A000001", "state=OH&unread=1", "". */
    filter: string;
    limit?: number;
    /** The heading level of each headline. */
    level?: 2 | 3;
    empty?: string;
    paged?: boolean;
    sources?: boolean;
  } = $props();

  let stories = $state<Story[]>([]);
  let response = $state<StoriesResponse | null>(null);
  let error = $state('');
  let loading = $state(false);
  let markError = $state('');
  // What the owner's last mark did, for a screen reader: one polite region
  // (web-conventions, Live updates).
  let announce = $state('');
  let ctrl: AbortController | null = null;

  async function load(offset: number): Promise<void> {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    loading = true;
    error = '';
    try {
      const q = new URLSearchParams(filter);
      q.set('limit', String(limit));
      q.set('offset', String(offset));
      const r = await getJSON<StoriesResponse>(`/api/stories?${q}`, c.signal);
      if (c.signal.aborted) return;
      response = r;
      stories = offset === 0 ? r.stories : [...stories, ...r.stories];
    } catch (e) {
      if (!c.signal.aborted) error = e instanceof Error ? e.message : String(e);
    } finally {
      if (!c.signal.aborted) loading = false;
    }
  }

  $effect(() => {
    void filter;
    void load(0);
    return () => ctrl?.abort();
  });

  // A mark shows at once and is undone if the server refuses it.
  async function mark(s: Story, what: 'read' | 'saved', on: boolean): Promise<void> {
    const key = what === 'read' ? 'readAt' : 'savedAt';
    const before = s[key];
    s[key] = on ? new Date().toISOString() : undefined;
    markError = '';
    try {
      const m = await setMarks(s.id, { [what]: on });
      s.readAt = m.readAt;
      s.savedAt = m.savedAt;
      announce = what === 'read' ? (on ? 'Marked read' : 'Marked unread') : on ? 'Saved' : 'Removed from saved';
    } catch (e) {
      s[key] = before;
      markError = `Could not save that mark: ${e instanceof Error ? e.message : String(e)}.`;
      announce = markError;
    }
  }

  const when = (s: Story) => s.published ?? s.fetchedAt;
  const now = new Date();
</script>

{#if error}
  <p role="alert">Could not load the stories: {error}.</p>
{:else if !response && loading}
  <p aria-busy="true" class="muted">Loading the stories…</p>
{:else if stories.length === 0}
  <p class="muted">{empty}</p>
{:else}
  {#if markError}<p class="small mark-error">{markError}</p>{/if}
  <ol class="stories">
    {#each stories as s (s.id)}
      {@const href = httpsOnly(s.url)}
      <li class:read={!!s.readAt}>
        <p class="meta">
          <span class="source">{s.feedName}</span>
          <span class="label">{s.label}</span>
          <span aria-hidden="true">·</span>
          <time datetime={when(s)} title={localDateTime(when(s))}
            >{#if s.published}{relativeTime(s.published, now)}{:else}fetched {relativeTime(s.fetchedAt, now)}{/if}</time
          >
        </p>
        <svelte:element this={`h${level}`} class="title">
          {#if href}
            <a {href} rel="noopener noreferrer" onclick={() => !s.readAt && mark(s, 'read', true)}>{s.title}</a>
          {:else}
            {s.title}
          {/if}
        </svelte:element>
        {#if s.summary}<p class="summary">{s.summary}</p>{/if}
        {#if s.mentions.length > 0}
          <ul class="tags" aria-label="Names">
            {#each s.mentions as m (m.kind + m.ref)}
              <li>
                <a href={m.kind === 'person' ? `/people/${m.ref}` : `/states/${m.ref}`}>{m.name ?? m.ref}</a>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="actions">
          <button type="button" aria-pressed={!!s.readAt} onclick={() => mark(s, 'read', !s.readAt)}>
            {s.readAt ? 'Read' : 'Mark read'}
          </button>
          <button type="button" aria-pressed={!!s.savedAt} onclick={() => mark(s, 'saved', !s.savedAt)}>
            {s.savedAt ? 'Saved' : 'Save'}
          </button>
        </div>
      </li>
    {/each}
  </ol>
  {#if paged && response?.next}
    <p class="more">
      <button type="button" onclick={() => load(response?.next ?? 0)} disabled={loading} aria-busy={loading}>More stories</button>
    </p>
  {/if}
{/if}
<p class="visually-hidden" aria-live="polite">{announce}</p>
{#if sources && response}<Sources sources={response.sources} />{/if}

<style>
  .stories {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .stories > li {
    padding-block: 1rem;
    border-block-end: 1px solid var(--line);
    display: grid;
    gap: 0.375rem;
    content-visibility: auto;
    contain-intrinsic-block-size: auto 11rem;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.5rem;
    align-items: baseline;
    font-size: 0.875rem;
    color: var(--muted);
  }
  .source {
    font-weight: 700;
    color: var(--fg);
  }
  .title {
    margin: 0;
    font-size: 1.125rem;
    font-weight: 700;
    line-height: 1.3;
  }
  .title a {
    color: var(--fg);
  }
  .read .title,
  .read .title a,
  .read .summary {
    color: var(--muted);
  }
  .summary {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .tags a {
    display: inline-flex;
    align-items: center;
    min-block-size: 1.75rem;
    padding-inline: 0.625rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    font-size: 0.875rem;
    text-decoration: none;
    background: var(--panel);
  }
  .actions {
    display: flex;
    gap: 0.5rem;
  }
  button {
    min-block-size: 2rem;
    padding-inline: 0.75rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--panel);
    color: var(--fg);
    font-size: 0.875rem;
    cursor: pointer;
  }
  button[aria-pressed='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
    font-weight: 650;
  }
  @media (pointer: coarse) {
    button,
    .tags a {
      min-block-size: 2.75rem;
    }
  }
  .more {
    margin-block-start: 1rem;
  }
  .small {
    font-size: 0.875rem;
  }
  .mark-error {
    color: var(--bad);
  }
</style>
