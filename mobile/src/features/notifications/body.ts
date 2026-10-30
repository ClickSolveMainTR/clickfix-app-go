/** Plain notification copy must not expose backend navigation paths. */
export function notificationBody(body: string | null | undefined, billingLabel: string): string {
  return (body ?? '')
    .replace(/(^|\s)\/billing(?=[?#\s.,;!)]|$)(?:[?#][^\s]*)?/g, `$1${billingLabel}`)
    .replace(/(^|\s)\/(?!\/)[a-zA-Z0-9_-][^\s]*/g, '$1')
    .replace(/\s+/g, ' ')
    .trim();
}

export function notificationBodyRoute(body: string | null | undefined): string | null {
  return /(?:^|\s)\/billing(?:[?#\s.,;!)]|$)/.test(body ?? '') ? '/more/billing' : null;
}
