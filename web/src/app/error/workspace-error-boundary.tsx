import { AlertCircle } from 'lucide-react';
import { Component, type ErrorInfo, type ReactNode } from 'react';

import { Button } from '@/components/ui/button';

interface WorkspaceErrorBoundaryProps {
  children: ReactNode;
}

interface WorkspaceErrorBoundaryState {
  hasError: boolean;
}

export class WorkspaceErrorBoundary extends Component<WorkspaceErrorBoundaryProps, WorkspaceErrorBoundaryState> {
  state: WorkspaceErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): WorkspaceErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('Workspace boundary caught an error', error, info);
  }

  render(): ReactNode {
    if (!this.state.hasError) {
      return this.props.children;
    }

    return (
      <div className="flex min-h-[40vh] items-center justify-center p-6">
        <div className="surface-card max-w-lg p-8">
          <div className="mb-4 flex items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
            <AlertCircle className="h-4 w-4" />
            Workspace Boundary
          </div>
          <h2 className="text-2xl font-semibold">当前工作区渲染失败</h2>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">
            应用壳层仍然可用。你可以切换到其他页面，或重新尝试打开这个工作区。
          </p>
          <Button className="mt-6" variant="outline" onClick={() => window.location.reload()}>
            重新加载当前页面
          </Button>
        </div>
      </div>
    );
  }
}
