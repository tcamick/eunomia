<script lang="ts">
  import type { PageProps } from '../lib/page';

  // A path no page answers, or a person or committee the roster does not have.
  let { what = 'page' }: Partial<PageProps> & { what?: string } = $props();
  document.title = 'Not found · Eunomia';
</script>

<h1 tabindex="-1">Not found</h1>
<p class="lede">There is no {what} at this address. It may have been mistyped, or the roster may no longer list it.</p>
<ul>
  <li><a href="/">Congress</a></li>
  <li><a href="/members">Every member</a></li>
  <li><a href="/search">Search</a></li>
</ul>

<style>
  .lede {
    margin-block: 0.75rem 1rem;
  }
</style>
