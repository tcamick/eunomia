<script lang="ts">
  import { onDestroy } from 'svelte';
  import NotFound from './NotFound.svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import Portrait from '../components/Portrait.svelte';
  import Sources from '../components/Sources.svelte';
  import type { Person, PersonSeat, PersonTerm } from '../lib/api';
  import { calendarDate, seatName, year } from '../lib/format';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';
  import { partyCode } from '../lib/party';
  import { httpsOnly } from '../lib/url';

  let { params }: Partial<PageProps> = $props();

  const load = new Load<Person>();
  $effect(() => load.get(`/api/people/${params?.['id'] ?? ''}`));
  onDestroy(() => load.stop());

  $effect(() => {
    document.title = `${load.data?.name ?? 'Member'} · Eunomia`;
  });

  const p = $derived(load.data);
  const current = $derived(p?.terms.find((t) => t.current));
  const terms = $derived([...(p?.terms ?? [])].sort((a, b) => b.start.localeCompare(a.start)));
  const today = new Date().toISOString().slice(0, 10);
  const roles = $derived(
    [...(p?.roles ?? [])].sort((a, b) => Number(isCurrent(b.end)) - Number(isCurrent(a.end)) || b.start.localeCompare(a.start)),
  );

  // Committee seats: each top-level committee, with this member's
  // subcommittees of it beneath.
  const committees = $derived.by(() => {
    const seats = p?.seats ?? [];
    const top = seats.filter((s) => !s.parent);
    const subsOf = (id: string): PersonSeat[] => seats.filter((s) => s.parent === id);
    const orphans = seats.filter((s) => s.parent && !top.some((t) => t.committee === s.parent));
    return { top: top.map((t) => ({ seat: t, subs: subsOf(t.committee) })), orphans };
  });

  function isCurrent(end?: string): boolean {
    return !end || end >= today;
  }

  function office(t: PersonTerm): string {
    return t.body === 'senate' ? 'Senator' : 'Representative';
  }

  function span(t: PersonTerm): string {
    const end = t.leftOn ?? t.end;
    return t.current ? `${year(t.start)}–` : `${year(t.start)}–${year(end)}`;
  }

  function how(t: PersonTerm): string {
    if (t.how === 'appointment') return 'appointed';
    if (t.how === 'special-election') return 'won a special election';
    return '';
  }

  function seatRole(s: PersonSeat): string {
    if (s.title) return s.title;
    return s.side === 'majority' ? 'Majority member' : 'Minority member';
  }
</script>

