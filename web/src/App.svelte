<script lang="ts">
  import Committee from './pages/Committee.svelte';
  import Committees from './pages/Committees.svelte';
  import Congress from './pages/Congress.svelte';
  import MapPage from './pages/Map.svelte';
  import State from './pages/State.svelte';
  import Members from './pages/Members.svelte';
  import NotFound from './pages/NotFound.svelte';
  import Person from './pages/Person.svelte';
  import Search from './pages/Search.svelte';
  import Status from './pages/Status.svelte';
  import { router } from './lib/router.svelte';
  import type { Route } from './lib/route';

  // The shell: a top bar with the sections, the current page, and a footer.
  // Each page is keyed by its path, so moving between two people remounts
  // the page and starts its request afresh.
  const pages = {
    congress: Congress,
    map: MapPage,
    state: State,
    members: Members,
    person: Person,
    committees: Committees,
    committee: Committee,
    search: Search,
    status: Status,
    notfound: NotFound,
  } satisfies Record<Route['name'], unknown>;

  const sections: { href: string; label: string; names: Route['name'][] }[] = [
    { href: '/', label: 'Congress', names: ['congress'] },
    { href: '/map', label: 'Map', names: ['map', 'state'] },
    { href: '/members', label: 'Members', names: ['members', 'person'] },
    { href: '/committees', label: 'Committees', names: ['committees', 'committee'] },
    { href: '/search', label: 'Search', names: ['search'] },
  ];

  const route = $derived(router.route);
  const Page = $derived(pages[route.name]);
  const key = $derived(`${route.name}/${route.params['id'] ?? route.params['code'] ?? ''}`);
</script>

<a class="skip" href="#main">Skip to the page</a>

<header class="top">
  <a class="brand" href="/">Eunomia</a>
  <nav aria-label="Sections">
    <ul>
      {#each sections as s (s.href)}
        <li>
          <a href={s.href} aria-current={s.names.includes(route.name) ? 'page' : undefined}>{s.label}</a>
        </li>
      {/each}
    </ul>
  </nav>
</header>

<main id="main">
  {#key key}
    <Page params={route.params} query={route.query} />
  {/key}
</main>

<footer class="bottom">
  <a href="/status" aria-current={route.name === 'status' ? 'page' : undefined}>Status</a>
</footer>

<style>
  .skip {
    position: absolute;
    inset-inline-start: 1rem;
    inset-block-start: -10rem;
    padding: 0.5rem 0.75rem;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    z-index: 1;
  }
  .skip:focus-visible {
    inset-block-start: 0.5rem;
  }
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem 2rem;
    padding-block: 0.5rem;
    padding-inline: max(1rem, env(safe-area-inset-left)) max(1rem, env(safe-area-inset-right));
    border-block-end: 1px solid var(--line);
    background: var(--panel);
  }
  .brand {
    color: var(--fg);
    font-weight: 820;
    letter-spacing: 0.01em;
    text-decoration: none;
  }
  nav ul {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.25rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  nav a {
    display: inline-flex;
    align-items: center;
    min-block-size: 2.25rem;
    padding-inline: 0.625rem;
    color: var(--fg);
    text-decoration: none;
    border-block-end: 2px solid transparent;
  }
  nav a:hover {
    border-block-end-color: var(--line);
  }
  nav a[aria-current='page'] {
    font-weight: 700;
    border-block-end-color: var(--accent);
  }
  @media (pointer: coarse) {
    nav a {
      min-block-size: 2.75rem;
    }
  }
  main {
    max-inline-size: 72rem;
    margin-inline: auto;
    padding-block: 2rem 1rem;
    padding-inline: max(1rem, env(safe-area-inset-left)) max(1rem, env(safe-area-inset-right));
  }
  .bottom {
    max-inline-size: 72rem;
    margin-inline: auto;
    padding: 2rem 1rem max(2rem, env(safe-area-inset-bottom));
    color: var(--muted);
    font-size: 0.875rem;
  }
</style>
