import { useEffect, useId, useRef } from 'react';

interface NewSessionModalProps {
  open: boolean;
  value: string;
  placeholder: string;
  onClose: () => void;
  onChange: (value: string) => void;
  onConfirm: () => void;
}

function collectFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true');
}

export default function NewSessionModal({
  open,
  value,
  placeholder,
  onClose,
  onChange,
  onConfirm,
}: NewSessionModalProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const titleID = useId();
  const descriptionID = useId();

  useEffect(() => {
    if (!open) {
      previousFocusRef.current?.focus();
      return;
    }

    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = 'hidden';

    window.requestAnimationFrame(() => {
      inputRef.current?.focus();
    });

    return () => {
      document.body.style.overflow = '';
      previousFocusRef.current?.focus();
    };
  }, [open]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(4,6,10,0.7)] px-4 backdrop-blur-xl" onClick={onClose}>
      <div
        className="ui-panel-strong w-full max-w-xl p-6 sm:p-7"
        ref={dialogRef}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            onClose();
            return;
          }

          if (event.key !== 'Tab' || !dialogRef.current) {
            return;
          }

          const focusable = collectFocusableElements(dialogRef.current);
          if (focusable.length === 0) {
            event.preventDefault();
            dialogRef.current.focus();
            return;
          }

          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          const active = document.activeElement;

          if (event.shiftKey && active === first) {
            event.preventDefault();
            last.focus();
          } else if (!event.shiftKey && active === last) {
            event.preventDefault();
            first.focus();
          }
        }}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={descriptionID}
        tabIndex={-1}
      >
        <div className="flex items-start justify-between gap-4 border-b border-white/8 pb-5">
          <div>
            <div className="ui-kicker mb-2">New session</div>
            <div id={titleID} className="text-[26px] font-medium tracking-[-0.03em] text-[var(--color-ink-strong)]">
              新建会话
            </div>
            <div id={descriptionID} className="mt-2 text-[14px] leading-6 tracking-[-0.011em] text-[var(--color-ink-soft)]">
              为空时自动生成 `web-&lt;UTC时间&gt;` 风格的 conversation_id。
            </div>
          </div>
          <button className="ui-button-chrome" type="button" onClick={onClose}>
            关闭
          </button>
        </div>
        <label className="mt-5 block">
          <span className="mb-2 block text-[12px] font-semibold uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">
            conversation_id
          </span>
          <div className="ui-input-shell">
            <input
              ref={inputRef}
              className="ui-input"
              value={value}
              onChange={(event) => onChange(event.target.value)}
              placeholder={placeholder}
            />
          </div>
        </label>
        <div className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button className="ui-button-secondary" type="button" onClick={onClose}>
            取消
          </button>
          <button className="ui-button-primary" type="button" onClick={onConfirm}>
            使用该会话
          </button>
        </div>
      </div>
    </div>
  );
}
