import { useEffect, useRef } from 'react';

// Old-school digital rain: columns of binary + katakana glyphs falling behind
// the UI. One canvas for the whole page, capped DPR, ~22fps — cheap enough to
// sit under everything. Honors prefers-reduced-motion by drawing a single
// static frame.
export function MatrixRain({
  opacity = 0.15,
  className = '',
  density = 1,
}: {
  opacity?: number;
  className?: string;
  /** 1 = full columns, 0.5 = half the columns active. */
  density?: number;
}) {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const RM = matchMedia('(prefers-reduced-motion: reduce)').matches;
    const DPR = Math.min(window.devicePixelRatio || 1, 1.5);
    const CELL = 16; // css px between glyphs
    const TRAIL = 9; // glyphs kept behind the head
    const GLYPHS = '01ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎ0123456789ABCDEF';

    type Column = {
      y: number;
      speed: number;
      lastCell: number;
      hist: string[];
      cyan: boolean;
      x: number;
      live: boolean;
    };
    let cols: Column[] = [];

    const seed = (n: number) => {
      const x = Math.sin(n * 91.7 + 13.3) * 43758.5453;
      return x - Math.floor(x);
    };
    const glyph = (n: number) => GLYPHS[Math.floor(seed(n) * GLYPHS.length) % GLYPHS.length];

    function resize() {
      const w = window.innerWidth;
      const h = window.innerHeight;
      canvas!.width = Math.round(w * DPR);
      canvas!.height = Math.round(h * DPR);
      canvas!.style.width = w + 'px';
      canvas!.style.height = h + 'px';

      const n = Math.ceil(w / (CELL * 1.35)) + 1;
      cols = [];
      for (let i = 0; i < n; i++) {
        cols.push({
          x: i * CELL * 1.35 + seed(i * 3 + 1) * 6,
          y: seed(i * 5 + 2) * h - h * 0.5,
          speed: 14 + seed(i * 7 + 3) * 34,
          lastCell: Number.NEGATIVE_INFINITY,
          hist: [],
          cyan: seed(i * 11 + 4) < 0.14,
          live: seed(i * 13 + 5) < Math.min(1, density + 0.25),
        });
      }
    }

    function step(dt: number) {
      const W = canvas!.width;
      const H = canvas!.height;
      const cell = CELL * DPR;
      ctx!.clearRect(0, 0, W, H);
      ctx!.font = `${Math.round(CELL * 1.02 * DPR)}px "JetBrains Mono", ui-monospace, monospace`;
      ctx!.textAlign = 'left';
      ctx!.textBaseline = 'top';

      for (let i = 0; i < cols.length; i++) {
        const c = cols[i];
        if (!c.live) continue;
        c.y += c.speed * dt;
        if (c.y > H + TRAIL * cell) {
          c.y = -seed(i + Math.floor(c.y)) * H * 0.4 - cell;
          c.hist = [];
          c.lastCell = Number.NEGATIVE_INFINITY;
        }

        const cellIndex = Math.floor(c.y / cell);
        if (cellIndex > c.lastCell) {
          c.hist.unshift(glyph(i * 101 + cellIndex * 17 + Math.floor(c.y)));
          if (c.hist.length > TRAIL + 1) c.hist.pop();
          c.lastCell = cellIndex;
        }

        for (let k = 0; k < c.hist.length; k++) {
          const y = cellIndex * cell - k * cell + (c.y % cell);
          if (y < -cell || y > H) continue;
          const fade = k === 0 ? 0.95 : Math.max(0, 0.72 - k * 0.085);
          if (fade <= 0.03) continue;
          const rgb = c.cyan ? '0,212,255' : '61,255,158';
          ctx!.fillStyle = k === 0
            ? `rgba(210,255,230,${(fade * 0.9).toFixed(3)})`
            : `rgba(${rgb},${fade.toFixed(3)})`;
          ctx!.fillText(c.hist[k], c.x * DPR, y);
        }
      }
    }

    let raf = 0;
    let last = 0;
    function frame(t: number) {
      raf = requestAnimationFrame(frame);
      if (t - last < 45) return; // ~22fps
      const dt = Math.min(0.12, (t - last) / 1000);
      last = t;
      step(dt);
    }

    resize();
    window.addEventListener('resize', resize);
    if (RM) {
      step(0.001); // static frame, no loop
    } else {
      raf = requestAnimationFrame(frame);
    }

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', resize);
    };
  }, [density]);

  return (
    <canvas
      ref={ref}
      className={`pointer-events-none ${className}`}
      style={{
        opacity,
        maskImage: 'linear-gradient(to bottom, transparent 0%, black 14%, black 86%, transparent 100%)',
        WebkitMaskImage: 'linear-gradient(to bottom, transparent 0%, black 14%, black 86%, transparent 100%)',
      }}
      aria-hidden="true"
    />
  );
}
