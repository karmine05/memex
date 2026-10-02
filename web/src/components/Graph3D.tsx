import { useRef, useEffect, useCallback } from 'react';
import type { GraphData, TelemetryActivity } from '../types';

type Node3D = {
  id: string;
  name: string;
  status: string;
  pos: [number, number, number];
  excite: number;
  base: number;
  exciteColor: [number, number, number] | null;
};

type Edge3D = {
  from: string;
  to: string;
  kind: string;
  count: number;
  act: number;
  rest: number;
};

type Flow = {
  type: 'hub' | 'edge' | 'amb';
  nodeId?: string;
  eo?: Edge3D;
  P0: [number, number, number];
  P1: [number, number, number];
  P2: [number, number, number];
  P3: [number, number, number];
  color: [number, number, number];
  t: number;
  speed: number;
  burst: number;
};

const KIND_COLOR: Record<string, [number, number, number]> = {
  used: [0.40, 0.68, 1.0],
  sent: [1.0, 0.62, 0.22],
  cited: [0.32, 0.90, 0.55],
  wrote: [0.32, 0.90, 0.55],
};

const HUB_COLOR: Record<'read' | 'write', [number, number, number]> = {
  read: [0.40, 0.75, 1.00],
  write: [0.30, 0.90, 0.55],
};

const KIND_SPEED: Record<string, number> = {
  used: 0.16,
  sent: 0.38,
  cited: 0.24,
  wrote: 0.2,
};

const TAU = Math.PI * 2;
const RW = [9, 7, 8.5];

// Seeded random for deterministic star positions
function rand(n: number): number {
  const x = Math.sin(n * 127.1 + 311.7) * 43758.5453;
  return x - Math.floor(x);
}

function nrm(v: [number, number, number]): [number, number, number] {
  const l = Math.hypot(v[0], v[1], v[2]) || 1;
  return [v[0] / l, v[1] / l, v[2] / l];
}

function cross(a: [number, number, number], b: [number, number, number]): [number, number, number] {
  return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
}

function clamp(x: number, a: number, b: number): number {
  return x < a ? a : x > b ? b : x;
}

function cubic(A: [number, number, number], B: [number, number, number], C: [number, number, number], D: [number, number, number], u: number): [number, number, number] {
  const w0 = (1 - u) * (1 - u) * (1 - u), w1 = 3 * (1 - u) * (1 - u) * u, w2 = 3 * (1 - u) * u * u, w3 = u * u * u;
  return [
    w0 * A[0] + w1 * B[0] + w2 * C[0] + w3 * D[0],
    w0 * A[1] + w1 * B[1] + w2 * C[1] + w3 * D[1],
    w0 * A[2] + w1 * B[2] + w2 * C[2] + w3 * D[2],
  ];
}

