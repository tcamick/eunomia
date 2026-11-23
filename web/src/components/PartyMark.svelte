<script lang="ts">
  import Shape from './Shape.svelte';
  import { partyLabel, partyStyle } from '../lib/party';

  // A party's shape in its colour, followed by its name unless hidden. The
  // name is always there for assistive technology.
  let {
    code,
    party = '',
    caucus = '',
    showLabel = true,
  }: { code: string; party?: string; caucus?: string; showLabel?: boolean } = $props();
  const style = $derived(partyStyle(code));
  const label = $derived(partyLabel(party, caucus));
</script>

<span class="mark">
  <svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true" style:--c="var({style.token})">
    <Shape shape={style.shape} cx={5} cy={5} r={4} />
  </svg>
  {#if showLabel}<span>{label}</span>{:else}<span class="visually-hidden">{label}</span>{/if}
</span>

<style>
  .mark {
    display: inline-flex;
    align-items: center;
    gap: 0.4em;
    white-space: nowrap;
  }
  svg {
    inline-size: 0.72em;
    block-size: 0.72em;
    flex: none;
    fill: var(--c);
    color: var(--c);
  }
</style>
