<script lang="ts">
  import { paint, type Fill } from '../lib/mapmode';

  // A map fill as a small square, for the legend and the list twin. It is
  // decoration: the text beside it always says the same thing.
  let { fill }: { fill: Fill } = $props();
</script>

<svg viewBox="0 0 14 14" width="14" height="14" aria-hidden="true" focusable="false">
  <rect x="0.5" y="0.5" width="13" height="13" rx="2" style:fill={paint(fill)} />
</svg>

<style>
  svg {
    inline-size: 0.95em;
    block-size: 0.95em;
    flex: none;
    vertical-align: -0.1em;
  }
  rect {
    stroke: var(--line);
  }
</style>
