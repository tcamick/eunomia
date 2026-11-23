// Seat positions for a hemicycle chart: concentric half rings, with seats
// spread over each ring in proportion to its length, then numbered left to
// right by angle so a block of seats reads as a wedge.

export interface Seat {
  x: number;
  y: number;
}

export interface Layout {
  seats: Seat[];
  /** r is the drawn radius of one seat. */
  r: number;
  width: number;
  height: number;
}

/** rowsFor picks how many rings n seats need: about 6 for 100, 12 for 441. */
export function rowsFor(n: number): number {
  return Math.max(2, Math.round(Math.sqrt(n) / 1.8));
}

/**
 * hemicycle lays out n seats in a half disc 200 units wide. The inner ring
 * leaves room in the middle for a caption.
 */
export function hemicycle(n: number): Layout {
  const width = 200;
  const outer = 96;
  const inner = outer * 0.42;
  const cx = width / 2;
  const cy = outer + 4;
  if (n <= 0) return { seats: [], r: 0, width, height: cy + 4 };

  const rows = Math.min(rowsFor(n), n);
  const radii = Array.from({ length: rows }, (_, i) => (rows === 1 ? outer : inner + ((outer - inner) * i) / (rows - 1)));
  const total = radii.reduce((a, b) => a + b, 0);
  const counts = radii.map((r) => Math.floor((n * r) / total));
  // Hand the seats lost to rounding to the outer rings, which are longest.
  for (let left = n - counts.reduce((a, b) => a + b, 0), i = rows - 1; left > 0; left--, i = (i - 1 + rows) % rows) {
    counts[i]! += 1;
  }

  const placed: { angle: number; radius: number; seat: Seat }[] = [];
  radii.forEach((radius, row) => {
    const c = counts[row]!;
    for (let i = 0; i < c; i++) {
      const angle = c === 1 ? Math.PI / 2 : Math.PI - (Math.PI * i) / (c - 1);
      placed.push({ angle, radius, seat: { x: cx + radius * Math.cos(angle), y: cy - radius * Math.sin(angle) } });
    }
  });
  placed.sort((a, b) => b.angle - a.angle || a.radius - b.radius);

  const ringGap = rows === 1 ? outer : (outer - inner) / (rows - 1);
  const arcGap = (Math.PI * outer) / Math.max(1, counts[rows - 1]! - 1);
  const r = Math.min(ringGap, arcGap) * 0.4;
  return { seats: placed.map((p) => p.seat), r, width, height: cy + r + 2 };
}
