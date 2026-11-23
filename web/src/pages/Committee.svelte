<script lang="ts">
  import { onDestroy } from 'svelte';
  import NotFound from './NotFound.svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import Portrait from '../components/Portrait.svelte';
  import Sources from '../components/Sources.svelte';
  import type { CommitteeMember, CommitteeResponse } from '../lib/api';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';
  import { httpsOnly } from '../lib/url';

  let { params }: Partial<PageProps> = $props();

  const load = new Load<CommitteeResponse>();
  $effect(() => load.get(`/api/committees/${params?.['id'] ?? ''}`));
  onDestroy(() => load.stop());

  $effect(() => {
    document.title = `${load.data?.committee.name ?? 'Committee'} · Eunomia`;
  });

  const chamberName = { senate: 'Senate', house: 'House', joint: 'Joint' };
  const byRank = (a: CommitteeMember, b: CommitteeMember): number =>
    (a.rank ?? 999) - (b.rank ?? 999) || a.name.localeCompare(b.name);
  const sides = $derived([
    { id: 'majority', label: 'Majority', members: (load.data?.members ?? []).filter((m) => m.side === 'majority').sort(byRank) },
    { id: 'minority', label: 'Minority', members: (load.data?.members ?? []).filter((m) => m.side === 'minority').sort(byRank) },
  ]);
</script>

{#if load.missing}
  <NotFound what="committee" />
{:else if load.error}
  <h1 tabindex="-1">Committee</h1>
  <p role="alert">Could not load this committee: {load.error}.</p>
{:else if !load.data}
  <h1 tabindex="-1" class="visually-hidden">Committee</h1>
  <p aria-busy="true">Loading the committee…</p>
{:else}
  {@const c = load.data.committee}
  {@const site = httpsOnly(c.url)}
  <p class="kicker muted">
    {chamberName[c.chamber]}
    {#if load.data.parent}subcommittee of <a href="/committees/{load.data.parent.id}">{load.data.parent.name}</a>{:else}committee{/if}
  </p>
  <h1 tabindex="-1">{c.name}</h1>
  {#if c.jurisdiction}<p class="lede">{c.jurisdiction}</p>{/if}
  {#if site}<p class="site"><a href={site} rel="noopener noreferrer">The committee's website</a></p>{/if}

  <div class="sides">
    {#each sides as s (s.id)}
      {#if s.members.length > 0}
        <section aria-labelledby="h-{s.id}">
          <h2 id="h-{s.id}">{s.label} <span class="count muted num">{s.members.length}</span></h2>
          <ol>
            {#each s.members as m (m.person)}
              <li>
                <Portrait id={m.person} name={m.name} has={m.portrait} size={2.5} />
                <div class="who">
                  <a href="/people/{m.person}">{m.name}</a>
                  <span class="detail">
                    <PartyMark code={m.partyCode} party={m.party} />
                    {#if m.state}<span class="muted">{m.state}</span>{/if}
                    {#if m.title}<strong class="title">{m.title}</strong>{/if}
                  </span>
                </div>
              </li>
            {/each}
          </ol>
        </section>
      {/if}
    {/each}
  </div>
  {#if load.data.members.length === 0}
    <p>The roster lists no members for this committee.</p>
  {/if}

  {#if load.data.subcommittees.length > 0}
    <section class="subs" aria-labelledby="h-subs">
      <h2 id="h-subs">Subcommittees</h2>
      <ul>
        {#each load.data.subcommittees as sc (sc.id)}
          <li>
            <a href="/committees/{sc.id}">{sc.name}</a>
            <span class="muted small">{sc.members} members{#if sc.chair}; chair {sc.chair}{/if}</span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <Sources sources={load.data.sources} />
{/if}

<style>
  .kicker {
    font-weight: 600;
    margin-block-end: 0.25rem;
  }
  .lede {
    margin-block: 0.75rem 0;
  }
  .site {
    margin-block-start: 0.5rem;
  }
  .sides {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
    gap: 2rem 3rem;
    margin-block-start: 2rem;
    align-items: start;
  }
  h2 {
    margin-block-end: 0.75rem;
  }
  .count {
    font-size: 1rem;
    font-weight: 600;
  }
  ol,
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  ol li {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    padding-block: 0.5rem;
    border-block-end: 1px solid var(--line);
  }
  .who {
    display: grid;
    min-inline-size: 0;
  }
  .who a {
    font-weight: 620;
  }
  .detail {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.75rem;
    font-size: 0.9375rem;
  }
  .title {
    font-weight: 700;
  }
  .subs {
    margin-block-start: 2.5rem;
  }
  .subs li {
    display: grid;
    padding-block: 0.5rem;
    border-block-end: 1px solid var(--line);
  }
  .small {
    font-size: 0.875rem;
  }
</style>
