<script lang="ts">
  import Shape from './Shape.svelte';
  import type { Chamber, Member } from '../lib/api';
  import { hemicycle } from '../lib/hemicycle';
  import { partyLabel, partyStyle, seatingOrder, vacantStyle } from '../lib/party';
  import { seatName } from '../lib/format';

  // A chamber's voting seats as a hemicycle. Each seat is a link to its
  // member for a pointer; the keyboard and screen readers get the same
  // information from the legend and the member list beside it, so the
  // drawing itself is one image with a text alternative.
  let { chamber, members }: { chamber: Chamber; members: Member[] } = $props();

  interface SeatEntry {
    key: string;
    href?: string;
    title: string;
    token: string;
    shape: 'circle' | 'square' | 'diamond' | 'triangle' | 'ring';
  }

  const entries = $derived.by((): SeatEntry[] => {
    const voting = members.filter((m) => m.body === chamber.body && m.voting);
    const groups = seatingOrder(chamber.parties);
    const byGroup = (party: string, caucus = '') =>
      voting
        .filter((m) => (m.party ?? '') === party && (m.caucus ?? '') === caucus)
        .sort((a, b) => a.stateName.localeCompare(b.stateName) || (a.district ?? 0) - (b.district ?? 0));
    const seat = (m: Member): SeatEntry => {
      const st = partyStyle(m.partyCode);
      return { key: m.id, href: `/people/${m.id}`, title: `${m.name}, ${partyLabel(m.party ?? '', m.caucus)}, ${seatName(m)}`, ...st };
    };
    const vacant = chamber.vacancies
      .filter((v) => v.voting)
      .map((v, i): SeatEntry => ({
        key: `vacant-${i}`,
        title: `Vacant: ${seatName({ body: chamber.body, stateName: v.stateName, district: v.district })}`,
        ...vacantStyle,
      }));
    const left: SeatEntry[] = [];
    const right: SeatEntry[] = [];
    for (const g of groups) {
      const side = g.code === 'R' || g.caucus === 'Republican' ? right : left;
      side.push(...byGroup(g.party, g.caucus).map(seat));
    }
    // Empty seats sit between the blocs.
    return [...left, ...vacant, ...right];
  });

  const layout = $derived(hemicycle(entries.length));
  const caption = $derived(
    `${chamber.body === 'senate' ? 'Senate' : 'House'} seats: ` +
      [...chamber.parties.map((p) => `${p.seats} ${partyLabel(p.party, p.caucus)}`), ...(chamber.vacancies.some((v) => v.voting) ? [`${chamber.vacancies.filter((v) => v.voting).length} vacant`] : [])].join(', ') +
      `, of ${chamber.seats}. A majority is ${chamber.majority}.`,
  );
</script>

<figure>
  <svg viewBox="0 0 {layout.width} {layout.height}" role="img" aria-label={caption}>
    {#each entries as e, i (e.key)}
      {@const p = layout.seats[i]!}
      {#if e.href}
        <a href={e.href} tabindex="-1" aria-hidden="true" style:--c="var({e.token})">
          <title>{e.title}</title>
          <Shape shape={e.shape} cx={p.x} cy={p.y} r={layout.r} />
        </a>
      {:else}
        <g style:--c="var({e.token})" aria-hidden="true">
          <title>{e.title}</title>
          <Shape shape={e.shape} cx={p.x} cy={p.y} r={layout.r} />
        </g>
      {/if}
    {/each}
    <text x={layout.width / 2} y={layout.height - 6} text-anchor="middle" class="total">{chamber.filled}</text>
    <text x={layout.width / 2} y={layout.height - 1} text-anchor="middle" class="of">of {chamber.seats} seats filled</text>
  </svg>
  <figcaption class="visually-hidden">{caption}</figcaption>
</figure>

<style>
  figure {
    margin: 0;
  }
  svg {
    display: block;
    inline-size: 100%;
    block-size: auto;
    overflow: visible;
  }
  a,
  g {
    fill: var(--c);
    color: var(--c);
  }
  @media (prefers-reduced-motion: no-preference) {
    a :global(*) {
      transition: opacity 120ms ease-out;
    }
  }
  svg:has(a:hover) a:not(:hover) {
    opacity: 0.55;
  }
  .total {
    font-size: 14px;
    font-weight: 820;
    fill: var(--fg);
    font-variant-numeric: tabular-nums;
  }
  .of {
    font-size: 4.2px;
    fill: var(--muted);
  }
</style>
