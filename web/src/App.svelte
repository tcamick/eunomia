<script lang="ts">
  import { onMount } from 'svelte';
  import { getJSON, type Status } from './lib/api';
  import { duration, localDateTime, relativeTime } from './lib/format';

  // E0 has one page, Status. E1a adds the shell, navigation and routes.
  let status = $state<Status | null>(null);
  let error = $state('');

  onMount(() => {
    document.title = 'Status · Eunomia';
    const ctrl = new AbortController();
    getJSON<Status>('/api/status', ctrl.signal)
      .then((s) => (status = s))
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) error = e instanceof Error ? e.message : String(e);
      });
    return () => ctrl.abort();
  });
</script>

<header class="top">
  <p class="brand">Eunomia</p>
</header>

<main aria-busy={status === null && error === ''}>
  <h1>Status</h1>

  {#if error}
    <p role="alert">Could not load the status: {error}</p>
  {:else if status}
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
              {@const now = new Date(status.now)}
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
  {:else}
    <p>Loading the status…</p>
  {/if}
</main>

<style>
  .top {
    padding-block: 0.75rem;
    padding-inline: max(1rem, env(safe-area-inset-left)) max(1rem, env(safe-area-inset-right));
    border-block-end: 1px solid var(--line);
  }
  .brand {
    margin: 0;
    font-weight: 700;
    letter-spacing: 0.02em;
  }
  main {
    max-inline-size: 60rem;
    margin-inline: auto;
    padding: 1rem;
  }
  .meta {
    color: var(--muted);
  }
  code {
    font-family: var(--face-mono);
  }
  .table-wrap {
    container-type: inline-size;
    overflow-x: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius);
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
  .message {
    display: block;
    color: var(--muted);
    font-size: 0.875rem;
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
