/** Only a fresh guide may request an atomic server claim. Manual starts bypass this. */
export function canAutoStart(
  status: string,
  enabled: boolean,
  steps: number,
): boolean {
  return enabled && steps > 0 && status === "new";
}
