// What the shell hands every page: the route's path parameters and query.

export interface PageProps {
  params: Record<string, string>;
  query: URLSearchParams;
}
