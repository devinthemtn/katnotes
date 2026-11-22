import { test } from 'node:test';
import assert from 'node:assert/strict';
import { toSim, neighbours, radius, linkDistance, showLabel, zoomAt, toGraph, hit, fit, type GNode, type GEdge } from './graph.ts';

const nodes: GNode[] = [
  { key: 'page/a', type: 'note', kind: 'page', id: 'a', title: 'A', degree: 2, dist: 0 },
  { key: 'page/b', type: 'note', kind: 'page', id: 'b', title: 'B', degree: 1, dist: 1 },
  { key: 'tag:x', type: 'tag', title: '#x', degree: 1, dist: 1 },
  { key: 'page/c', type: 'note', kind: 'page', id: 'c', title: 'C', degree: 0, dist: 0 },
];
const edges: GEdge[] = [
  { from: 'page/a', to: 'page/b', type: 'link', count: 3 },
  { from: 'page/a', to: 'tag:x', type: 'tag', count: 1 },
  { from: 'page/a', to: 'page/gone', type: 'link', count: 1 },
];

test('toSim keeps positions and drops dangling edges', () => {
  const first = toSim(nodes, edges);
  assert.equal(first.links.length, 2);
  first.nodes[0].x = 10;
  first.nodes[0].y = 20;
  const prev = new Map(first.nodes.map((n) => [n.key, n]));
  const again = toSim(nodes, edges, prev);
  assert.equal(again.nodes[0].x, 10);
  assert.equal(again.nodes[0].y, 20);
  assert.equal(again.nodes[1].x, undefined);
});

test('neighbours is undirected', () => {
  const nb = neighbours(edges);
  assert.deepEqual([...nb.get('page/a')!].sort(), ['page/b', 'page/gone', 'tag:x']);
  assert.deepEqual([...nb.get('page/b')!], ['page/a']);
});

test('sizes and distances', () => {
  assert.ok(radius({ degree: 9, type: 'note' }) > radius({ degree: 1, type: 'note' }));
  assert.ok(radius({ degree: 1, type: 'tag' }) < radius({ degree: 1, type: 'note' }));
  assert.ok(linkDistance({ type: 'link', count: 3 }) < linkDistance({ type: 'link', count: 1 }));
  assert.equal(linkDistance({ type: 'link', count: 100 }), 30);
});

test('showLabel', () => {
  const { nodes: s } = toSim(nodes, edges);
  const nb = neighbours(edges);
  const [a, b, , c] = s;
  assert.equal(showLabel(c, 1, null, null, nb), false);
  assert.equal(showLabel(c, 2, null, null, nb), true);
  assert.equal(showLabel(b, 1, 'page/a', null, nb), true);
  assert.equal(showLabel(b, 1, null, 'page/a', nb), true);
  assert.equal(showLabel(a, 1, 'page/a', null, nb), true);
  assert.equal(showLabel(c, 1, 'page/a', null, nb), false);
});

test('zoomAt keeps the point under the cursor', () => {
  const v = { x: 100, y: 50, k: 1 };
  const before = toGraph(v, 300, 200);
  const z = zoomAt(v, 300, 200, 2);
  assert.equal(z.k, 2);
  assert.deepEqual(toGraph(z, 300, 200), before);
  assert.equal(zoomAt(v, 0, 0, 1000).k, 5);
  assert.equal(zoomAt(v, 0, 0, 0.0001).k, 0.15);
});

test('hit and fit', () => {
  const { nodes: s } = toSim(nodes, edges);
  s.forEach((n, i) => { n.x = i * 100; n.y = 0; });
  assert.equal(hit(s, 101, 1, 1)?.key, 'page/b');
  assert.equal(hit(s, 50, 0, 1), null);
  const v = fit(s, 800, 400);
  const [gx] = toGraph(v, 400, 200);
  assert.ok(Math.abs(gx - 150) < 5, `centre ${gx}`);
  assert.deepEqual(fit([], 800, 400), { x: 400, y: 200, k: 1 });
});
