// A small history router: same-origin links change the page without a
// reload, and the back button works. The server answers every page path
// with the app shell, so any URL can be opened directly.

import { match, type Route } from './route';

class Router {
  route = $state<Route>(match('/'));
  #watch: MutationObserver | null = null;

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
    requestAnimationFrame(() => {
      document.querySelector<HTMLElement>('main h1')?.focus({ preventScroll: true });
      this.#keepHeadingFocus();
    });
  }

  /**
   * A page that loads its data first shows a loading heading, then replaces
   * it; focus on the removed heading would fall to <body>, and the next Tab
   * would start again at the skip link. For a few seconds after a
   * navigation, a heading that replaces the focused one takes the focus.
   */
  #keepHeadingFocus(): void {
    this.#watch?.disconnect();
    const main = document.querySelector('main');
    if (!main) return;
    const watch = new MutationObserver(() => {
      if (document.activeElement && document.activeElement !== document.body) return;
      main.querySelector<HTMLElement>('h1')?.focus({ preventScroll: true });
    });
    watch.observe(main, { childList: true, subtree: true });
    this.#watch = watch;
    setTimeout(() => {
      if (this.#watch === watch) {
        watch.disconnect();
        this.#watch = null;
      }
    }, 5000);
  }
}

export const router = new Router();
