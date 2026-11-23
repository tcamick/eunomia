<script lang="ts">
  import { fillCodes, patternId } from '../lib/mapmode';
  import { partyStyle } from '../lib/party';

  // The patterns the maps on a page refer to by id: stripes of two parties
  // for a tie (never a blend of their colours), and a hatch for a vacant
  // seat. Render once per page.
  const token = (code: string) => `var(${code ? partyStyle(code).token : '--muted'})`;
  const pairs = fillCodes.flatMap((a) => fillCodes.filter((b) => b !== a).map((b) => [a, b] as [string, string]));
</script>

<svg width="0" height="0" aria-hidden="true" focusable="false" class="defs">
  <defs>
    {#each pairs as codes (patternId(codes))}
      <pattern id={patternId(codes)} width="10" height="10" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
        <rect width="5" height="10" style:fill={token(codes[0])} />
        <rect x="5" width="5" height="10" style:fill={token(codes[1])} />
      </pattern>
    {/each}
    <pattern id="vacant" width="7" height="7" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <rect width="7" height="7" style:fill="var(--panel)" />
      <rect width="2" height="7" style:fill="var(--vacant)" />
    </pattern>
  </defs>
</svg>

<style>
  .defs {
    position: absolute;
    inline-size: 0;
    block-size: 0;
    overflow: hidden;
  }
</style>
