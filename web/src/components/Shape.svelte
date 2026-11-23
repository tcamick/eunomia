<script lang="ts">
  import type { Shape } from '../lib/party';

  // One party shape, drawn inside an SVG the caller provides, centred on
  // (cx, cy) with radius r. Its colour comes from the caller's fill.
  let { shape, cx, cy, r }: { shape: Shape; cx: number; cy: number; r: number } = $props();
  const s = $derived(r * 0.86);
</script>

{#if shape === 'circle'}
  <circle {cx} {cy} {r} />
{:else if shape === 'square'}
  <rect x={cx - s} y={cy - s} width={2 * s} height={2 * s} rx={r * 0.12} />
{:else if shape === 'diamond'}
  <polygon points="{cx},{cy - r * 1.08} {cx + r * 1.08},{cy} {cx},{cy + r * 1.08} {cx - r * 1.08},{cy}" />
{:else if shape === 'triangle'}
  <polygon points="{cx},{cy - r * 1.05} {cx + r * 1.05},{cy + r * 0.8} {cx - r * 1.05},{cy + r * 0.8}" />
{:else if shape === 'box'}
  <rect x={cx - s * 0.82} y={cy - s * 0.82} width={2 * s * 0.82} height={2 * s * 0.82} fill="none" stroke="currentColor" stroke-width={r * 0.34} />
{:else}
  <circle {cx} {cy} r={r * 0.78} fill="none" stroke="currentColor" stroke-width={r * 0.38} />
{/if}
