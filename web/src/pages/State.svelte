<script lang="ts">
  import { onDestroy } from 'svelte';
  import NotFound from './NotFound.svelte';
  import DistrictMap from '../components/DistrictMap.svelte';
  import MapDefs from '../components/MapDefs.svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import Portrait from '../components/Portrait.svelte';
  import Sources from '../components/Sources.svelte';
  import StoryList from '../components/StoryList.svelte';
  import type { Delegation, Member, StateResponse, StateVacancy } from '../lib/api';
  import { calendarDate, ordinal } from '../lib/format';
  import { Load } from '../lib/load.svelte';
  import type { PageProps } from '../lib/page';

  let { params }: Partial<PageProps> = $props();

  const load = new Load<StateResponse>();
  $effect(() => load.get(`/api/states/${params?.['code'] ?? ''}`));
  onDestroy(() => load.stop());
  $effect(() => {
    document.title = `${load.data?.state.name ?? 'State'} · Eunomia`;
  });

  const kinds = { state: 'State', district: 'Federal district', territory: 'Territory' };

  type Row = { district: number; member?: Member; vacancy?: StateVacancy };
  // The House seats in district order, members and vacancies together.
  const houseRows = $derived.by((): Row[] => {
    const d = load.data;
    if (!d) return [];
    const rows: Row[] = d.house.map((m) => ({ district: m.district ?? 0, member: m }));
    for (const v of d.vacancies) if (v.body === 'house') rows.push({ district: v.district ?? 0, vacancy: v });
    return rows.sort((a, b) => a.district - b.district);
  });
  const senateVacancies = $derived(load.data?.vacancies.filter((v) => v.body === 'senate') ?? []);

  const seat = (n: number) => (n === 0 ? 'At large' : `${ordinal(n)} district`);
  const rank = (m: Member) => (m.stateRank ? `${m.stateRank[0]!.toUpperCase()}${m.stateRank.slice(1)} senator` : 'Senator');
</script>

{#snippet totals(label: string, d: Delegation)}
  <div class="total">
    <span class="what">{label}</span>
    <ul>
      {#each d.parties as p (p.party + (p.caucus ?? ''))}
        <li><PartyMark code={p.code} party={p.party} caucus={p.caucus} /> <span class="num">{p.seats}</span></li>
      {/each}
      {#if d.vacant > 0}
        <li><PartyMark code="vacant" party="Vacant" /> <span class="num">{d.vacant}</span></li>
      {/if}
    </ul>
  </div>
{/snippet}

{#snippet vacant(v: StateVacancy)}
  <span class="vacant">Vacant</span>
  {#if v.lastHolder}
    <span class="muted small">
      last held by <a href="/people/{v.lastHolder.person}">{v.lastHolder.name}</a>{#if v.lastHolder.party}
        ({v.lastHolder.party}){/if}, no longer listed from {calendarDate(v.lastHolder.leftOn)}
    </span>
  {/if}
{/snippet}

{#if load.missing}
  <NotFound what="state or territory" />
{:else if load.error}
  <h1 tabindex="-1">State</h1>
  <p role="alert">Could not load this state: {load.error}.</p>
{:else if !load.data}
  <h1 tabindex="-1" class="visually-hidden">State</h1>
  <p aria-busy="true">Loading the state…</p>
{:else}
  {@const d = load.data}
  <MapDefs />
  <p class="kicker muted">{kinds[d.state.kind]} · <a href="/map">the map</a></p>
  <h1 tabindex="-1">{d.state.name}</h1>
  <div class="totals">
    {#if d.state.senate.seats > 0}{@render totals('Senate', d.state.senate)}{/if}
    {@render totals(d.state.voting ? 'House' : 'Delegate to the House', d.state.house)}
  </div>

  {#if d.state.senate.seats > 0}
    <section aria-labelledby="h-senate">
      <h2 id="h-senate">Senators</h2>
      <ul class="people">
        {#each d.senators as m (m.id)}
          <li>
            <Portrait id={m.id} name={m.name} has={m.portrait} size={4} />
            <div class="who">
              <a href="/people/{m.id}">{m.name}</a>
              <PartyMark code={m.partyCode} party={m.party} caucus={m.caucus} />
              <span class="muted small">{rank(m)}{#if m.class}, class {m.class}{/if}; term ends {calendarDate(m.end)}</span>
            </div>
          </li>
        {/each}
        {#each senateVacancies as v, i (i)}
          <li class="empty">{@render vacant(v)}</li>
        {/each}
      </ul>
    </section>
  {:else}
    <p class="muted">{d.state.name} has no senators.</p>
  {/if}

  <section aria-labelledby="h-house">
    <h2 id="h-house">{d.state.voting ? 'House delegation' : 'Delegate to the House'}</h2>
    {#if !d.state.voting}
      <p class="muted small">Its member of the House can vote in committee but not on the House floor.</p>
    {/if}
    <div class="house" class:with-map={!!d.districtMap}>
      {#if d.districtMap}
        <DistrictMap code={d.state.code} house={d.house} congress={d.districtMap} sitting={d.houseCongress ?? d.districtMap} />
      {/if}
      <ol class="seats">
        {#each houseRows as r (r.district + (r.member?.id ?? 'vacant'))}
          <li>
            <span class="district num">{seat(r.district)}</span>
            {#if r.member}
              <Portrait id={r.member.id} name={r.member.name} has={r.member.portrait} size={2.5} />
              <span class="who">
                <a href="/people/{r.member.id}">{r.member.name}</a>
                <PartyMark code={r.member.partyCode} party={r.member.party} caucus={r.member.caucus} />
              </span>
            {:else if r.vacancy}
              <span class="who">{@render vacant(r.vacancy)}</span>
            {/if}
          </li>
        {/each}
      </ol>
    </div>
  </section>

  <section aria-labelledby="h-news">
    <h2 id="h-news">In the news</h2>
    <p class="muted small">
      Stories that name {d.state.name} or anyone who sits for it. <a href="/news?state={d.state.code}">See them all</a>
    </p>
    <StoryList filter="state={d.state.code}" limit={5} level={3} paged={false}
      empty="No story from your sources has named {d.state.name} or its members lately." />
  </section>

  <Sources sources={d.sources} />
{/if}

<style>
  .kicker {
    font-weight: 600;
    margin-block-end: 0.25rem;
  }
  .totals {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 2.5rem;
    margin-block: 1rem 2rem;
  }
  .total {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
    align-items: baseline;
  }
  .what {
    font-weight: 700;
  }
  .total ul {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .total .num {
    font-weight: 760;
  }
  section {
    margin-block-start: 2rem;
  }
  h2 {
    margin-block-end: 0.75rem;
  }
  .people {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr));
    gap: 1rem 2rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .people li {
    display: flex;
    gap: 0.875rem;
    align-items: center;
  }
  .who {
    display: grid;
    min-inline-size: 0;
  }
  .who a {
    font-weight: 650;
  }
  @media (pointer: coarse) {
    :is(.who, .empty) a {
      display: inline-flex;
      align-items: center;
      min-block-size: 2.75rem;
    }
  }
  .house {
    display: grid;
    gap: 1.5rem 2.5rem;
    align-items: start;
  }
  .house.with-map {
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
  }
  .seats {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .seats li {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    padding-block: 0.4rem;
    border-block-end: 1px solid var(--line);
  }
  .district {
    flex: 0 0 6.5rem;
    color: var(--muted);
    font-size: 0.9375rem;
  }
  .vacant {
    font-weight: 650;
  }
  .small {
    font-size: 0.875rem;
  }
</style>
