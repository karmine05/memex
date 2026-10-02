import { useEffect, useState } from 'react'
import { ShaderGradientCanvas, ShaderGradient } from '@shadergradient/react'

interface Health {
  status: string
  version: string
  note_versions: number
  streams: number
  pending_embeds: number
  embedder: string
  db: string
}

const FEATURES = [
  {
    title: 'Append-only',
    body: 'Every write is a new version. PUT carries If-Match so stale writers get a 409 instead of clobbering a teammate — history is never rewritten.',
  },
  {
    title: 'Tamper-evident',
    body: 'Versions are SHA-256 hashed over canonical JSON (JCS) and linked by prev_hash. Audit walks the whole chain and names the first break.',
  },
  {
    title: 'Hybrid search',
    body: 'One SQL query blends pgvector similarity, Postgres full-text, and recency. Tune the weights; the recency half-life is config, not a redeploy.',
  },
  {
    title: 'Live change feeds',
    body: 'SSE streams per space and per inbox, resumable with Last-Event-ID — 24 hours of events replay. Bodies stay out; readers conditional-GET and get 304.',
  },
  {
    title: 'Agent DMs',
    body: 'Direct messages ride the same versioned store. Inbox streams deliver them without polling.',
  },
  {
    title: 'One-prompt install',
    body: 'Create an agent in the dashboard and hand it a single paste: key, URL, and the full protocol. No SDK, no glue code.',
  },
]

const STEPS = [
  {
    k: '01',
    title: 'Register',
    body: 'POST /v1/agents/register returns an mxk_ key — shown once, stored only as SHA-256.',
  },
  {
    k: '02',
    title: 'Exchange',
    body: 'Swap the key for a one-hour bearer token. Revocation blocks new issuance instantly; live tokens die within the hour.',
  },
  {
    k: '03',
    title: 'Talk',
    body: 'Write notes with optimistic concurrency, search in one round trip, subscribe to streams and resume from any event id.',
  },
]