{#if load.missing}
  <NotFound what="member" />
{:else if load.error}
  <h1 tabindex="-1">Member</h1>
  <p role="alert">Could not load this member: {load.error}.</p>
{:else if !p}
  <h1 tabindex="-1" class="visually-hidden">Member</h1>
  <p aria-busy="true">Loading the member…</p>
{:else}
  <header class="who">
    <Portrait id={p.id} name={p.name} has={p.portrait} size={7} />
    <div class="titles">
      <h1 tabindex="-1">{p.name}</h1>
      {#if current}
        <p class="seat">
          {current.body === 'senate' ? 'Senator for' : 'Representative for'}
          {seatName(current)}{#if current.stateRank}{' '}<span class="muted">({current.stateRank} senator)</span>{/if}
        </p>
        <p><PartyMark code={current.partyCode} party={current.party} caucus={current.caucus} /></p>
        <p class="muted small">This term began {calendarDate(current.start)} and ends {calendarDate(current.end)}.</p>
      {:else}
        <p class="seat muted">Not serving in Congress now.</p>
      {/if}
    </div>
  </header>

  {#if roles.length > 0}
    <section aria-labelledby="h-roles">
      <h2 id="h-roles">Leadership</h2>
      <ul class="plain">
        {#each roles as r (r.title + r.start)}
          <li>
            <span class:muted={!isCurrent(r.end)}>{r.title}</span>
            <span class="muted small">
              {#if isCurrent(r.end)}since {calendarDate(r.start)}{:else}{calendarDate(r.start)} to {calendarDate(r.end)}{/if}
            </span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if p.seats.length > 0}
    <section aria-labelledby="h-committees">
      <h2 id="h-committees">Committees</h2>
      <ul class="committees">
        {#each committees.top as c (c.seat.committee)}
          <li>
            <a href="/committees/{c.seat.committee}">{c.seat.name}</a>
            <span class="muted small">{seatRole(c.seat)}</span>
            {#if c.subs.length > 0}
              <ul class="subs">
                {#each c.subs as s (s.committee)}
                  <li><a href="/committees/{s.committee}">{s.name}</a> <span class="muted small">{seatRole(s)}</span></li>
                {/each}
              </ul>
            {/if}
          </li>
        {/each}
        {#each committees.orphans as s (s.committee)}
          <li><a href="/committees/{s.committee}">{s.name}</a> <span class="muted small">{seatRole(s)}</span></li>
        {/each}
      </ul>
    </section>
  {/if}

  <section aria-labelledby="h-service">
    <h2 id="h-service">Service in Congress</h2>
    <ol class="terms">
      {#each terms as t (t.start + t.body)}
        <li>
          <span class="years num">{span(t)}</span>
          <span>
            {office(t)} for {seatName(t)}
            {#if how(t)}<span class="muted">({how(t)})</span>{/if}
            {#if t.leftOn && !t.current}<span class="muted">, left {calendarDate(t.leftOn)}</span>{/if}
          </span>
          <span class="party">
            {#if t.affiliations && t.affiliations.length > 1}
              {#each t.affiliations as a, i (i)}
                <span class="aff"><PartyMark code={partyCode(a.party)} party={a.party} caucus={a.caucus} /><span class="muted small">from {calendarDate(a.start)}</span></span>
              {/each}
            {:else}
              <PartyMark code={t.partyCode} party={t.party} caucus={t.caucus} />
            {/if}
          </span>
        </li>
      {/each}
    </ol>
  </section>

  {#if p.links.length > 0}
    <section aria-labelledby="h-links">
      <h2 id="h-links">Elsewhere</h2>
      <ul class="plain">
        {#each p.links as l (l.url)}
          {@const href = httpsOnly(l.url)}
          {#if href}<li><a {href} rel="noopener noreferrer">{l.label}</a></li>{/if}
        {/each}
      </ul>
    </section>
  {/if}

  <Sources sources={p.sources} />
{/if}

<style>
  .who {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem 1.5rem;
    align-items: end;
    margin-block-end: 2.5rem;
  }
  .titles {
    display: grid;
    gap: 0.25rem;
    min-inline-size: min(100%, 16rem);
    flex: 1;
  }
  .titles h1 {
    margin-block-end: 0.25rem;
  }
  .seat {
    font-size: 1.125rem;
    font-weight: 600;
  }
  section {
    margin-block-start: 2.25rem;
  }
  h2 {
    margin-block-end: 0.75rem;
  }
  .plain,
  .committees,
  .subs {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.375rem;
  }
  .plain li,
  .committees li {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.75rem;
    align-items: baseline;
  }
  .subs {
    flex-basis: 100%;
    margin-block: 0.25rem 0.25rem;
    padding-inline-start: 1rem;
    border-inline-start: 2px solid var(--line);
  }
  .terms {
    margin: 0;
    padding: 0;
    list-style: none;
    container-type: inline-size;
  }
  .terms li {
    display: grid;
    grid-template-columns: 7rem 1fr auto;
    gap: 0.25rem 1rem;
    padding-block: 0.5rem;
    border-block-end: 1px solid var(--line);
  }
  .years {
    font-weight: 650;
  }
  .party {
    display: grid;
    justify-items: end;
  }
  .aff {
    display: inline-flex;
    gap: 0.4em;
    align-items: center;
  }
  @container (max-width: 36rem) {
    .terms li {
      grid-template-columns: 1fr;
    }
    .party {
      justify-items: start;
    }
  }
  .small {
    font-size: 0.875rem;
  }
</style>
