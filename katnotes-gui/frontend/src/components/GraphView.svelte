<script lang="ts">
  // The link graph: every note (or the notes around the current one) as a
  // force-directed layout on a canvas. Data from NotesService.Graph
  // (internal/notes/graph.go); layout from d3-force; drawing, pan and zoom
  // here; the pure parts in ../graph.ts.
  import { onMount } from 'svelte';
  import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide, forceX, forceY, type Simulation } from 'd3-force';
  import { NotesService as api } from '../../bindings/github.com/devinthemtn/katnotes/katnotes-gui';
  import { toSim, neighbours, linkDistance, showLabel, zoomAt, toGraph, hit, fit, type SimNode, type SimLink, type View, type GEdge } from '../graph';

  let {
    current,
    scope = $bindable(),
    onopen,
    onmissing,
    ontag,
    onclose,
  }: {
    current: string | null; // "kind/id" of the open note
    scope: 'all' | 'local';
    onopen: (kind: string, id: string) => void;
    onmissing: (kind: string, id: string, title: string) => void;
    ontag: (tag: string) => void;
    onclose: () => void;
  } = $props();

  function pref(k: string, d: string): string {
    try { return localStorage.getItem('katnotes.graph.' + k) ?? d; } catch { return d; }
  }
  function setPref(k: string, v: string) {
    try { localStorage.setItem('katnotes.graph.' + k, v); } catch { /* ignore */ }
  }

  let depth = $state(Number(pref('depth', '2')) || 2);
  let tags = $state(pref('tags', '0') === '1');
  let missing = $state(pref('missing', '1') === '1');
  let journal = $state(pref('journal', '1') === '1');
  let err = $state('');
  let counts = $state({ nodes: 0, edges: 0 });

  $effect(() => { setPref('scope', scope); setPref('depth', String(depth)); });
  $effect(() => { setPref('tags', tags ? '1' : '0'); setPref('missing', missing ? '1' : '0'); setPref('journal', journal ? '1' : '0'); });

  let wrap: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let w = 0, h = 0;
  let view: View = { x: 0, y: 0, k: 1 };
  let fitted = false;
  let nodes: SimNode[] = [];
  let links: SimLink[] = [];
  let nb = new Map<string, Set<string>>();
  let hover: string | null = null;
  let sim: Simulation<SimNode, SimLink> | null = null;
  let frame = 0;

  const local = $derived(scope === 'local' && !!current);

  // Reload whenever what's asked for changes.
  $effect(() => {
    const center = local ? current! : '';
    load(center, depth, tags, missing, journal);
  });

  let loadSeq = 0;
  async function load(center: string, d: number, t: boolean, m: boolean, j: boolean) {
    const seq = ++loadSeq;
    try {
      const g = await api.Graph(center, d, t, m, j);
      if (seq !== loadSeq || !g) return;
      err = '';
      const prev = new Map(nodes.map((n) => [n.key, n]));
      const edges = (g.edges ?? []) as GEdge[];
      ({ nodes, links } = toSim((g.nodes ?? []) as any, edges, prev));
      nb = neighbours(edges);
      counts = { nodes: nodes.length, edges: links.length };
      fitted = false;
      start();
    } catch (e) {
      if (seq === loadSeq) err = e && typeof e === 'object' && 'message' in e ? String((e as any).message) : String(e);
    }
  }

  function start() {
    sim?.stop();
    const big = nodes.length > 400;
    sim = forceSimulation<SimNode, SimLink>(nodes)
      .force('link', forceLink<SimNode, SimLink>(links).id((n) => n.key).distance(linkDistance).strength(0.6))
      .force('charge', forceManyBody<SimNode>().strength(big ? -40 : -120).distanceMax(400))
      .force('center', forceCenter(0, 0))
      // Keep unlinked notes from drifting off forever.
      .force('x', forceX<SimNode>(0).strength(0.04))
      .force('y', forceY<SimNode>(0).strength(0.04))
      .force('collide', forceCollide<SimNode>((n) => n.r + 2))
      .alphaDecay(big ? 0.05 : 0.0228)
      .on('tick', () => {
        if (!fitted && sim && sim.alpha() < 0.5) {
          view = fit(nodes, w, h);
          fitted = true;
        }
        redraw();
      });
  }

  function color(name: string, fallback: string) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
  }

  function redraw() {
    if (frame) return;
    frame = requestAnimationFrame(() => { frame = 0; draw(); });
  }

  function draw() {
    const ctx = canvas?.getContext('2d');
    if (!ctx) return;
    const dpr = window.devicePixelRatio || 1;
    const c = {
      bg: color('--bg', '#fff'), text: color('--text', '#222'), muted: color('--muted', '#888'),
      border: color('--border', '#ddd'), accent: color('--accent', '#7d56f4'), info: color('--info', '#3d6fd8'),
      font: color('--font', 'system-ui'),
    };
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = c.bg;
    ctx.fillRect(0, 0, w, h);
    ctx.setTransform(dpr * view.k, 0, 0, dpr * view.k, dpr * view.x, dpr * view.y);

    const focus = hover;
    const lit = (k: string) => !focus || k === focus || !!nb.get(focus)?.has(k);

    // Edges
    for (const l of links) {
      const s = l.source as SimNode, t = l.target as SimNode;
      if (s.x === undefined || t.x === undefined) continue;
      const on = !focus || s.key === focus || t.key === focus;
      ctx.globalAlpha = on ? (focus ? 0.9 : 0.55) : 0.08;
      ctx.strokeStyle = focus && on ? c.accent : c.muted;
      ctx.lineWidth = (l.type === 'tag' ? 0.8 : Math.min(1 + (l.count - 1) * 0.6, 3.5)) / Math.sqrt(view.k);
      ctx.setLineDash(l.type === 'tag' ? [3 / view.k, 3 / view.k] : []);
      ctx.beginPath();
      ctx.moveTo(s.x!, s.y!);
      ctx.lineTo(t.x!, t.y!);
      ctx.stroke();
      if (l.type === 'link' && view.k > 0.6) arrow(ctx, s, t);
    }
    ctx.setLineDash([]);

    // Nodes
    for (const n of nodes) {
      if (n.x === undefined) continue;
      ctx.globalAlpha = lit(n.key) ? 1 : 0.15;
      ctx.beginPath();
      ctx.arc(n.x!, n.y!, n.r, 0, Math.PI * 2);
      if (n.type === 'missing') {
        ctx.fillStyle = c.bg;
        ctx.fill();
        ctx.setLineDash([2, 2]);
        ctx.strokeStyle = c.muted;
        ctx.lineWidth = 1.2;
        ctx.stroke();
        ctx.setLineDash([]);
      } else {
        ctx.fillStyle = n.type === 'tag' ? c.info : n.kind === 'journal' ? c.muted : c.accent;
        ctx.fill();
      }
      if (n.key === current) {
        ctx.beginPath();
        ctx.arc(n.x!, n.y!, n.r + 3.5, 0, Math.PI * 2);
        ctx.strokeStyle = c.accent;
        ctx.lineWidth = 2;
        ctx.stroke();
      }
    }

    // Labels, drawn at screen size.
    ctx.textAlign = 'center';
    ctx.textBaseline = 'top';
    const fs = 12 / view.k;
    ctx.font = `${fs}px ${c.font}`;
    for (const n of nodes) {
      if (n.x === undefined || !showLabel(n, view.k, hover, current, nb)) continue;
      ctx.globalAlpha = lit(n.key) ? 1 : 0.25;
      const label = n.title.length > 40 ? n.title.slice(0, 39) + '…' : n.title;
      const y = n.y! + n.r + 3 / view.k;
      ctx.lineWidth = 3 / view.k;
      ctx.strokeStyle = c.bg;
      ctx.strokeText(label, n.x!, y);
      ctx.fillStyle = n.type === 'missing' ? c.muted : c.text;
      ctx.fillText(label, n.x!, y);
    }
    ctx.globalAlpha = 1;
  }

  function arrow(ctx: CanvasRenderingContext2D, s: SimNode, t: SimNode) {
    const dx = t.x! - s.x!, dy = t.y! - s.y!;
    const len = Math.hypot(dx, dy);
    if (len < t.r * 2) return;
    const ux = dx / len, uy = dy / len;
    const tipX = t.x! - ux * (t.r + 1), tipY = t.y! - uy * (t.r + 1);
    const a = 5 / Math.sqrt(view.k);
    ctx.beginPath();
    ctx.moveTo(tipX, tipY);
    ctx.lineTo(tipX - ux * a - uy * a * 0.5, tipY - uy * a + ux * a * 0.5);
    ctx.lineTo(tipX - ux * a + uy * a * 0.5, tipY - uy * a - ux * a * 0.5);
    ctx.closePath();
    ctx.fillStyle = ctx.strokeStyle;
    ctx.fill();
  }

  // Pointer: drag the background to pan, drag a node to move it, click to open.
  let drag: { node: SimNode | null; sx: number; sy: number; vx: number; vy: number; moved: boolean } | null = null;

  function pos(e: PointerEvent | WheelEvent): [number, number] {
    const r = canvas.getBoundingClientRect();
    return [e.clientX - r.left, e.clientY - r.top];
  }

  function onDown(e: PointerEvent) {
    if (e.button !== 0) return;
    const [sx, sy] = pos(e);
    const [gx, gy] = toGraph(view, sx, sy);
    const node = hit(nodes, gx, gy, view.k);
    drag = { node, sx, sy, vx: view.x, vy: view.y, moved: false };
    canvas.setPointerCapture(e.pointerId);
    if (node) {
      node.fx = node.x;
      node.fy = node.y;
      sim?.alphaTarget(0.2).restart();
    }
  }

  function onMove(e: PointerEvent) {
    const [sx, sy] = pos(e);
    if (drag) {
      if (Math.hypot(sx - drag.sx, sy - drag.sy) > 3) drag.moved = true;
      if (drag.node) {
        const [gx, gy] = toGraph(view, sx, sy);
        drag.node.fx = gx;
        drag.node.fy = gy;
      } else {
        view = { ...view, x: drag.vx + sx - drag.sx, y: drag.vy + sy - drag.sy };
      }
      redraw();
      return;
    }
    const [gx, gy] = toGraph(view, sx, sy);
    const k = hit(nodes, gx, gy, view.k)?.key ?? null;
    if (k !== hover) {
      hover = k;
      canvas.style.cursor = k ? 'pointer' : 'grab';
      redraw();
    }
  }

  function onUp(e: PointerEvent) {
    const d = drag;
    drag = null;
    if (!d) return;
    if (d.node) {
      d.node.fx = null;
      d.node.fy = null;
      sim?.alphaTarget(0);
      if (!d.moved) activate(d.node);
    }
    canvas.releasePointerCapture?.(e.pointerId);
  }

  function activate(n: SimNode) {
    if (n.type === 'tag') ontag(n.key.replace(/^tag:/, ''));
    else if (n.type === 'missing') onmissing(n.kind!, n.id!, n.title);
    else onopen(n.kind!, n.id!);
  }

  function onWheel(e: WheelEvent) {
    e.preventDefault();
    const [sx, sy] = pos(e);
    view = zoomAt(view, sx, sy, Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0015)));
    redraw();
  }

  function zoomBy(f: number) {
    view = zoomAt(view, w / 2, h / 2, f);
    redraw();
  }

  function refit() {
    view = fit(nodes, w, h);
    redraw();
  }

  onMount(() => {
    const ro = new ResizeObserver(() => {
      const r = wrap.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      if (!w) view = { x: r.width / 2, y: r.height / 2, k: 1 };
      w = r.width;
      h = r.height;
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      canvas.style.width = w + 'px';
      canvas.style.height = h + 'px';
      redraw();
    });
    ro.observe(wrap);
    // Theme changes set CSS variables on <html>: redraw with the new colours.
    const mo = new MutationObserver(redraw);
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['style', 'class'] });
    return () => {
      ro.disconnect();
      mo.disconnect();
      sim?.stop();
      if (frame) cancelAnimationFrame(frame);
    };
  });

  // Redraw when the current note changes (its ring moves).
  $effect(() => { void current; redraw(); });
