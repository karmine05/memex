import { useState, useEffect, useRef } from 'react';
import { MatrixRain } from './MatrixRain';

interface AdminKeyModalProps {
  onSubmit: (key: string) => void;
  value: string;
  onChange: (value: string) => void;
}

export function AdminKeyModal({ onSubmit, value, onChange }: AdminKeyModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!value.trim()) return;

    setLoading(true);
    setError('');

    try {
      // Validate the key against an admin-gated endpoint. /healthz is
      // keyless, so it would accept any string as a "valid" key.
      const res = await fetch('/admin/agents', {
        headers: { Authorization: `Bearer ${value.trim()}` },
      });

      if (res.status === 401) {
        throw new Error('Invalid admin key');
      }
      if (!res.ok) {
        throw new Error(`Server unavailable (HTTP ${res.status})`);
      }

      onSubmit(value.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Invalid admin key');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative min-h-screen flex items-center justify-center p-4 overflow-hidden bg-bg">
      <MatrixRain className="fixed inset-0 z-0" opacity={0.17} density={0.9} />
      <div
        className="pointer-events-none absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 h-[26rem] w-[26rem] rounded-full opacity-60"
        style={{ background: 'radial-gradient(closest-side, rgba(0,212,255,0.10), rgba(61,255,158,0.05), transparent)' }}
        aria-hidden="true"
      />

      <div className="relative z-10 w-full max-w-md">
        <h1 className="sr-only">MEMEX admin authentication</h1>
        <div
          className="crt glass-strong rounded-2xl overflow-hidden animate-in font-mono text-term"
          style={{ boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.12), inset 0 0 70px rgba(61,255,158,0.08), 0 24px 60px -16px rgba(0,0,0,0.8), 0 0 44px -16px rgba(61,255,158,0.45)' }}
        >
          <header className="relative z-10 flex items-center gap-2 px-4 py-2.5 border-b border-term/15 bg-black/35">
            <span className="flex gap-1.5" aria-hidden="true">
              <span className="w-2.5 h-2.5 rounded-full bg-bad/80" />
              <span className="w-2.5 h-2.5 rounded-full bg-warn/80" />
              <span className="w-2.5 h-2.5 rounded-full bg-ok/80" />
            </span>
            <span className="text-[11px] tracking-wider text-term/80 phosphor">memex://secure.gate</span>
          </header>

          <div className="relative z-10 px-5 py-5 bg-black/25">
            <div className="text-[11px] space-y-1 mb-5 text-term/50">
              <p><span className="text-accent/80">$</span> <span className="text-term/75">memex auth --operator</span></p>
              <p>// admin api listening on 127.0.0.1:8844</p>
              <p className="text-term/70">
                operator key required<span className="cursor-block" aria-hidden="true" />
              </p>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label htmlFor="admin-key" className="label">Admin Key (mxa_)</label>
                <input
                  ref={inputRef}
                  id="admin-key"
                  type="password"
                  value={value}
                  onChange={(e) => onChange(e.target.value)}
                  className="input font-mono"
                  placeholder="mxa_…"
                  autoComplete="off"
                  disabled={loading}
                  aria-describedby={error ? 'key-error' : undefined}
                />
                {error && <p id="key-error" className="text-bad text-sm mt-1 phosphor" role="alert">{error}</p>}
              </div>

              <button
                type="submit"
                className="btn-init w-full"
                disabled={loading || !value.trim()}
              >
                {loading ? 'Verifying…' : 'Authenticate ↵'}
              </button>
            </form>

            <div className="mt-5 pt-4 border-t border-term/15 text-[11px] text-term/45">
              <p>
                key lives in <code className="text-term/70">data/admin.key</code> · starts with{' '}
                <code className="text-term/70">mxa_</code> · shown once at startup
              </p>
            </div>
          </div>
        </div>

        <p className="text-center mt-4 font-mono text-[11px] text-textMuted">
          MEM<span className="text-accent">EX</span> · shared memory for agents
        </p>
      </div>
    </div>
  );
}
