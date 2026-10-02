import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// Renders an error screen instead of a silent blank page when a render
// exception unmounts the React tree.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('UI crashed:', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen flex items-center justify-center p-4 bg-bg">
          <div className="panel-strong w-full max-w-md">
            <div className="text-center mb-4">
              <div className="font-ui font-bold text-2xl tracking-wider mb-1">MEM<span className="text-accent">EX</span></div>
              <p className="text-bad font-mono text-sm">Something broke</p>
            </div>
            <pre className="font-mono text-xs text-textMuted bg-border/30 p-3 rounded-lg overflow-auto max-h-40 whitespace-pre-wrap">
              {this.state.error.message}
            </pre>
            <button onClick={() => window.location.reload()} className="btn-primary w-full mt-4">
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