</script>

<section class="graph-view">
  <header class="tasks-head">
    <h2>Graph <span class="tasks-total">{counts.nodes} notes · {counts.edges} links</span></h2>
    <div class="seg">
      <button class:active={scope === 'all'} onclick={() => (scope = 'all')}>All notes</button>
      <button class:active={scope === 'local'} onclick={() => (scope = 'local')} disabled={!current} title={current ? 'Notes around the open note' : 'Open a note first'}>Around this note</button>
    </div>
    {#if scope === 'local'}
      <div class="seg" title="Links away from the note">
        {#each [1, 2, 3] as d (d)}
          <button class:active={depth === d} onclick={() => (depth = d)}>{d}</button>
        {/each}
      </div>
    {/if}
    <label class="graph-toggle"><input type="checkbox" bind:checked={tags} /> Tags</label>
    <label class="graph-toggle"><input type="checkbox" bind:checked={missing} /> Missing notes</label>
    <label class="graph-toggle"><input type="checkbox" bind:checked={journal} /> Journal</label>
    <span class="graph-spacer"></span>
    <div class="seg">
      <button onclick={() => zoomBy(1 / 1.3)} title="Zoom out">−</button>
      <button onclick={refit} title="Fit everything">Fit</button>
      <button onclick={() => zoomBy(1.3)} title="Zoom in">+</button>
    </div>
    <button class="link" onclick={onclose} title="Esc">Back to note</button>
  </header>
  {#if err}<p class="graph-msg error">{err}</p>{/if}
  {#if !err && counts.nodes === 0}<p class="graph-msg">No notes to show.</p>{/if}
  <div class="graph-canvas" bind:this={wrap}>
    <canvas
      bind:this={canvas}
      onpointerdown={onDown}
      onpointermove={onMove}
      onpointerup={onUp}
      onpointerleave={() => { if (!drag && hover) { hover = null; redraw(); } }}
      onwheel={onWheel}
      style="cursor: grab"
    ></canvas>
  </div>
  <footer class="graph-legend">
    <span><i class="dot note"></i>page</span>
    <span><i class="dot journal"></i>journal</span>
    {#if tags}<span><i class="dot tag"></i>tag</span>{/if}
    {#if missing}<span><i class="dot missing"></i>not written yet</span>{/if}
    <span class="graph-hint">click opens · drag to move · scroll to zoom</span>
  </footer>
</section>

<style>
  .graph-view { flex: 1; display: flex; flex-direction: column; min-height: 0; position: relative; }
  .graph-canvas { flex: 1; min-height: 0; position: relative; overflow: hidden; }
  .graph-canvas canvas { position: absolute; inset: 0; touch-action: none; }
  .graph-toggle { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; color: var(--muted); cursor: pointer; }
  .graph-spacer { flex: 1; }
  .graph-msg { position: absolute; top: 70px; left: 0; right: 0; text-align: center; color: var(--muted); pointer-events: none; z-index: 1; }
  .graph-msg.error { color: var(--danger); }
  .graph-legend {
    display: flex; gap: 14px; flex-wrap: wrap; align-items: center;
    padding: 6px 20px; border-top: 1px solid var(--border); font-size: 12px; color: var(--muted);
  }
  .graph-hint { margin-left: auto; }
  .dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 5px; vertical-align: -1px; }
  .dot.note { background: var(--accent); }
  .dot.journal { background: var(--muted); }
  .dot.tag { background: var(--info); }
  .dot.missing { border: 1px dashed var(--muted); }
</style>
