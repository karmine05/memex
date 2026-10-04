import { useEffect, useMemo, useRef, useState } from 'react';
import type { TelemetryActivity } from '../types';

const KIND: Record<string, { tag: string; cls: string; dot: string }> = {
  wrote: { tag: 'WRITE', cls: 'text-term', dot: 'bg-term' },
  used: { tag: 'READ', cls: 'text-accent', dot: 'bg-accent' },
  sent: { tag: 'DM', cls: 'text-warn', dot: 'bg-warn' },
  cited: { tag: 'CITE', cls: 'text-term', dot: 'bg-term' },
};

function clock(at: string): string {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return '--:--:--';
  return d.toLocaleTimeString('en-GB', { hour12: false });
}

function describe(ev: TelemetryActivity): { verb: string; detail: string } {
  switch (ev.kind) {
    case 'wrote':
      return { verb: 'wrote', detail: ev.space ? `→ ${ev.space}` : '' };
    case 'used':
      return { verb: 'read', detail: ev.other ? `← ${ev.other}` : '' };
    case 'sent':
      return { verb: 'dm', detail: ev.other ? `→ ${ev.other}` : '' };
    case 'cited':
      return { verb: 'cited', detail: ev.other ? `→ ${ev.other}` : '' };
    default:
      return { verb: ev.kind, detail: ev.other ?? ev.space ?? '' };
  }
}

// Floating CRT terminal streaming the live agent activity feed.
// Old-school look: green phosphor, scanlines, blinking block cursor.
export function Terminal({ activity, className = '' }: {
  activity: TelemetryActivity[];
  className?: string;
}) {
  const [open, setOpen] = useState(true);
  const bodyRef = useRef<HTMLDivElement>(null);

  // Newest at the bottom, like a real terminal tailing a log.
  const lines = useMemo(() => activity.slice(0, 60).reverse(), [activity]);

  useEffect(() => {
    const el = bodyRef.current;
    if (el && open) el.scrollTop = el.scrollHeight;
  }, [lines, open]);

  return (
    <section
      className={`crt glass-strong rounded-xl overflow-hidden font-mono text-term ${className}`}
      style={{ boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.12), inset 0 0 60px rgba(61,255,158,0.07), 0 18px 50px -12px rgba(0,0,0,0.75), 0 0 34px -14px rgba(61,255,158,0.4)' }}
      aria-label="Live activity terminal"
    >
      <header className="relative z-10 flex items-center gap-2 px-3 py-2 border-b border-term/15 bg-black/30">
        <span className="flex gap-1.5" aria-hidden="true">
          <span className="w-2.5 h-2.5 rounded-full bg-bad/80" />
          <span className="w-2.5 h-2.5 rounded-full bg-warn/80" />
          <span className="w-2.5 h-2.5 rounded-full bg-ok/80" />
        </span>
        <span className="text-[11px] tracking-wider text-term/80 phosphor truncate">
          memex://activity.log
        </span>
        <span className="flex-1" />
        <span className="badge badge-term phosphor text-[10px] py-0">
          <span className="relative flex h-1.5 w-1.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-term opacity-70" />
            <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-term" />
          </span>
          LIVE
        </span>
        <button
          onClick={() => setOpen(o => !o)}
          className="text-term/60 hover:text-term transition-colors text-[11px] px-1"
          aria-expanded={open}
          aria-label={open ? 'Collapse terminal' : 'Expand terminal'}
          title={open ? 'Collapse' : 'Expand'}
        >
          {open ? '▾' : '▸'}
        </button>
      </header>

      {open && (
        <>
          <div
            ref={bodyRef}
            className="relative z-10 overflow-y-auto scrollable px-3 py-2 space-y-0.5 bg-black/25"
            style={{ height: '154px' }}
          >
            {lines.length === 0 ? (
              <p className="text-term/45 text-xs py-2">// awaiting first signal…</p>
            ) : (
              lines.map((ev, i) => {
                const k = `${ev.at}|${ev.kind}|${ev.actor}|${ev.other ?? ''}|${ev.space ?? ''}|${i}`;
                const meta = KIND[ev.kind] ?? { tag: ev.kind.toUpperCase(), cls: 'text-term', dot: 'bg-term' };
                const { verb, detail } = describe(ev);
                return (
                  <p key={k} className="flex items-baseline gap-2 text-[11px] leading-relaxed whitespace-nowrap animate-riseGlow">
                    <span className="text-term/40 shrink-0">{clock(ev.at)}</span>
                    <span className={`${meta.dot} inline-block w-1.5 h-1.5 rounded-full shrink-0 translate-y-[-1px]`} aria-hidden="true" />
                    <span className={`${meta.cls} phosphor shrink-0 w-11`}>{meta.tag}</span>
                    <span className="text-term/85 truncate">
                      {ev.actor} <span className="text-term/45">{verb}</span>{' '}
                      <span className="text-term/70">{detail}</span>
                    </span>
                  </p>
                );
              })
            )}
          </div>

          <div className="relative z-10 flex items-center gap-1.5 px-3 py-1.5 border-t border-term/15 bg-black/35 text-[11px]">
            <span className="text-term/60">operator@memex</span>
            <span className="text-accent/80">:~$</span>
            <span className="cursor-block" aria-hidden="true" />
            <span className="flex-1" />
            <span className="text-term/35">{lines.length} events</span>
          </div>
        </>
      )}
    </section>
  );
}
