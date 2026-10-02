import { useState, useEffect, useRef } from 'react';

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
    <div className="min-h-screen flex items-center justify-center p-4 bg-bg">
      <div className="w-full max-w-md">
        <div className="panel-strong animate-in">
          <div className="text-center mb-8">
            <div className="font-ui font-bold text-3xl tracking-wider mb-2">MEM<span className="text-accent">EX</span></div>
            <p className="text-textMuted">Admin authentication required</p>
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
              {error && <p id="key-error" className="text-bad text-sm mt-1" role="alert">{error}</p>}
            </div>

            <button
              type="submit"
              className="btn-primary w-full"
              disabled={loading || !value.trim()}
            >
              {loading ? 'Verifying…' : 'Continue'}
            </button>
          </form>

          <div className="mt-6 p-3 bg-border/30 rounded-lg text-xs text-textMuted">
            <p className="font-mono mb-1">Your admin key is in <code>data/admin.key</code></p>
            <p>It starts with <code className="font-mono">mxa_</code> and is shown once at startup.</p>
          </div>
        </div>
      </div>
    </div>
  );
}