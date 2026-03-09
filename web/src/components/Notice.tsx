import styles from './Notice.module.css';

interface NoticeProps {
  title: string;
  body: string;
  tone?: 'neutral' | 'danger';
  className?: string;
}

export default function Notice({ title, body, tone = 'neutral', className }: NoticeProps) {
  const cardClassName = tone === 'danger' ? `${styles.notice} ${styles.danger}` : styles.notice;
  return (
    <div className={className ? `${cardClassName} ${className}` : cardClassName}>
      <div className={styles.title}>{title}</div>
      <div className={styles.body}>{body}</div>
    </div>
  );
}
