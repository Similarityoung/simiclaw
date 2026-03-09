import { motion } from 'framer-motion';
import { Outlet } from 'react-router-dom';

import { WorkspaceErrorBoundary } from '@/app/error/workspace-error-boundary';
import { motionTokens } from '@/app/motion/tokens';
import { useRuntimeHealth } from '@/features/runtime/hooks/use-runtime-health';
import { deriveHeaderSummary } from '@/features/runtime/model/runtime-model';
import { useRunsList } from '@/features/runs/hooks/use-runs-list';
import { useSessionList } from '@/features/sessions/hooks/use-session-list';
import { AppHeader } from '@/widgets/app-header/app-header';
import { AppSidebar } from '@/widgets/app-sidebar/app-sidebar';

export function RouteShell(): JSX.Element {
  const { healthQuery, readyQuery } = useRuntimeHealth();
  const sessionsQuery = useSessionList(12);
  const runsQuery = useRunsList(undefined, 8);
  const header = deriveHeaderSummary(
    healthQuery.data,
    readyQuery.data,
    sessionsQuery.data?.items,
    runsQuery.data?.items,
  );

  return (
    <div className="mx-auto flex min-h-screen max-w-[1800px] gap-6 p-6">
      <AppSidebar />
      <div className="flex min-w-0 flex-1 flex-col gap-6">
        <AppHeader
          onlineLabel={header.onlineLabel}
          readyLabel={header.readyLabel}
          primaryModel={header.primaryModel}
          updatedAt={header.updatedAt}
        />
        <WorkspaceErrorBoundary>
          <motion.main
            initial={motionTokens.page.initial}
            animate={motionTokens.page.animate}
            exit={motionTokens.page.exit}
            transition={motionTokens.transition}
            className="min-w-0"
          >
            <Outlet />
          </motion.main>
        </WorkspaceErrorBoundary>
      </div>
    </div>
  );
}
