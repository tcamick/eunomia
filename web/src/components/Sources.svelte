<script lang="ts">
  import type { SourceLine } from '../lib/api';
  import { localDateTime, relativeTime } from '../lib/format';

  // Where the page's facts came from and how fresh they are (PRD §3).
  let { sources, now = new Date() }: { sources: SourceLine[]; now?: Date } = $props();
</script>

{#if sources.length > 0}
  <footer class="sources">
    <h2 class="visually-hidden">Sources</h2>
    <ul>
      {#each sources as s (s.url + s.name)}
        <li>
          Source: <a href={s.url} rel="noopener noreferrer">{s.name}</a>{#if s.checkedAt}, checked
            <time datetime={s.checkedAt}>{relativeTime(s.checkedAt, now)}</time>{/if}{#if s.changedAt}, last changed
            <time datetime={s.changedAt}>{localDateTime(s.changedAt)}</time>{/if}.
        </li>
      {/each}
    </ul>
  </footer>
{/if}

<style>
  .sources {
    margin-block-start: 3rem;
    padding-block-start: 1rem;
    border-block-start: 1px solid var(--line);
    color: var(--muted);
    font-size: 0.875rem;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.25rem;
  }
</style>
