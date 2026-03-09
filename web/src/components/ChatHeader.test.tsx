import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import ChatHeader from './ChatHeader';

describe('ChatHeader', () => {
  it('没有 lastActivity 时不显示分隔符和占位时间', () => {
    render(
      <ChatHeader
        conversation="demo-conversation"
        model="fake/default"
        onToggleSidebar={vi.fn()}
        onToggleDebug={vi.fn()}
      />,
    );

    expect(screen.getByText('等待输入')).toBeInTheDocument();
    expect(screen.queryByText('·')).not.toBeInTheDocument();
    expect(screen.queryByText('—')).not.toBeInTheDocument();
  });
});
