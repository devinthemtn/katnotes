// Pure helpers for the graph view (GraphView.svelte): turning the Go graph
// into simulation nodes and links, sizing, neighbour highlighting and the
// pan/zoom transform. Kept free of the DOM so node --test can run them.

export type NodeType = 'note' | 'tag' | 'missing';

export interface GNode {
  key: string;
  type: string;
  kind?: string;
  id?: string;
  title: string;
  degree: number;
  dist: number;
}

export interface GEdge {
  from: string;
  to: string;
  type: string;
  count: number;
}

export interface SimNode extends GNode {
  x?: number;
  y?: number;
  vx?: number;
  vy?: number;
  fx?: number | null;
  fy?: number | null;
  r: number;
}

export interface SimLink {
  source: SimNode | string;
  target: SimNode | string;
  type: string;
  count: number;
}

export function radius(n: { degree: number; type: string }): number {
  const r = 4 + Math.sqrt(n.degree) * 2.2;
  return n.type === 'tag' ? r * 0.85 : r;
}

// toSim builds simulation nodes and links, keeping the positions of nodes
// that were already on screen so a refresh doesn't scramble the layout.
export function toSim(
  nodes: GNode[],
  edges: GEdge[],
  prev: Map<string, SimNode> = new Map(),
): { nodes: SimNode[]; links: SimLink[] } {
  const out = nodes.map((n): SimNode => {
    const p = prev.get(n.key);
    return { ...n, r: radius(n), x: p?.x, y: p?.y, vx: p?.vx, vy: p?.vy };
  });
  const keys = new Set(out.map((n) => n.key));
  const links = edges
    .filter((e) => keys.has(e.from) && keys.has(e.to))
    .map((e) => ({ source: e.from, target: e.to, type: e.type, count: e.count }));
  return { nodes: out, links };
}

// neighbours maps each key to the keys it shares an edge with.
export function neighbours(edges: GEdge[]): Map<string, Set<string>> {
  const m = new Map<string, Set<string>>();
  const add = (a: string, b: string) => {
    let s = m.get(a);
    if (!s) m.set(a, (s = new Set()));
    s.add(b);
  };
  for (const e of edges) {
    add(e.from, e.to);
    add(e.to, e.from);
  }
  return m;
}

// linkDistance: notes linked many times sit closer; tags sit a bit further out.
export function linkDistance(l: { type: string; count: number }): number {
  if (l.type === 'tag') return 70;
  return Math.max(30, 60 - 6 * (l.count - 1));
}

// showLabel: labels for the hovered node and its neighbours, the current
// note and its neighbours, every node once zoomed in, and big hubs always.
export function showLabel(
  n: SimNode,
  scale: number,
  hover: string | null,
  current: string | null,
  nb: Map<string, Set<string>>,
): boolean {
  if (scale >= 1.4 || n.degree >= 8) return true;
  for (const focus of [hover, current]) {
    if (!focus) continue;
    if (n.key === focus || nb.get(focus)?.has(n.key)) return true;
  }
  return false;
}

export interface View {
  x: number; // screen offset of graph origin
  y: number;
  k: number; // scale
}

export const MIN_ZOOM = 0.15;
export const MAX_ZOOM = 5;

// zoomAt scales the view by factor around the screen point (sx, sy), so that
// point stays under the cursor.
export function zoomAt(v: View, sx: number, sy: number, factor: number): View {
  const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, v.k * factor));
  const f = k / v.k;
  return { k, x: sx - (sx - v.x) * f, y: sy - (sy - v.y) * f };
}

export function toGraph(v: View, sx: number, sy: number): [number, number] {
  return [(sx - v.x) / v.k, (sy - v.y) / v.k];
}

// hit returns the node under graph point (gx, gy), preferring the top-most
// (last drawn), with a little slack for small nodes.
export function hit(nodes: SimNode[], gx: number, gy: number, k: number): SimNode | null {
  const slack = 3 / k;
  for (let i = nodes.length - 1; i >= 0; i--) {
    const n = nodes[i];
    if (n.x === undefined || n.y === undefined) continue;
    const dx = n.x - gx, dy = n.y - gy, r = n.r + slack;
    if (dx * dx + dy * dy <= r * r) return n;
  }
  return null;
}

// fit returns a view that shows every node inside a w×h canvas.
export function fit(nodes: SimNode[], w: number, h: number, pad = 40): View {
  const pts = nodes.filter((n) => n.x !== undefined && n.y !== undefined);
  if (!pts.length) return { x: w / 2, y: h / 2, k: 1 };
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const n of pts) {
    x0 = Math.min(x0, n.x! - n.r); y0 = Math.min(y0, n.y! - n.r);
    x1 = Math.max(x1, n.x! + n.r); y1 = Math.max(y1, n.y! + n.r);
  }
  const k = Math.min(2, Math.max(MIN_ZOOM, Math.min((w - 2 * pad) / Math.max(x1 - x0, 1), (h - 2 * pad) / Math.max(y1 - y0, 1))));
  return { k, x: w / 2 - ((x0 + x1) / 2) * k, y: h / 2 - ((y0 + y1) / 2) * k };
}