export function Graph3D({ graph, activity, onNodeClick }: {
  graph: GraphData | null;
  activity: TelemetryActivity[];
  onNodeClick?: (id: string) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const stateRef = useRef<{
    nodes: Map<string, Node3D>;
    edges: Edge3D[];
    flows: Flow[];
    screenPos: Map<string, [number, number, number]>;
    cam: { az: number; el: number; r: number };
    camT: { az: number; el: number; r: number };
    dragging: boolean;
    lx: number;
    ly: number;
    hoverID: string | null;
    selectID: string | null;
    selectUntil: number;
    coreFlash: number;
    globalBoost: number;
    waves: { t0: number }[];
    shocks: { id: string; t0: number }[];
    seenIds: Set<string>;
    animationId: number;
    lastT: number;
  }>({
    nodes: new Map(),
    edges: [],
    flows: [],
    screenPos: new Map(),
    cam: { az: 0.85, el: 0.12, r: 48 },
    camT: { az: 0.85, el: 0.12, r: 31 },
    dragging: false,
    lx: 0,
    ly: 0,
    hoverID: null,
    selectID: null,
    selectUntil: 0,
    coreFlash: 0,
    globalBoost: 0,
    waves: [],
    shocks: [],
    seenIds: new Set(),
    animationId: 0,
    lastT: 0,
  });

  const handleMouseDown = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const s = stateRef.current;
    s.dragging = true;
    s.lx = e.clientX;
    s.ly = e.clientY;

    const canvas = canvasRef.current;
    if (!canvas) return;

    // Check for node click
    const rect = canvas.getBoundingClientRect();
    const mx = (e.clientX - rect.left) * (canvas.width / rect.width);
    const my = (e.clientY - rect.top) * (canvas.height / rect.height);
    let best: string | null = null;
    let bd = 1e9;
    for (const [id, sp] of s.screenPos) {
      const dd = Math.hypot(sp[0] - mx, sp[1] - my);
      if (dd < 30 && dd < bd) { bd = dd; best = id; }
    }
    if (best) {
      s.selectID = best;
      s.selectUntil = performance.now() + 4000;
      onNodeClick?.(best);
    }
  }, [onNodeClick]);

  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const s = stateRef.current;
    if (s.dragging) {
      s.camT.az -= (e.clientX - s.lx) * 0.005;
      s.camT.el = clamp(s.camT.el + (e.clientY - s.ly) * 0.005, -1.25, 1.25);
      s.lx = e.clientX;
      s.ly = e.clientY;
    }
  }, []);

  const handleMouseUp = useCallback(() => {
    stateRef.current.dragging = false;
  }, []);

  const handleWheel = useCallback((e: React.WheelEvent<HTMLCanvasElement>) => {
    const s = stateRef.current;
    s.camT.r = clamp(s.camT.r + e.deltaY * 0.02, 16, 58);
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const s = stateRef.current;
    const DPR = Math.min(window.devicePixelRatio || 1, 2);
    const RM = matchMedia('(prefers-reduced-motion: reduce)').matches;

    function resize() {
      const rect = canvas!.getBoundingClientRect();
      canvas!.width = Math.round(rect.width * DPR);
      canvas!.height = Math.round(rect.height * DPR);
      canvas!.style.width = rect.width + 'px';
      canvas!.style.height = rect.height + 'px';
    }

    // Generate stars once
    const stars: [number, number, number, number, number][] = [];
    for (let i = 0; i < 420; i++) {
      const u = rand(i + 900) * 2 - 1, ph = rand(i + 700) * TAU;
      const r = 48 + rand(i + 500) * 38, s2 = Math.sqrt(1 - u * u);
      stars.push([r * s2 * Math.cos(ph) * 1.35, r * u * 1.1, r * s2 * Math.sin(ph) * 1.35, 0.4 + rand(i) * 1.1, rand(i + 300) * TAU]);
    }

    s.lastT = performance.now();

    function updateLayout() {
      const nodes = graph?.nodes ?? [];
      const edges = graph?.edges ?? [];

      // Layout nodes on a fibonacci sphere
      s.nodes.clear();
      const n = nodes.length;
      const golden = Math.PI * (3 - Math.sqrt(5));
      nodes.forEach((nd, i) => {
        const y = n === 1 ? 0 : 1 - (i / (n - 1)) * 2;
        const r = Math.sqrt(Math.max(0, 1 - y * y));
        const th = golden * i + rand(i) * 0.35;
        const x = Math.cos(th) * r * RW[0];
        const yy = y * RW[1];
        const z = Math.sin(th) * r * RW[2];

        const existing = s.nodes.get(nd.id);
        s.nodes.set(nd.id, {
          id: nd.id,
          name: nd.name,
          status: nd.status,
          pos: [x, yy, z],
          excite: existing?.excite ?? 1.8,
          base: existing?.base ?? 0,
          exciteColor: existing?.exciteColor ?? null,
        });

        if (!s.seenIds.has(nd.id)) {
          s.seenIds.add(nd.id);
          s.shocks.push({ id: nd.id, t0: performance.now() });
        }
      });

      // Clean up dead nodes
      const live = new Set(nodes.map(nd => nd.id));
      for (const id of [...s.nodes.keys()]) {
        if (!live.has(id)) s.nodes.delete(id);
      }

      // Build edges
      const old = new Map(s.edges.map(e => [e.from + '|' + e.to + '|' + e.kind, e]));
      s.edges = edges.map(e => {
        const prev = old.get(e.from + '|' + e.to + '|' + (e.kind || 'used'));
        return {
          from: e.from,
          to: e.to,
          kind: e.kind || 'used',
          count: e.count || 1,
          act: prev?.act ?? 0,
          rest: prev?.rest ?? 0,
        };
      });

      // Build flows
      buildFlows();
    }

    function buildFlows() {
      s.flows = [];

      // Hub flows: core <-> each agent
      for (const nd of s.nodes.values()) {
        const P = nd.pos;
        const L = Math.hypot(P[0], P[1], P[2]) || 1;
        const dir: [number, number, number] = [P[0] / L, P[1] / L, P[2] / L];
        const ref: [number, number, number] = Math.abs(dir[1]) > 0.9 ? [1, 0, 0] : [0, 1, 0];
        const u1 = nrm(cross(dir, ref));
        const u2 = cross(dir, u1);
        const i = [...s.nodes.keys()].indexOf(nd.id);
        for (let j = 0; j < 3; j++) {
          const ang = (j / 3) * TAU + rand(i * 31 + j * 7 + 3) * 0.9;
          const px = u1[0] * Math.cos(ang) + u2[0] * Math.sin(ang);
          const py = u1[1] * Math.cos(ang) + u2[1] * Math.sin(ang);
          const pz = u1[2] * Math.cos(ang) + u2[2] * Math.sin(ang);
          const spread = L * (0.10 + rand(i * 31 + j * 13 + 5) * 0.12);
          const toCore = j === 1;
          s.flows.push({
            type: 'hub',
            nodeId: nd.id,
            P0: toCore ? P.slice() as [number, number, number] : [0, 0, 0],
            P1: [dir[0] * L * 0.35 + px * spread, dir[1] * L * 0.35 + py * spread, dir[2] * L * 0.35 + pz * spread],
            P2: [dir[0] * L * 0.80 + px * spread * 1.8, dir[1] * L * 0.80 + py * spread * 1.8, dir[2] * L * 0.80 + pz * spread * 1.8],
            P3: toCore ? [0, 0, 0] : P.slice() as [number, number, number],
            color: toCore ? HUB_COLOR.write : HUB_COLOR.read,
            t: rand(i * 31 + j * 17 + 9),
            speed: (toCore ? 0.16 : 0.13) + rand(i * 31 + j * 23 + 11) * 0.07,
            burst: 0,
          });
        }
      }

      // Edge flows: agent <-> agent along graph edges
      for (const eo of s.edges) {
        const A = s.nodes.get(eo.from)?.pos, B = s.nodes.get(eo.to)?.pos;
        if (!A || !B) continue;
        const mid: [number, number, number] = [(A[0] + B[0]) / 2, (A[1] + B[1]) / 2, (A[2] + B[2]) / 2];
        const nrmv = nrm(cross([B[0] - A[0], B[1] - A[1], B[2] - A[2]], [0, 1, 0.3]));
        const off = (Math.hypot(B[0] - A[0], B[1] - A[1], B[2] - A[2]) * 0.18 + 0.8) * (rand(s.edges.indexOf(eo) * 61) > 0.5 ? 1 : -1);
        const C: [number, number, number] = [mid[0] + nrmv[0] * off, mid[1] + nrmv[1] * off, mid[2] + nrmv[2] * off];
        s.flows.push({
          type: 'edge',
          eo,
          P0: A.slice() as [number, number, number],
          P1: [A[0] + (C[0] - A[0]) * 2 / 3, A[1] + (C[1] - A[1]) * 2 / 3, A[2] + (C[2] - A[2]) * 2 / 3],
          P2: [B[0] + (C[0] - B[0]) * 2 / 3, B[1] + (C[1] - B[1]) * 2 / 3, B[2] + (C[2] - B[2]) * 2 / 3],
          P3: B.slice() as [number, number, number],
          color: KIND_COLOR[eo.kind] || KIND_COLOR.used,
          t: rand((eo.from.length + eo.to.length) * 7 + eo.kind.length * 13 + eo.from.length),
          speed: KIND_SPEED[eo.kind] || 0.2,
          burst: 0,
        });
      }

      // Ambient flows: far field converging on the core
      for (let i = 0; i < 28; i++) {
        const u = rand(i + 400) * 2 - 1, ph = rand(i + 500) * TAU, s2 = Math.sqrt(1 - u * u);
        const R = 55 + rand(i + 600) * 30;
        const A: [number, number, number] = [s2 * Math.cos(ph) * R * 1.3, u * R, s2 * Math.sin(ph) * R * 1.3];
        const d = nrm(A);
        const pref: [number, number, number] = Math.abs(d[1]) > 0.9 ? [1, 0, 0] : [0, 1, 0];
        const per = nrm(cross(d, pref));
        const bulge = (rand(i + 700) - 0.5) * 14;
        s.flows.push({
          type: 'amb',
          P0: A,
          P1: [d[0] * R * 0.62 + per[0] * bulge, d[1] * R * 0.62 + per[1] * bulge, d[2] * R * 0.62 + per[2] * bulge],
          P2: [d[0] * R * 0.24, d[1] * R * 0.24, d[2] * R * 0.24],
          P3: [0, 0, 0],
          color: [0.65, 0.78, 0.95],
          t: rand(i + 800),
          speed: 0.05 + rand(i + 900) * 0.05,
          burst: 0,
        });
      }
    }

    function flashEdge(from: string, to: string, amount: number, color?: [number, number, number]) {
      const amp = amount ?? 1.0;
      for (const eo of s.edges) {
        if ((eo.from === from && eo.to === to) || (eo.from === to && eo.to === from)) {
          eo.act = Math.min(1.8, eo.act + amp);
          for (const fl of s.flows) {
            if (fl.type === 'edge' && fl.eo === eo) fl.burst = Math.max(fl.burst, Math.min(1.4, amp));
          }
        }
      }
      if (from) {
        const nd = s.nodes.get(from);
        if (nd) {
          nd.excite = Math.min(2.5, nd.excite + amp);
          if (color) nd.exciteColor = color;
        }
      }
      if (to) {
        const nd = s.nodes.get(to);
        if (nd) {
          nd.excite = Math.min(2.5, nd.excite + amp * 0.85);
          if (color) nd.exciteColor = color;
        }
      }
    }

    function onArrive(fl: Flow) {
      if (fl.type === 'amb') return;
      if (fl.type === 'hub' && fl.nodeId) {
        if (fl.P3[0] === 0 && fl.P3[1] === 0 && fl.P3[2] === 0) {
          s.coreFlash = Math.min(1.6, s.coreFlash + 0.3);
        } else {
          const nd = s.nodes.get(fl.nodeId);
          if (nd) nd.excite = Math.min(2.5, nd.excite + 0.16);
        }
      } else if (fl.type === 'edge' && fl.eo) {
        const nd = s.nodes.get(fl.eo.to);
        if (nd) nd.excite = Math.min(2.5, nd.excite + 0.2);
      }
    }

    function lookBasis() {
      const ca = Math.cos(s.cam.az), sa = Math.sin(s.cam.az), ce = Math.cos(s.cam.el), se = Math.sin(s.cam.el);
      const ex = s.cam.r * ce * ca, ey = s.cam.r * se, ez = s.cam.r * ce * sa;
      const fx = -ex, fy = -ey, fz = -ez;
      const fl = Math.hypot(fx, fy, fz) || 1;
      const f: [number, number, number] = [fx / fl, fy / fl, fz / fl];
      let r: [number, number, number] = nrm(cross([0, 1, 0], f));
      if (Math.hypot(r[0], r[1], r[2]) < 1e-5) r = nrm(cross([1, 0, 0], f));
      const u = cross(f, r);
      return { ex, ey, ez, r, u, f };
    }

    function project(x: number, y: number, z: number, basis: ReturnType<typeof lookBasis>): [number, number, number] {
      const dx = x - basis.ex, dy = y - basis.ey, dz = z - basis.ez;
      const cx = dx * basis.r[0] + dy * basis.r[1] + dz * basis.r[2];
      const cy = dx * basis.u[0] + dy * basis.u[1] + dz * basis.u[2];
      const depth = dx * basis.f[0] + dy * basis.f[1] + dz * basis.f[2];
      const d = Math.max(0.4, depth);
      const fl = Math.min(canvas!.width, canvas!.height) * 1.25;
      return [canvas!.width / 2 + (cx * fl) / d, canvas!.height / 2 - (cy * fl) / d, d];
    }

    function rgba(c: [number, number, number], a: number): string {
      return `rgba(${Math.round(c[0] * 255)},${Math.round(c[1] * 255)},${Math.round(c[2] * 255)},${clamp(a, 0, 1).toFixed(3)})`;
    }

    let prevLayout = '';

    function frame() {
      const now = performance.now();
      const dt = Math.min(0.05, (now - s.lastT) / 1000);
      s.lastT = now;
      const t = now / 1000;

      // Update layout if graph changed
      const layoutKey = JSON.stringify(graph?.nodes?.map(n => n.id) ?? []);
      if (layoutKey !== prevLayout) {
        updateLayout();
        prevLayout = layoutKey;
      }

      // Camera
      if (!s.dragging && !RM) s.camT.az += 0.0006;
      s.cam.az += (s.camT.az - s.cam.az) * 0.08;
      s.cam.el += (s.camT.el - s.cam.el) * 0.08;
      s.cam.r += (s.camT.r - s.cam.r) * 0.08;

      // Decay
      for (const eo of s.edges) eo.act *= Math.exp(-dt * 0.9);
      for (const nd of s.nodes.values()) nd.excite *= Math.exp(-dt * 1.4);
      s.coreFlash *= Math.exp(-dt * 1.7);
      s.globalBoost *= Math.exp(-dt * 1.1);
      if (s.selectID && now >= s.selectUntil) s.selectID = null;

      // Clear
      const W = canvas!.width, H = canvas!.height;
      ctx!.setTransform(1, 0, 0, 1, 0, 0);
      ctx!.clearRect(0, 0, W, H);

      const basis = lookBasis();
      const selOn = s.selectID && now < s.selectUntil;

      // Stars
      for (const st of stars) {
        const [px, py, d] = project(st[0], st[1], st[2], basis);
        if (d < 1) continue;
        const a = (RM ? 0.6 : 0.35 + 0.45 * Math.sin(t * 0.7 + st[4])) * Math.max(0, 0.85 - d / 180);
        if (a <= 0.02) continue;
        ctx!.fillStyle = `rgba(120,180,230,${a.toFixed(3)})`;
        const r = Math.max(0.6, st[3] * (Math.min(W, H) / d) * 0.04);
        ctx!.fillRect(px, py, r, r);
      }

      // Flows
      ctx!.lineCap = 'round';
      for (const fl of s.flows) {
        const isEdge = fl.type === 'edge';
        const isAmb = fl.type === 'amb';
        const glow = isEdge && fl.eo ? Math.min(1.6, fl.eo.act + fl.eo.rest + Math.min((fl.eo.count || 1) / 8, 1)) : 0;
        if (fl.burst > 0.01) fl.burst *= Math.exp(-dt * 1.3); else fl.burst = 0;

        let em = 1;
        if (s.hoverID || selOn) {
          const touch = isEdge && fl.eo
            ? (fl.eo.from === s.hoverID || fl.eo.to === s.hoverID || (selOn && (fl.eo.from === s.selectID || fl.eo.to === s.selectID)))
            : (isAmb ? false : (fl.nodeId === s.hoverID || (selOn && fl.nodeId === s.selectID)));
          em = touch ? 1.9 : (isAmb ? 0.5 : 0.4);
        }

        let a = (isAmb ? 0.07 : fl.type === 'hub' ? 0.22 : 0.24 + 0.4 * Math.min(1.4, glow)) * em
          + fl.burst * 0.35 + s.globalBoost * 0.2;
        a = Math.min(1, a);

        const path = new Path2D();
        for (let j = 0; j <= 22; j++) {
          const p = cubic(fl.P0, fl.P1, fl.P2, fl.P3, j / 22);
          const pr = project(p[0], p[1], p[2], basis);
          if (j === 0) path.moveTo(pr[0], pr[1]); else path.lineTo(pr[0], pr[1]);
        }

        if (isEdge && glow > 0.45) {
          ctx!.setLineDash([]);
          ctx!.strokeStyle = rgba(fl.color, (0.05 + 0.13 * Math.min(1.4, glow)) * em);
          ctx!.lineWidth = (2.2 + 3 * Math.min(1.4, glow)) * DPR;
          ctx!.stroke(path);
        }

        ctx!.setLineDash([2 * DPR, 5 * DPR]);
        ctx!.strokeStyle = rgba(fl.color, a);
        ctx!.lineWidth = (isAmb ? 0.7 : fl.type === 'hub' ? 0.9 : 0.9 + 0.9 * Math.min(1.3, glow)) * DPR;
        ctx!.stroke(path);
        ctx!.setLineDash([]);

        // Particles
        const u = fl.t;
        for (let g = 2; g >= 0; g--) {
          const uu = u - g * 0.05;
          if (uu < 0) continue;
          const p = cubic(fl.P0, fl.P1, fl.P2, fl.P3, uu);
          const pr = project(p[0], p[1], p[2], basis);
          if (pr[2] < 1) continue;
          const b = (g === 0 ? 0.85 : g === 1 ? 0.4 : 0.18) * (isAmb ? 0.55 : 1) * em;
          ctx!.fillStyle = rgba(fl.color, Math.min(1, b + fl.burst * 0.3));
          const rr = (isAmb ? 0.55 : 1) * clamp(180 / pr[2], 1.2, 9) * (g ? 0.6 : 1);
          ctx!.beginPath(); ctx!.arc(pr[0], pr[1], rr, 0, TAU); ctx!.fill();
        }

        fl.t += dt * fl.speed * (1 + Math.min(2, glow + fl.burst) * 0.7);
        if (fl.t >= 1) { fl.t -= 1; onArrive(fl); }
      }

      // Core hub
      const core = project(0, 0, 0, basis);
      if (core[2] > 0.6) {
        const size = (4.6 + (RM ? 0 : Math.sin(t * 1.3) * 0.6) + s.coreFlash * 2.8 + s.globalBoost * 1.6) * (24 / core[2]);
        const g = ctx!.createRadialGradient(core[0], core[1], 0, core[0], core[1], size * 4.4);
        g.addColorStop(0, 'rgba(255,224,170,0.95)');
        g.addColorStop(0.24, `rgba(255,140,60,${(0.5 + s.coreFlash * 0.3).toFixed(2)})`);
        g.addColorStop(1, 'rgba(255,80,20,0)');
        ctx!.fillStyle = g;
        ctx!.beginPath(); ctx!.arc(core[0], core[1], size * 4.4, 0, TAU); ctx!.fill();
        ctx!.fillStyle = 'rgba(255,244,220,0.9)';
        ctx!.beginPath(); ctx!.arc(core[0], core[1], size * 0.5, 0, TAU); ctx!.fill();
      }

      // Inference waves
      for (let i = s.waves.length - 1; i >= 0; i--) {
        const wv = s.waves[i], u = (now - wv.t0) / 1500;
        if (u >= 1) { s.waves.splice(i, 1); continue; }
        const r = Math.max(0.1, u * Math.min(W, H) * 0.62 * (24 / Math.max(12, core[2])));
        ctx!.strokeStyle = `rgba(111,211,255,${(0.4 * (1 - u)).toFixed(3)})`;
        ctx!.lineWidth = (2.5 * (1 - u) + 0.5) * DPR;
        ctx!.beginPath(); ctx!.arc(core[0], core[1], r, 0, TAU); ctx!.stroke();
      }

      // Shockwaves
      for (let i = s.shocks.length - 1; i >= 0; i--) {
        const sh = s.shocks[i], u = (now - sh.t0) / 900;
        if (u >= 1) { s.shocks.splice(i, 1); continue; }
        const m = s.nodes.get(sh.id);
        if (!m) continue;
        const [px, py, d] = project(m.pos[0], m.pos[1], m.pos[2], basis);
        if (d < 1) continue;
        ctx!.strokeStyle = `rgba(255,255,255,${(0.55 * (1 - u)).toFixed(3)})`;
        ctx!.lineWidth = (2 * (1 - u) + 0.4) * DPR;
        ctx!.beginPath(); ctx!.arc(px, py, (22 / d) * u * 6 + 2, 0, TAU); ctx!.stroke();
      }

      // Agent nodes
      s.screenPos.clear();
      ctx!.font = `${11 * DPR}px ui-monospace, Menlo, monospace`;
      ctx!.textAlign = 'center';
      ctx!.textBaseline = 'top';

      for (const nd of s.nodes.values()) {
        const e = nd.excite + nd.base;
        const [px, py, d] = project(nd.pos[0], nd.pos[1], nd.pos[2], basis);
        if (d < 1) continue;
        const foc = nd.id === s.hoverID || (selOn && nd.id === s.selectID);
        const size = Math.max(2.8, (nd.status === 'active' ? 7 : 4.2) * (1 + 0.7 * Math.min(e, 1.6)) * (1 + (foc ? 0.15 : 0)) * (22 / d));
        const cc = nd.exciteColor || (nd.status === 'active' ? [110, 210, 255] : [200, 165, 105]);
        s.screenPos.set(nd.id, [px, py, size]);

        const g = ctx!.createRadialGradient(px, py, 0, px, py, size * 3.4);
        g.addColorStop(0, `rgba(255,255,255,${Math.min(1, 0.55 + 0.45 * e).toFixed(2)})`);
        g.addColorStop(0.3, `rgba(${cc[0]},${cc[1]},${cc[2]},${(0.4 + 0.5 * Math.min(e, 1)).toFixed(2)})`);
        g.addColorStop(1, `rgba(${cc[0]},${cc[1]},${cc[2]},0)`);
        ctx!.fillStyle = g;
        ctx!.beginPath(); ctx!.arc(px, py, size * 3.4, 0, TAU); ctx!.fill();

        if (nd.status === 'active') {
          ctx!.setLineDash([size * 1.05, size * 1.9]);
          ctx!.lineDashOffset = -t * (RM ? 1.5 : 9) * DPR;
          ctx!.strokeStyle = rgba([cc[0] / 255, cc[1] / 255, cc[2] / 255], 0.4 + 0.45 * Math.min(e, 1));
          ctx!.lineWidth = Math.max(0.7, DPR);
          ctx!.beginPath(); ctx!.arc(px, py, size * 2.4, 0, TAU); ctx!.stroke();
          ctx!.setLineDash([]);
        } else {
          ctx!.setLineDash([size * 0.5, size * 0.5]);
          ctx!.strokeStyle = 'rgba(200,165,105,0.4)';
          ctx!.lineWidth = Math.max(0.6, DPR);
          ctx!.beginPath(); ctx!.arc(px, py, size * 2.1, 0, TAU); ctx!.stroke();
          ctx!.setLineDash([]);
        }

        ctx!.fillStyle = `rgba(255,255,255,${clamp(0.7 + 0.3 * Math.min(e, 1), 0, 1).toFixed(2)})`;
        ctx!.beginPath(); ctx!.arc(px, py, size * 0.45, 0, TAU); ctx!.fill();

        const la = clamp((1.25 - d / 46) * (0.6 + 0.4 * Math.min(e, 1)) * (foc ? 1.7 : 1), 0, 1);
        if (la > 0.06) {
          ctx!.fillStyle = `rgba(215,232,255,${la.toFixed(2)})`;
          ctx!.fillText(nd.name, px, py + size * 2.5);
        }
      }

      // Empty state
      if (s.nodes.size === 0) {
        ctx!.fillStyle = 'rgba(123,135,163,0.8)';
        ctx!.font = `${12 * DPR}px ui-monospace, Menlo, monospace`;
        ctx!.fillText('no agents yet — click + INIT AGENT', W / 2, H / 2 + 60 * DPR);
      }

      s.animationId = requestAnimationFrame(frame);
    }

    resize();
    window.addEventListener('resize', resize);
    s.animationId = requestAnimationFrame(frame);

    // Process activity
    const prevAct = new Set<string>();
    const actInterval = setInterval(() => {
      for (const ev of activity) {
        const k = `${ev.at}|${ev.kind}|${ev.actor}|${ev.other}`;
        if (!prevAct.has(k) && ev.from && ev.to) {
          prevAct.add(k);
          flashEdge(ev.from, ev.to, 1.4, KIND_COLOR[ev.kind] || KIND_COLOR.used);
          if (ev.from && s.nodes.has(ev.from)) {
            s.waves.push({ t0: performance.now() });
            s.globalBoost = 1;
          }
        }
      }
    }, 1000);

    return () => {
      cancelAnimationFrame(s.animationId);
      clearInterval(actInterval);
      window.removeEventListener('resize', resize);
    };
  }, [graph, activity]);

  return (
    <canvas
      ref={canvasRef}
      className="w-full h-full cursor-grab active:cursor-grabbing"
      style={{ touchAction: 'none' }}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseUp}
      onWheel={handleWheel}
      aria-label="3D agent network visualization"
      role="img"
    />
  );
}
