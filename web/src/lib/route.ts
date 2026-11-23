// The app's pages and how a URL maps to one. Kept apart from the router's
// state so Node's test runner can import it.

export interface Route {
  name: 'congress' | 'map' | 'state' | 'members' | 'person' | 'committees' | 'committee' | 'search' | 'status' | 'notfound';
  params: Record<string, string>;
  query: URLSearchParams;
}

const patterns: [Route['name'], RegExp][] = [
  ['congress', /^\/$/],
  ['map', /^\/map$/],
  ['state', /^\/states\/(?<code>[A-Z]{2})$/],
  ['members', /^\/members$/],
  ['person', /^\/people\/(?<id>[A-Z][0-9]{6})$/],
  ['committees', /^\/committees$/],
  ['committee', /^\/committees\/(?<id>[A-Z]{4}[0-9]{0,2})$/],
  ['search', /^\/search$/],
  ['status', /^\/status$/],
];

/** match turns a path and query into a route. */
export function match(pathname: string, search = ''): Route {
  const path = pathname.length > 1 ? pathname.replace(/\/$/, '') : pathname;
  for (const [name, re] of patterns) {
    const m = re.exec(path);
    if (m) return { name, params: { ...(m.groups ?? {}) }, query: new URLSearchParams(search) };
  }
  return { name: 'notfound', params: {}, query: new URLSearchParams(search) };
}
