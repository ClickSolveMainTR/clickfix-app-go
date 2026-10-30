/** Clipboard revisions are metadata only: never read image bytes to decide whether to offer. */
export type ClipboardOffer = { revision: number; consumed: number; hasImage: boolean };
export const initialClipboardOffer: ClipboardOffer = { revision: 0, consumed: -1, hasImage: false };
export function clipboardChanged(state: ClipboardOffer, hasImage: boolean): ClipboardOffer {
  return { ...state, revision: state.revision + 1, hasImage };
}
export function shouldOfferClipboard(state: ClipboardOffer, focused: boolean) {
  return focused && state.hasImage && state.consumed !== state.revision;
}
export function consumeClipboard(state: ClipboardOffer, revision = state.revision): ClipboardOffer {
  return { ...state, consumed: revision };
}
export type Point = { x: number; y: number };
export type Stroke = {
  id: string;
  tool: 'pen' | 'highlighter' | 'arrow' | 'rectangle' | 'text';
  color: string;
  width: number;
  points: Point[];
  text?: string;
};
/** Stable versioned format, independent of screen size (coordinates in image pixels). */
export function serializeStrokes(strokes: Stroke[]) {
  return JSON.stringify({ version: 1, strokes });
}
export function deserializeStrokes(value: string): Stroke[] {
  const data = JSON.parse(value);
  if (data.version !== 1 || !Array.isArray(data.strokes)) throw new Error('Invalid strokes');
  const tools = ['pen', 'highlighter', 'arrow', 'rectangle', 'text'];
  for (const s of data.strokes) {
    if (
      !s ||
      typeof s.id !== 'string' ||
      !tools.includes(s.tool) ||
      typeof s.color !== 'string' ||
      !Number.isFinite(s.width) ||
      s.width <= 0 ||
      !Array.isArray(s.points) ||
      !s.points.length ||
      s.points.some((p: Point) => !p || !Number.isFinite(p.x) || !Number.isFinite(p.y)) ||
      (s.text !== undefined && typeof s.text !== 'string')
    )
      throw new Error('Invalid stroke');
  }
  return data.strokes;
}
export function strokePath(s: Stroke) {
  const first = s.points[0];
  const last = s.points[s.points.length - 1];
  if (!first || !last) return '';
  if (s.tool === 'rectangle') return `M${first.x},${first.y} H${last.x} V${last.y} H${first.x} Z`;
  const line = s.points.map((p, i) => `${i ? 'L' : 'M'}${p.x},${p.y}`).join(' ');
  if (s.tool !== 'arrow') return s.points.length === 1 ? `${line} l0.1,0.1` : line;
  const angle = Math.atan2(last.y - first.y, last.x - first.x);
  const length = s.width * 5;
  return `M${first.x},${first.y} L${last.x},${last.y} M${last.x - length * Math.cos(angle - 0.5)},${last.y - length * Math.sin(angle - 0.5)} L${last.x},${last.y} L${last.x - length * Math.cos(angle + 0.5)},${last.y - length * Math.sin(angle + 0.5)}`;
}
function segmentDistance(p: Point, a: Point, b: Point) {
  const dx = b.x - a.x,
    dy = b.y - a.y;
  const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / (dx * dx + dy * dy || 1)));
  return Math.hypot(p.x - a.x - t * dx, p.y - a.y - t * dy);
}
export function hitsStroke(s: Stroke, p: Point, radius: number) {
  const a = s.points[0],
    b = s.points[s.points.length - 1];
  if (!a || !b) return false;
  if (s.tool === 'text')
    return (
      p.x >= a.x - radius &&
      p.x <= a.x + (s.text?.length ?? 1) * s.width * 4 &&
      Math.abs(p.y - a.y) < s.width * 8
    );
  const points =
    s.tool === 'rectangle'
      ? [a, { x: b.x, y: a.y }, b, { x: a.x, y: b.y }, a]
      : s.tool === 'arrow'
        ? [a, b]
        : s.points;
  if (points.length === 1) return Math.hypot(p.x - a.x, p.y - a.y) <= radius + s.width / 2;
  return points.slice(1).some((b, i) => segmentDistance(p, points[i], b) <= radius + s.width / 2);
}
