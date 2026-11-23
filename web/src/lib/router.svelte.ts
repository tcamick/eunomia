// A small history router: same-origin links change the page without a
// reload, and the back button works. The server answers every page path
// with the app shell, so any URL can be opened directly.

import { match, type Route } from './route';

class Router {
  route = $state<Route>(match('/'));

  start(): void {
    this.route = match(location.pathname, location.search);
    addEventListener('popstate', () => {
      this.route = match(location.pathname, location.search);
    });
    document.addEventListener('click', (e) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = (e.target as Element | null)?.closest?.('a');
      if (!a || a.target || a.hasAttribute('download') || a.origin !== location.origin) return;
      if (a.pathname.startsWith('/portraits/') || a.pathname.startsWith('/api/')) return;
      e.preventDefault();
      this.go(a.pathname + a.search);
    });
  }

  /** go moves to a same-origin path and puts focus on the new page's heading. */
  go(href: string): void {
    const url = new URL(href, location.origin);
    if (url.pathname + url.search !== location.pathname + location.search) {
      history.pushState(null, '', url.pathname + url.search);
    }
    this.route = match(url.pathname, url.search);
    scrollTo({ top: 0 });
    // After the new page renders, move focus to its heading so a keyboard
    // or screen-reader user starts at the top of what changed.
    requestAnimationFrame(() => document.querySelector<HTMLElement>('main h1')?.focus({ preventScroll: true }));
  }
}

export const router = new Router();
