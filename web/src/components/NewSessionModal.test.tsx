import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import NewSessionModal from './NewSessionModal';

describe('NewSessionModal', () => {
  it('在输入框按 Escape 时关闭弹窗', async () => {
    const onClose = vi.fn();

    render(
      <NewSessionModal
        open
        value="demo-conversation"
        placeholder="web-20260309T120000Z"
        onClose={onClose}
        onChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    const input = screen.getByRole('textbox', { name: 'conversation_id' });
    await userEvent.type(input, '{Escape}');

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
