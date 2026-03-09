import { motion } from 'framer-motion';
import { Activity, BrainCircuit, DatabaseZap, LayoutDashboard, PanelLeftClose, PanelLeftOpen, SquareTerminal } from 'lucide-react';
import { NavLink } from 'react-router-dom';

import { motionTokens } from '@/app/motion/tokens';
import { useShellStore } from '@/app/store/shell-store';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/console', label: 'Console', icon: SquareTerminal },
  { to: '/channels', label: 'Channels', icon: Activity },
  { to: '/memory', label: 'Memory', icon: DatabaseZap },
  { to: '/skills', label: 'Skills', icon: BrainCircuit },
];

export function AppSidebar(): JSX.Element {
  const sidebarCollapsed = useShellStore((state) => state.sidebarCollapsed);
  const toggleSidebarCollapsed = useShellStore((state) => state.toggleSidebarCollapsed);

  return (
    <motion.aside
      animate={{ width: sidebarCollapsed ? '5.5rem' : '18rem' }}
      transition={motionTokens.transition}
      className="surface-shell sticky top-6 z-sidebar hidden h-[calc(100vh-3rem)] shrink-0 flex-col rounded-[2rem] lg:flex"
    >
      <div className="flex items-center justify-between border-b border-border/70 px-5 py-5">
        <div className={cn('overflow-hidden transition-opacity', sidebarCollapsed ? 'opacity-0' : 'opacity-100')}>
          <div className="text-xs uppercase tracking-[0.3em] text-muted-foreground">SimiClaw</div>
          <div className="mt-2 text-lg font-semibold">Runtime Console</div>
        </div>
        <Button variant="ghost" size="icon" onClick={toggleSidebarCollapsed} aria-label="toggle sidebar">
          {sidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
        </Button>
      </div>

      <nav className="flex flex-1 flex-col gap-2 px-4 py-5">
        {navItems.map((item) => {
          const Icon = item.icon;
          return (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-2xl px-4 py-3 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                  isActive && 'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground',
                  sidebarCollapsed && 'justify-center px-0',
                )
              }
            >
              <Icon className="h-4 w-4 shrink-0" />
              {!sidebarCollapsed ? <span>{item.label}</span> : null}
            </NavLink>
          );
        })}
      </nav>

      <div className="border-t border-border/70 px-5 py-4 text-xs leading-6 text-muted-foreground">
        {!sidebarCollapsed ? '黑白双主题控制台壳层。只展示真实接口可获得的数据。' : 'SC'}
      </div>
    </motion.aside>
  );
}
