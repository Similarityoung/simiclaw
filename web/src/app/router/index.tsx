import { Navigate, createBrowserRouter } from 'react-router-dom';

import { RouteShell } from '@/app/router/route-shell';

export const appRoutes = [
  {
    path: '/',
    element: <Navigate to="/dashboard" replace />,
  },
  {
    path: '/',
    element: <RouteShell />,
    children: [
      {
        path: 'dashboard',
        lazy: async () => {
          const module = await import('@/pages/dashboard/page');
          return { Component: module.DashboardPage };
        },
      },
      {
        path: 'console',
        lazy: async () => {
          const module = await import('@/pages/console/page');
          return { Component: module.ConsolePage };
        },
      },
      {
        path: 'channels',
        lazy: async () => {
          const module = await import('@/pages/channels/page');
          return { Component: module.ChannelsPage };
        },
      },
      {
        path: 'memory',
        lazy: async () => {
          const module = await import('@/pages/memory/page');
          return { Component: module.MemoryPage };
        },
      },
      {
        path: 'skills',
        lazy: async () => {
          const module = await import('@/pages/skills/page');
          return { Component: module.SkillsPage };
        },
      },
    ],
  },
];

export const router = createBrowserRouter(appRoutes);
