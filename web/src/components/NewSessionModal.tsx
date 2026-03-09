import styles from './NewSessionModal.module.css';

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
  if (!open) {
    return null;
  }

  return (
    <div className={styles.backdrop} onClick={onClose}>
      <div
        className={styles.card}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.key === 'Escape' && onClose()}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
      >
        <div className={styles.header}>
          <div>
            <div className={styles.title}>新建会话</div>
            <div className={styles.subtitle}>为空时自动生成 `web-&lt;UTC时间&gt;` 风格的 conversation_id。</div>
          </div>
          <button className={styles.chromeButton} type="button" onClick={onClose}>
            关闭
          </button>
        </div>
        <label className={styles.field}>
          <span>conversation_id</span>
          <input value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} autoFocus />
        </label>
        <div className={styles.actions}>
          <button className={styles.ghostButton} type="button" onClick={onClose}>
            取消
          </button>
          <button className={styles.primaryButton} type="button" onClick={onConfirm}>
            使用该会话
          </button>
        </div>
      </div>
    </div>
  );
}
