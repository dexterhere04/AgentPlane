import { ReactNode, useMemo, useState } from 'react';
import { Icon, IconName } from '../icons';

/* ============================================================
   Layout & text primitives
   ============================================================ */

export function Metric(props: {
  label: string;
  value: ReactNode;
  icon: IconName;
  tone?: string;
  emphasis?: 'primary' | 'secondary';
}) {
  const emphasis = props.emphasis ?? 'secondary';
  return (
    <div className={'metric ' + emphasis + (props.tone ? ' ' + props.tone : '')}>
      <span className="m-icon"><Icon name={props.icon} size={emphasis === 'primary' ? 18 : 15} /></span>
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
  variant?: 'default' | 'quiet';
}) {
  return (
    <section className={'card' + (props.variant === 'quiet' ? ' quiet' : '')}>
      {(props.title || props.right) && (
        <header className="card-head">
          <div className="card-head-text">
            {props.title && <h3>{props.title}</h3>}
            {props.subtitle && <span className="card-sub">{props.subtitle}</span>}
          </div>
          {props.right && <div className="card-head-right">{props.right}</div>}
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
        <div className="section-head-text">
          <h2>{props.title}</h2>
          {props.description && <p>{props.description}</p>}
        </div>
        {props.right && <div className="section-right">{props.right}</div>}
      </header>
      {props.children}
    </section>
  );
}

/* ============================================================
   States: empty, loading, skeleton, error, success
   ============================================================ */

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

/** Shimmering skeleton placeholder — use where a spinner would be too small. */
export function Skeleton(props: {
  width?: string | number;
  height?: string | number;
  radius?: string | number;
  lines?: number;
  className?: string;
}) {
  const style = {
    width: props.width ?? '100%',
    height: props.height ?? 14,
    borderRadius: props.radius ?? 6
  } as React.CSSProperties;

  if (props.lines && props.lines > 1) {
    return (
      <div className={'skeleton-stack' + (props.className ? ' ' + props.className : '')}>
        {Array.from({ length: props.lines }).map((_, i) => (
          <span
            key={i}
            className="skeleton"
            style={{ ...style, width: i === props.lines! - 1 ? '62%' : style.width }}
          />
        ))}
      </div>
    );
  }

  return <span className={'skeleton' + (props.className ? ' ' + props.className : '')} style={style} />;
}

/** Alias that maps naturally onto Alert for the "error state" case. */
export function ErrorState(props: { children: ReactNode; icon?: ReactNode }) {
  return (
    <Alert tone="error" icon={props.icon}>
      {props.children}
    </Alert>
  );
}

/** Alias that maps naturally onto Alert for the "success state" case. */
export function SuccessState(props: { children: ReactNode; icon?: ReactNode }) {
  return (
    <Alert tone="ok" icon={props.icon}>
      {props.children}
    </Alert>
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

/* ============================================================
   Tabs & segmented control
   ============================================================ */

export interface TabItem {
  id: string;
  label: string;
  icon?: ReactNode;
}

/** Underline-style tabs. */
export function Tabs(props: { items: TabItem[]; active: string; onChange: (id: string) => void; ariaLabel?: string }) {
  return (
    <div className="tabs" role="tablist" aria-label={props.ariaLabel}>
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

/** Boxed segmented control — for binary / 2–4-way mode switches. */
export function Segmented(props: {
  items: TabItem[];
  active: string;
  onChange: (id: string) => void;
  ariaLabel?: string;
}) {
  return (
    <div className="segmented" role="tablist" aria-label={props.ariaLabel}>
      {props.items.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={t.id === props.active}
          className={'segmented-item' + (t.id === props.active ? ' active' : '')}
          onClick={() => props.onChange(t.id)}
        >
          {t.icon}
          {t.label}
        </button>
      ))}
    </div>
  );
}

/* ============================================================
   Badges & status
   ============================================================ */

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

/** One dot to rule them all — replaces .live-dot, .lane-dot, .ov-sys-dot, .ov-h-dot. */
export function StatusDot(props: { tone: 'ok' | 'warn' | 'bad' | 'scan' | 'neutral'; size?: 'sm' | 'md' | 'lg' }) {
  return <span className={'status-dot ' + props.tone + ' ' + (props.size ?? 'md')} aria-hidden="true" />;
}

/* ============================================================
   Tooltip & disclosure
   ============================================================ */

/** Lightweight tooltip. Renders into place — no portal, no dependency. */
export function Tooltip(props: { content: ReactNode; children: ReactNode; side?: 'top' | 'bottom' | 'right' }) {
  return (
    <span className={'tooltip-wrap' + (props.side ? ' side-' + props.side : '')}>
      {props.children}
      <span className="tooltip-bubble" role="tooltip">{props.content}</span>
    </span>
  );
}

/** Collapsible details panel. Promoted from the old Collapsible in RequestActivity. */
export function Disclosure(props: {
  title: string;
  subtitle?: string;
  defaultOpen?: boolean;
  badge?: ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(!!props.defaultOpen);
  return (
    <section className={'collapsible' + (open ? ' open' : '')}>
      <button
        className="collapsible-head"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={'collapsible-caret' + (open ? ' open' : '')}>
          <Icon name="chevron" size={13} />
        </span>
        <span className="collapsible-text">
          <span className="collapsible-title">{props.title}</span>
          {props.subtitle && <span className="collapsible-sub">{props.subtitle}</span>}
        </span>
        {props.badge && <span className="collapsible-badge">{props.badge}</span>}
      </button>
      {open && <div className="collapsible-body">{props.children}</div>}
    </section>
  );
}

/* ============================================================
   Stats (legacy helper — kept for ObservabilityView / UsageView)
   ============================================================ */

/* ============================================================
   Formatters
   ============================================================ */

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

/* ============================================================
   FilterBar — shared filtering/search/time-range control
   ============================================================ */

export interface FilterOption {
  value: string;
  label: string;
}

export interface FilterChip {
  label: string;
  onClear: () => void;
}

export function FilterBar(props: {
  /** Free-text search. Omit to hide the search input. */
  search?: {
    value: string;
    onChange: (v: string) => void;
    placeholder?: string;
  };
  /** One or more dropdowns. Omit to hide. */
  selects?: {
    value: string;
    onChange: (v: string) => void;
    options: FilterOption[];
    ariaLabel?: string;
  }[];
  /** Time-range dropdown. Omit to hide. */
  timeRange?: {
    value: string;
    onChange: (v: string) => void;
    options: FilterOption[];
  };
  /** Optional right-aligned content (e.g. count, pause button). */
  right?: ReactNode;
  /** Active filter chips shown under the bar. */
  chips?: FilterChip[];
}) {
  const hasChips = props.chips && props.chips.length > 0;
  return (
    <div className="filter-bar-wrap">
      <div className="filter-bar">
        {props.search && (
          <div className="fb-search">
            <Icon name="search" size={13} />
            <input
              className="input fb-search-input"
              value={props.search.value}
              onChange={(e) => props.search!.onChange(e.target.value)}
              placeholder={props.search.placeholder ?? 'Filter…'}
            />
            {props.search.value && (
              <button
                className="fb-clear"
                onClick={() => props.search!.onChange('')}
                aria-label="Clear search"
              >
                <Icon name="x" size={12} />
              </button>
            )}
          </div>
        )}

        {props.selects?.map((s, i) => (
          <select
            key={i}
            className="select"
            value={s.value}
            onChange={(e) => s.onChange(e.target.value)}
            aria-label={s.ariaLabel}
          >
            {s.options.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        ))}

        {props.timeRange && (
          <select
            className="select"
            value={props.timeRange.value}
            onChange={(e) => props.timeRange!.onChange(e.target.value)}
            aria-label="Time range"
          >
            {props.timeRange.options.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        )}

        {props.right && <div className="fb-right">{props.right}</div>}
      </div>

      {hasChips && (
        <div className="filter-chips">
          {props.chips!.map((c, i) => (
            <button key={i} className="filter-chip" onClick={c.onClear}>
              {c.label}
              <Icon name="x" size={11} />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/* ============================================================
   JsonBlock — raw event payload viewer
   ============================================================ */

export function JsonBlock(props: { value: unknown; maxHeight?: number }) {
  const text = useMemo(() => {
    try {
      return JSON.stringify(props.value, null, 2);
    } catch {
      return String(props.value);
    }
  }, [props.value]);
  return (
    <pre className="json-block" style={props.maxHeight ? { maxHeight: props.maxHeight } : undefined}>
      {text}
    </pre>
  );
}

/* ============================================================
   Legend — a compact key for status vocabularies
   ============================================================ */

export function Legend(props: {
  items: { tone: string; label: string; hint?: string }[];
  className?: string;
}) {
  return (
    <div className={'legend-keys' + (props.className ? ' ' + props.className : '')}>
      {props.items.map((i) => (
        <span className="legend-key" key={i.label} title={i.hint}>
          <span className={'legend-swatch ' + i.tone} />
          <span className="legend-label">{i.label}</span>
          {i.hint && <span className="legend-hint">{i.hint}</span>}
        </span>
      ))}
    </div>
  );
}