import { useAppStore } from '../store';

type StatTone = 'accent' | 'term' | 'muted' | 'ok' | 'bad';

// Heroicons (outline) paths — same inline-SVG convention as the rest of the UI.
const ICONS = {
  versions:
    'M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z',
  streams:
    'M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z',
  agents:
    'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z',
  links: 'M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5',
  db: 'M21.75 17.25v-.228a4.5 4.5 0 00-.12-1.03l-2.268-9.64a3.375 3.375 0 00-3.285-2.602H7.923a3.375 3.375 0 00-3.285 2.602l-2.268 9.64a4.5 4.5 0 00-.12 1.03v.228m19.5 0a3 3 0 01-3 3H5.25a3 3 0 01-3-3m19.5 0a3 3 0 00-3-3H5.25a3 3 0 00-3 3m16.5 0h.008v.008h-.008v-.008zm-3 0h.008v.008h-.008v-.008z',
  embed:
    'M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456z',
} as const;

function Stat({
  icon,
  label,
  value,
  tone = 'accent',
}: {
  icon: keyof typeof ICONS;
  label: string;
  value: string;
  tone?: StatTone;
}) {
  const toneCls = {
    accent: 'text-accent',
    term: 'text-term',
    muted: 'text-text',
    ok: 'text-ok',
    bad: 'text-bad',
  }[tone];
  const iconCls = tone === 'ok' ? 'text-ok' : tone === 'bad' ? 'text-bad' : 'text-textMuted';
  return (
    <div
      className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-2.5 py-1.5 transition-colors hover:border-white/25"
      title={label}
    >
      <svg
        className={`h-3.5 w-3.5 shrink-0 ${iconCls}`}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <path strokeLinecap="round" strokeLinejoin="round" d={ICONS[icon]} />
      </svg>
      <span className={`font-mono text-sm leading-none ${toneCls}`}>{value}</span>
    </div>
  );
}

// Compact telemetry strip above the graph: title left, six icon+number
// chips right. Chips are content-sized (no stretch grid) so they never
// balloon in wide layouts; they wrap 3+3 / 2+2+2 on narrow viewports.
export function HeroStrip() {
  const { health, agents, graph } = useAppStore();

  const status = (v: string | undefined) =>
    v === undefined
      ? { value: '—', tone: 'muted' as StatTone }
      : v === 'ok'
        ? { value: 'online', tone: 'ok' as StatTone }
        : { value: v, tone: 'bad' as StatTone };
  const db = status(health?.db);
  const embed = status(health?.embedder);

  return (
    <section className="relative shrink-0 border-b border-white/5 bg-grid" aria-label="System overview">
      <div className="relative grid gap-x-8 gap-y-2.5 px-5 py-4 2xl:grid-cols-12 2xl:items-center">
        <div className="min-w-0 2xl:col-span-4">
          <p className="font-mono text-[11px] uppercase tracking-[0.3em] text-accent/90 mb-1.5">
            {'// mission control'}
          </p>
          <h1 className="font-ui text-3xl sm:text-[2.6rem] font-bold leading-[1.02] tracking-tight">
            <span className="text-gradient-accent">Memory Grid</span>
          </h1>
          <p className="mt-2 max-w-xl text-sm text-textMuted hidden sm:block">
            Every write, read, and message moving between agents, live from Postgres.
          </p>
        </div>

        <div className="flex flex-wrap justify-end gap-2 2xl:col-span-8">
          <Stat icon="versions" label="note versions" value={health ? health.note_versions.toLocaleString() : '—'} />
          <Stat icon="streams" label="live streams" value={health ? String(health.streams) : '—'} />
          <Stat icon="agents" label="agents" value={String(agents.length)} tone="term" />
          <Stat icon="links" label="links" value={String(graph?.edges.length ?? 0)} tone="term" />
          <Stat icon="db" label="postgres" value={db.value} tone={db.tone} />
          <Stat icon="embed" label="embedder" value={embed.value} tone={embed.tone} />
        </div>
      </div>
    </section>
  );
}
