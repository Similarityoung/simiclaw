import { PenSquare } from 'lucide-react';

import { Button } from './ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';
import { Input } from './ui/input';

interface NewSessionModalProps {
  open: boolean;
  value: string;
  placeholder: string;
  onClose: () => void;
  onChange: (value: string) => void;
  onConfirm: () => void;
}

export default function NewSessionModal({
  open,
  value,
  placeholder,
  onClose,
  onChange,
  onConfirm,
}: NewSessionModalProps) {
  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !nextOpen && onClose()}>
      <DialogContent>
        <DialogHeader className="border-b border-[rgba(15,23,42,0.08)] pb-5">
          <div className="ui-kicker mb-2">New session</div>
          <DialogTitle>新建会话</DialogTitle>
          <DialogDescription>为空时自动生成 `web-&lt;UTC时间&gt;` 风格的 conversation_id。</DialogDescription>
        </DialogHeader>
        <label className="mt-5 block">
          <span className="mb-2 block text-[12px] font-semibold uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">
            conversation_id
          </span>
          <div className="relative">
            <PenSquare className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-ink-muted)]" />
            <Input autoFocus className="pl-10" value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} />
          </div>
        </label>
        <DialogFooter className="mt-6">
          <Button variant="secondary" type="button" onClick={onClose}>
            取消
          </Button>
          <Button type="button" onClick={onConfirm}>
            使用该会话
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
