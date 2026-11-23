<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { Status } from '../lib/api';
  import { duration, localDateTime, relativeTime } from '../lib/format';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';

  let {}: Partial<PageProps> = $props();

  // Each registered job's last run, from the job history the pods write.
  document.title = 'Status · Eunomia';
  const load = new Load<Status>();
  load.get('/api/status');
  onDestroy(() => load.stop());
  const now = $derived(load.data ? new Date(load.data.now) : new Date());
</script>

<h1 tabindex="-1">Status</h1>

{#if load.error}
  <p role="alert">Could not load the status: {load.error}.</p>
{:else if !load.data}
  <p aria-busy="true">Loading the status…</p>
{:else}
  {@const status = load.data}
  <p class="meta">Version <code>{status.version}</code></p>
  {#if status.jobsError}
    <p role="alert">{status.jobsError}</p>
  {/if}
  {#if status.jobs.length === 0 && !status.jobsError}
    <p>No job has run yet.</p>
  {:else if status.jobs.length > 0}
    <div class="table-wrap">
      <table>
        <caption class="visually-hidden">Each job's last run</caption>
        <thead>
          <tr>
            <th scope="col">Job</th>
            <th scope="col">Last run</th>
            <th scope="col">Result</th>
            <th scope="col">Took</th>
            <th scope="col">Last success</th>
          </tr>
        </thead>
        <tbody>
          {#each status.jobs as job (job.name)}
            <tr>
              <th scope="row"><code>{job.name}</code></th>
              <td data-label="Last run"><time datetime={job.lastRun}>{relativeTime(job.lastRun, now)}</time></td>
              <td data-label="Result">
                {#if job.ok}
                  <span class="ok">✓ Succeeded</span>
                {:else}
                  <span class="bad">✗ Failed</span>
                  {#if job.message}<span class="message">{job.message}</span>{/if}
                {/if}
              </td>
              <td data-label="Took">{duration(job.durationMs)}</td>
              <td data-label="Last success">
                {#if job.lastOk}
                  <time datetime={job.lastOk}>{localDateTime(job.lastOk)}</time>
                {:else}
                  never
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/if}

<style>
  h1 {
    margin-block-end: 1rem;
  }
  .meta {
    color: var(--muted);
    margin-block-end: 1rem;
  }
  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 0.9375em;
  }
  .table-wrap {
    container-type: inline-size;
    overflow-x: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
  }
  table {
    inline-size: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: start;
    padding: 0.5rem 0.75rem;
    border-block-end: 1px solid var(--line);
    vertical-align: top;
  }
  thead th {
    color: var(--muted);
    font-weight: 600;
  }
  tbody tr:last-child > * {
    border-block-end: none;
  }
  .ok {
    color: var(--ok);
  }
  .bad {
    color: var(--bad);
  }
  /* A failure message can hold a URL or a long token with no spaces;
     "anywhere" lets it break, and lowers the cell's minimum width, so the
     table never grows past its container. */
  .message {
    display: block;
    color: var(--muted);
    font-size: 0.875rem;
    overflow-wrap: anywhere;
  }
  /* On a narrow screen each job is a block of label and value pairs; the
     header row stays for screen readers. */
  @container (max-width: 34rem) {
    thead {
      position: absolute;
      inline-size: 1px;
      block-size: 1px;
      overflow: hidden;
      clip-path: inset(50%);
    }
    tbody tr {
      display: block;
      padding-block: 0.5rem;
      border-block-end: 1px solid var(--line);
    }
    tbody tr:last-child {
      border-block-end: none;
    }
    th,
    td {
      display: flex;
      flex-wrap: wrap;
      column-gap: 1rem;
      border: none;
      padding-block: 0.125rem;
    }
    td::before {
      content: attr(data-label);
      flex: 0 0 7.5rem;
      color: var(--muted);
    }
  }
</style>
