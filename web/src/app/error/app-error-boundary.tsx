import { AlertTriangle, RotateCcw } from 'lucide-react';
import { Component, type ErrorInfo, type ReactNode } from 'react';

import { Button } from '@/components/ui/button';

interface AppErrorBoundaryProps {
  children: ReactNode;
}

interface AppErrorBoundaryState {
  hasError: boolean;
}

export class AppErrorBoundary extends Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  state: AppErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): AppErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('App boundary caught an error', error, info);
  }

  render(): ReactNode {
    if (!this.state.hasError) {
      return this.props.children;
    }

    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <div className="surface-shell flex max-w-xl flex-col items-start gap-4 rounded-[2rem] p-8">
          <div className="flex items-center gap-3 text-sm uppercase tracking-[0.28em] text-muted-foreground">
            <AlertTriangle className="h-4 w-4" />
            Application Boundary
          </div>
          <div>
            <h1 className="text-3xl font-semibold">控制台外壳发生异常</h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              页面级错误已被拦截，避免整个应用白屏。刷新后会重新进入 Dashboard。
            </p>
          </div>
          <Button onClick={() => window.location.assign('/dashboard')}>
            <RotateCcw className="mr-2 h-4 w-4" />
            返回控制台
          </Button>
        </div>
      </div>
    );
  }
}
