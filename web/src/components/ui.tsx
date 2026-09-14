import { ReactNode } from 'react';
import { Icon, IconName } from '../icons';

export function Metric(props: {
  label: string;
  value: ReactNode;
  icon: IconName;
  tone?: string;
}) {
  return (
    <div className={'metric' + (props.tone ? ' ' + props.tone : '')}>
      <span className="m-icon"><Icon name={props.icon} size={18} /></span>
      <div className="m-body">
        <span className="m-value">{props.value}</span>
        <span className="m-label">{props.label}</span>
      </div>
    </div>
  );
}

export function Card(props: {
  title?: string;
  subtitle?: string;
  right?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="card">
      {(props.title || props.right) && (
        <header className="card-head">
          <div>
            {props.title && <h3>{props.title}</h3>}
            {props.subtitle && <span className="card-sub">{props.subtitle}</span>}
          </div>
          {props.right}
        </header>
      )}
      <div className="card-body">{props.children}</div>
    </section>
  );
}

export function Section(props: {
  id: string;
  index: string;
  title: string;
  description?: string;
  right?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="section reveal" id={props.id}>
      <header className="section-head">
        <div className="section-num">{props.index}</div>
        <div>
          <h2>{props.title}</h2>
          {props.description && <p>{props.description}</p>}
        </div>
        {props.right && <div className="section-right">{props.right}</div>}
      </header>
      {props.children}
    </section>
  );
}

export function EmptyState(props: { icon: ReactNode; text: string }) {
  return (
    <div className="empty">
      <span className="empty-icon">{props.icon}</span>
      <span>{props.text}</span>
    </div>
  );
}

export function Spinner() {
  return <span className="spinner" role="status" aria-hidden="true" />;
}

export function LoadingState(props: { text?: string; inline?: boolean }) {
  if (props.inline) {
    return (
      <span className="loading-inline">
        <Spinner />
        {props.text ?? 'Loading…'}
      </span>
    );
  }
  return (
    <div className="loading-state">
      <Spinner />
      {props.text ?? 'Loading…'}
    </div>
  );
}

export function Alert(props: { tone: 'error' | 'ok' | 'info' | 'warn'; children: ReactNode; icon?: ReactNode }) {
  return (
    <div className={'alert ' + props.tone}>
      {props.icon}
      <span>{props.children}</span>
    </div>
  );
}

export interface TabItem {
  id: string;
  label: string;
  icon?: ReactNode;
}

export function Tabs(props: { items: TabItem[]; active: string; onChange: (id: string) => void }) {
  return (
    <div className="tabs" role="tablist">
      {props.items.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={t.id === props.active}
          className={'tab' + (t.id === props.active ? ' active' : '')}
          onClick={() => props.onChange(t.id)}
        >
          {t.icon}
          {t.label}
        </button>
      ))}
    </div>
  );
}

export function Badge(props: { tone: string; children: ReactNode; dot?: boolean }) {
  return (
    <span className={'badge ' + props.tone}>
      {props.dot && <span className="b-dot" />}
      {props.children}
    </span>
  );
}

export function DecisionBadge(props: { decision: string }) {
  const d = (props.decision || 'pass').toLowerCase();
  const tone =
    d === 'block' ? 'block' : d === 'redact' ? 'redact' : d === 'warn' ? 'warn' : d === 'log_only' ? 'info' : 'pass';
  return <Badge tone={tone}>{d.toUpperCase()}</Badge>;
}

export function StatusBadge(props: { status: string }) {
  const s = (props.status || 'info').toLowerCase();
  const tone =
    s === 'completed' ? 'pass'
    : s === 'error' ? 'error'
    : s === 'started' ? 'warn'
    : s === 'authenticated' || s === 'info' ? 'accent'
    : 'neutral';
  return <Badge tone={tone}>{s.toUpperCase()}</Badge>;
}

export function Stat(props: { label: string; value: ReactNode; tone?: string; note?: string; small?: boolean }) {
  return (
    <div className={'stat' + (props.tone ? ' ' + props.tone : '')}>
      <div className="stat-label">{props.label}</div>
      <div className={'stat-value' + (props.small ? ' sm' : '')}>{props.value}</div>
      {props.note && <div className="stat-note">{props.note}</div>}
    </div>
  );
}

export function fmtTime(ts: number): string {
  return new Date(ts).toLocaleTimeString('en-US', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  });
}

export function shortId(id: string): string {
  return id.replace('req-', '').slice(-8);
}

export function fmtNum(n: number, digits = 0): string {
  return n.toLocaleString('en-US', { maximumFractionDigits: digits, minimumFractionDigits: 0 });
}