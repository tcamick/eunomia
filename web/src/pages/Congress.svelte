<script lang="ts">
  import { onDestroy } from 'svelte';
  import Hemicycle from '../components/Hemicycle.svelte';
  import PartyMark from '../components/PartyMark.svelte';
  import Portrait from '../components/Portrait.svelte';
  import Sources from '../components/Sources.svelte';
  import type { CongressResponse, MembersResponse } from '../lib/api';
  import { Load } from '../lib/load.svelte';
  import { partyLabel } from '../lib/party';
  import { seatName } from '../lib/format';
  import type { PageProps } from '../lib/page';

  let {}: Partial<PageProps> = $props();

  document.title = 'Congress · Eunomia';
  const congress = new Load<CongressResponse>();
  const members = new Load<MembersResponse>();
  congress.get('/api/congress');
  members.get('/api/members');
  onDestroy(() => {
    congress.stop();
    members.stop();
  });

  const portraitOf = $derived(new Map((members.data?.members ?? []).map((m) => [m.id, m.portrait])));
  const names = { senate: 'Senate', house: 'House of Representatives' };
</script>

<h1 tabindex="-1">Congress</h1>
<p class="lede muted">Who holds each seat now, as the roster lists them. Select a seat, or a name below, to see that member.</p>

{#if congress.error || members.error}
  <p role="alert">Could not load Congress: {congress.error || members.error}.</p>
{:else if !congress.data || !members.data}
  <p aria-busy="true">Loading Congress…</p>
{:else if congress.data.chambers.every((c) => c.filled === 0)}
  <p>The roster has not been loaded yet. It fills in after the roster job's first run.</p>
{:else}
  <div class="chambers">
    {#each congress.data.chambers as c (c.body)}
      <section aria-labelledby="h-{c.body}">
        <h2 id="h-{c.body}">{names[c.body]}</h2>
        <Hemicycle chamber={c} members={members.data.members} />

        <h3 class="visually-hidden">Seats by party</h3>
        <ul class="legend">
          {#each c.parties as p (p.party + (p.caucus ?? ''))}
            <li><PartyMark code={p.code} party={p.party} caucus={p.caucus} /><span class="num">{p.seats}</span></li>
          {/each}
          {#if c.vacancies.some((v) => v.voting)}
            <li><PartyMark code="vacant" party="Vacant" /><span class="num">{c.vacancies.filter((v) => v.voting).length}</span></li>
          {/if}
        </ul>
        <p class="majority">
          {c.majority} of {c.seats} seats is a majority{#if c.body === 'senate'}; the Vice President breaks a 50–50 tie{/if}.
        </p>

        {#if c.vacancies.length > 0}
          <h3>Vacant seats</h3>
          <ul class="plain">
            {#each c.vacancies as v, i (i)}
              <li>
                {seatName({ body: c.body, stateName: v.stateName, district: v.district })}{#if !v.voting}
                  <span class="muted">(non-voting)</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}

        {#if c.leaders.length > 0}
          <h3>Leaders</h3>
          <ul class="leaders">
            {#each c.leaders as l (l.title)}
              <li>
                <Portrait id={l.person} name={l.name} has={portraitOf.get(l.person) ?? false} size={2.5} />
                <div>
                  <a href="/people/{l.person}">{l.name}</a>
                  <span class="muted">{l.title}</span>
                  {#if l.party}<span class="muted small">{partyLabel(l.party)}</span>{/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}

        {#if c.delegates.length > 0}
          <h3>Non-voting members</h3>
          <p class="muted small">The District of Columbia and five territories each send a member who serves on committees but does not vote on bills.</p>
          <ul class="plain">
            {#each c.delegates as d (d.id)}
              <li><a href="/people/{d.id}">{d.name}</a> <span class="muted">{d.state}</span></li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}
  </div>
  <p class="more"><a href="/members">Every member, with filters</a></p>
  <Sources sources={[...congress.data.sources, congress.data.apportionment]} />
{/if}

<style>
  .lede {
    margin-block: 0.5rem 2rem;
  }
  .chambers {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 24rem), 1fr));
    gap: 3rem 4rem;
    align-items: start;
  }
  section > h2 {
    margin-block-end: 1rem;
  }
  h3 {
    margin-block: 1.75rem 0.5rem;
    font-size: 1rem;
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 1.5rem;
    margin: 1rem 0 0;
    padding: 0;
    list-style: none;
  }
  .legend li {
    display: inline-flex;
    gap: 0.5rem;
    align-items: baseline;
  }
  .legend .num {
    font-weight: 760;
  }
  .majority {
    margin-block-start: 0.5rem;
    color: var(--muted);
  }
  .plain {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.25rem;
  }
  .leaders {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.75rem;
  }
  .leaders li {
    display: flex;
    gap: 0.75rem;
    align-items: center;
  }
  .leaders div {
    display: grid;
  }
  .small {
    font-size: 0.875rem;
  }
  .more {
    margin-block-start: 2.5rem;
    font-weight: 600;
  }
</style>
