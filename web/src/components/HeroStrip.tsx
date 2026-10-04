import { useAppStore } from '../store';

function Stat({
  label,
  value,
  tone = 'accent',
}: {
  label: string;
  value: string;
  tone?: 'accent' | 'term' | 'muted';
}) {
  const toneCls = tone === 'term' ? 'text-term' : tone === 'muted' ? 'text-text' : 'text-accent';
  return (
    <div className="sheen glass rounded-xl px-3.5 py-2.5 min-w-[104px]">
      <div className="label mb-0.5">{label}</div>
      <div className={`font-mono text-xl leading-none phosphor ${toneCls}`}>{value}</div>
    </div>
  );
}

// Compact command-center hero above the graph: identity + live telemetry.
export function HeroStrip() {
  const { health, agents, graph, connection } = useAppStore();

  const online = (v: string | undefined) => (v === 'ok' ? 'online' : v ?? '—');
  const dbOk = health?.db === 'ok';
  const embedOk = health?.embedder === 'ok';

  return (
    <section className="relative shrink-0 overflow-hidden border-b border-white/5 bg-grid" aria-label="System overview">
      {/* Accent bloom behind the title */}
      <div
        className="pointer-events-none absolute -top-24 -left-16 h-64 w-[38rem] rounded-full opacity-60"
        style={{ background: 'radial-gradient(closest-side, rgba(0,212,255,0.14), transparent)' }}
        aria-hidden="true"
      />
      <div
        className="pointer-events-none absolute -top-20 right-10 h-48 w-96 rounded-full opacity-50"
        style={{ background: 'radial-gradient(closest-side, rgba(61,255,158,0.10), transparent)' }}
        aria-hidden="true"
      />

      <div className="relative flex flex-wrap items-end gap-x-8 gap-y-4 px-5 py-5">
        <div className="min-w-0">
          <p className="font-mono text-[11px] uppercase tracking-[0.3em] text-accent/90 mb-1.5">
            {'// mission control'}
          </p>
          <h1 className="font-ui text-3xl sm:text-[2.6rem] font-bold leading-[1.02] tracking-tight">
            <span className="text-gradient-accent">Memory Grid</span>
          </h1>
          <p className="mt-2 max-w-xl text-sm text-textMuted hidden sm:block">
            Every write, read, and message moving between agents — live from Postgres.
            <span className="font-mono text-term/80 ml-2">[{connection.status}]</span>
          </p>
        </div>

        <div className="flex-1" />

        <dl className="flex flex-wrap items-stretch gap-2.5">
          <Stat label="note versions" value={health ? health.note_versions.toLocaleString() : '—'} />
          <Stat label="live streams" value={health ? String(health.streams) : '—'} />
          <Stat label="agents" value={String(agents.length)} tone="term" />
          <Stat label="links" value={String(graph?.edges.length ?? 0)} tone="term" />
          <Stat
            label="db"
            value={health ? online(health.db) : '—'}
            tone={dbOk ? 'term' : 'muted'}
          />
          <Stat
            label="embedder"
            value={health ? online(health.embedder) : '—'}
            tone={embedOk ? 'term' : 'muted'}
          />
        </dl>
      </div>
    </section>
  );
}
