<script lang="ts">
  // A member's portrait, mirrored to this server by the portraits job. A
  // member without one gets their initials, never a placeholder face.
  // lazy is false for a portrait at the top of a page, which is seen at once.
  let { id, name, has, size = 3, lazy = true }: { id: string; name: string; has: boolean; size?: number; lazy?: boolean } =
    $props();
  const initials = $derived(
    name
      .split(/\s+/)
      .filter((w) => /^[A-Z]/.test(w))
      .map((w) => w[0])
      .filter((_, i, a) => i === 0 || i === a.length - 1)
      .join(''),
  );
</script>

{#if has}
  <img src="/portraits/{id}.jpg" alt={name} width="225" height="275" loading={lazy ? 'lazy' : 'eager'} style:--size="{size}rem" />
{:else}
  <span class="initials" role="img" aria-label={name} style:--size="{size}rem">{initials}</span>
{/if}

<style>
  img,
  .initials {
    inline-size: var(--size);
    block-size: calc(var(--size) * 275 / 225);
    border-radius: var(--radius-s);
    flex: none;
    background: var(--sunk);
  }
  img {
    object-fit: cover;
  }
  .initials {
    display: grid;
    place-items: center;
    color: var(--muted);
    font-weight: 700;
    font-size: calc(var(--size) * 0.36);
    letter-spacing: 0.02em;
  }
</style>