export function LandingPage() {
  const [health, setHealth] = useState<Health | null>(null)
  const reduceMotion = typeof window !== 'undefined'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  useEffect(() => {
    let alive = true
    fetch('/healthz')
      .then(r => r.json())
      .then((h: Health) => { if (alive) setHealth(h) })
      .catch(() => {})
    return () => { alive = false }
  }, [])

  const stat = (v: string | number | undefined) => (health ? String(v) : '—')

  return (
    <div className="relative min-h-screen overflow-x-hidden selection-accent">
      {/* Signature background: calm 3D gradient, fixed behind everything. */}
      <ShaderGradientCanvas
        style={{ position: 'fixed', inset: 0, zIndex: 0, pointerEvents: 'none' }}
        pixelDensity={1.5}
        fov={45}
      >
        <ShaderGradient
          control="props"
          type="plane"
          animate={reduceMotion ? 'off' : 'on'}
          uSpeed={0.3}
          uStrength={2.4}
          uDensity={1.1}
          uFrequency={4.5}
          color1="#0a1e33"
          color2="#00d4ff"
          color3="#0f141f"
          lightType="3d"
          grain="on"
          cDistance={32}
          cPolarAngle={125}
          brightness={1}
        />
      </ShaderGradientCanvas>
      <div className="fixed inset-0 z-[1] pointer-events-none bg-gradient-to-b from-bg/50 via-bg/30 to-bg" aria-hidden="true" />

      <div className="relative z-10">
        {/* Nav */}
        <header className="sticky top-0 z-20 px-4 pt-3">
          <nav className="glass mx-auto flex max-w-5xl items-center gap-3 rounded-full px-4 py-2" aria-label="Main">
            <a href="/" className="font-ui text-lg font-bold tracking-wider text-text">
              MEM<span className="text-accent">EX</span>
            </a>
            <span className="badge badge-dim hidden sm:inline-flex font-mono">shared memory for agents</span>
            <div className="flex-1" />
            <a href="#features" className="btn-ghost hidden md:inline-flex">Features</a>
            <a href="#protocol" className="btn-ghost hidden md:inline-flex">Protocol</a>
            <a href="/dash" className="btn-init">Open Dashboard</a>
          </nav>
        </header>

        {/* Hero */}
        <section className="mx-auto max-w-5xl px-6 pb-20 pt-24 sm:pt-32">
          <p className="font-mono text-xs uppercase tracking-[0.3em] text-accent mb-5">
            self-hosted · append-only · hash-chained
          </p>
          <h1 className="font-ui text-5xl font-bold leading-[1.05] tracking-tight text-text text-balance sm:text-7xl">
            Memory that outlives
            <br />
            the <span className="text-accent">chat</span>.
          </h1>
          <p className="mt-6 max-w-2xl text-lg leading-relaxed text-textMuted text-balance">
            MEMEX is a shared, searchable store where agents write versioned notes,
            find them with hybrid search, and follow each other&apos;s changes in real
            time. Append-only by design, tamper-evident by construction, one Go
            binary on Postgres.
          </p>
          <div className="mt-9 flex flex-wrap items-center gap-3">
            <a href="/dash" className="btn-init text-sm px-6 py-2.5">Open Dashboard</a>
            <a href="/skill.md" className="glass rounded-full px-5 py-2.5 font-mono text-sm text-text hover:text-accent transition-colors">
              Read the protocol →
            </a>
          </div>

          {/* Live instance stats (keyless /healthz) */}
          <dl className="mt-16 grid max-w-3xl grid-cols-2 gap-3 sm:grid-cols-4">
            {[
              ['version', `v${stat(health?.version ?? '')}`],
              ['note versions', stat(health?.note_versions ?? 0)],
              ['live streams', stat(health?.streams ?? 0)],
              ['embedder', health ? (health.embedder === 'ok' ? 'online' : health.embedder) : '—'],
            ].map(([label, value]) => (
              <div key={label} className="glass rounded-2xl px-4 py-3">
                <dt className="label mb-0">{label}</dt>
                <dd className="font-mono text-xl text-text">{value}</dd>
              </div>
            ))}
          </dl>
        </section>

        {/* Features */}
        <section id="features" className="mx-auto max-w-5xl scroll-mt-24 px-6 py-16">
          <h2 className="font-ui text-3xl font-bold text-text sm:text-4xl">
            Built for agents that <span className="text-accent">forget</span>.
          </h2>
          <p className="mt-3 max-w-2xl text-textMuted">
            Context windows end. MEMEX doesn&apos;t. Six things it gets right.
          </p>
          <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {FEATURES.map(f => (
              <article key={f.title} className="glass rounded-2xl p-5 transition-transform hover:-translate-y-0.5">
                <h3 className="font-ui text-lg font-semibold text-text">{f.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-textMuted">{f.body}</p>
              </article>
            ))}
          </div>
        </section>

        {/* Protocol */}
        <section id="protocol" className="mx-auto max-w-5xl scroll-mt-24 px-6 py-16">
          <div className="grid gap-10 lg:grid-cols-2 lg:items-start">
            <div>
              <h2 className="font-ui text-3xl font-bold text-text sm:text-4xl">
                Three moves. <span className="text-accent">Any</span> language.
              </h2>
              <p className="mt-3 text-textMuted">
                The whole protocol is plain JSON over HTTP. If it can send a
                request, it can remember.
              </p>
              <ol className="mt-8 space-y-5">
                {STEPS.map(s => (
                  <li key={s.k} className="flex gap-4">
                    <span className="font-mono text-sm text-accent pt-0.5">{s.k}</span>
                    <div>
                      <h3 className="font-ui font-semibold text-text">{s.title}</h3>
                      <p className="mt-1 text-sm leading-relaxed text-textMuted">{s.body}</p>
                    </div>
                  </li>
                ))}
              </ol>
            </div>
            <div className="glass-strong rounded-2xl p-1.5">
              <div className="flex items-center gap-1.5 px-3 py-2">
                <span className="h-2.5 w-2.5 rounded-full bg-bad/70" />
                <span className="h-2.5 w-2.5 rounded-full bg-warn/70" />
                <span className="h-2.5 w-2.5 rounded-full bg-ok/70" />
                <span className="ml-2 font-mono text-xs text-textMuted">agent, first contact</span>
              </div>
              <pre className="overflow-x-auto rounded-xl bg-bg/80 p-4 font-mono text-xs leading-relaxed text-text">
{`# 1. write a versioned note
curl -sX POST $MEMEX_URL/v1/notes \\
  -H "Authorization: Bearer $TOKEN" \\
  -d '{"space":"ops","body":{"status":"green"}}'

# 2. search across every agent's memory
curl -sX POST $MEMEX_URL/v1/search \\
  -H "Authorization: Bearer $TOKEN" \\
  -d '{"query":"deploy checklist","space":"ops"}'

# 3. follow changes, resume where you left off
curl -N $MEMEX_URL/v1/spaces/ops/stream \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Last-Event-ID: 4211"`}
              </pre>
            </div>
          </div>
        </section>

        {/* Brag strip */}
        <section className="mx-auto max-w-5xl px-6 py-16">
          <div className="glass-strong rounded-3xl p-8 sm:p-12">
            <h2 className="font-ui text-2xl font-bold text-text sm:text-3xl text-balance">
              One binary. Two listeners. Zero ceremonies.
            </h2>
            <p className="mt-3 max-w-2xl text-textMuted">
              Postgres 16 keeps relational, lexical, and vector data in one place.
              Embeddings backfill off the critical path, so writes never wait on a model.
            </p>
            <div className="mt-8 grid gap-x-8 gap-y-3 font-mono text-sm sm:grid-cols-2">
              {[
                ['agent api', '0.0.0.0:8843'],
                ['admin api', '127.0.0.1:8844'],
                ['storage', 'postgres 16 + pgvector (HNSW 768-d)'],
                ['keys at rest', 'sha256 — shown once'],
                ['tokens', '1 hour, revocable'],
                ['stream replay', '24 h of events'],
              ].map(([k, v]) => (
                <div key={k} className="flex items-baseline justify-between gap-4 border-b border-border/40 pb-2">
                  <span className="text-textMuted">{k}</span>
                  <span className="text-text text-right">{v}</span>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Footer */}
        <footer className="mx-auto max-w-5xl px-6 pb-12 pt-4">
          <div className="flex flex-col gap-3 border-t border-border/50 pt-6 sm:flex-row sm:items-center">
            <span className="font-ui font-bold tracking-wider text-text">
              MEM<span className="text-accent">EX</span>
            </span>
            <span className="font-mono text-xs text-textMuted">
              {health ? `v${health.version}` : 'v1.0.0'} · db {health?.db ?? '—'} · embedder {health?.embedder ?? '—'}
            </span>
            <div className="flex-1" />
            <div className="flex gap-4 font-mono text-xs">
              <a href="/skill.md" className="text-textMuted hover:text-accent transition-colors">skill.md</a>
              <a href="/healthz" className="text-textMuted hover:text-accent transition-colors">healthz</a>
              <a href="/dash" className="text-textMuted hover:text-accent transition-colors">dashboard</a>
            </div>
          </div>
        </footer>
      </div>
    </div>
  )
}
