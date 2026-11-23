// Links taken from data are rendered only when they are https
// (docs/web-conventions.md, External links).

/** httpsOnly returns url when it is an absolute https URL, and '' otherwise. */
export function httpsOnly(url: string | undefined): string {
  try {
    return new URL(url ?? '').protocol === 'https:' ? (url ?? '') : '';
  } catch {
    return '';
  }
}
