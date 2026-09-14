import { ReactElement } from 'react';

export type IconName =
  | 'gauge'
  | 'route'
  | 'layers'
  | 'message'
  | 'shield'
  | 'list'
  | 'key'
  | 'activity'
  | 'chart'
  | 'check'
  | 'x'
  | 'bolt'
  | 'chevron'
  | 'arrow'
  | 'refresh'
  | 'search'
  | 'copy'
  | 'lock'
  | 'plus'
  | 'dot'
  | 'warning'
  | 'sun'
  | 'moon'
  | 'minus'
  | 'menu'
  | 'users';

const ICONS: Record<IconName, ReactElement> = {
  gauge: (
    <>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 12l3.5-3.5" />
      <path d="M5.5 19a8.5 8.5 0 0 1 13 0" />
    </>
  ),
  route: (
    <>
      <circle cx="5.5" cy="6" r="2.2" />
      <circle cx="18.5" cy="18" r="2.2" />
      <path d="M7.7 6h5.6a5 5 0 0 1 5 5v4.8" />
    </>
  ),
  layers: (
    <>
      <path d="M12 3l9 5-9 5-9-5 9-5z" />
      <path d="M3 13.5l9 5 9-5" />
    </>
  ),
  message: (
    <>
      <path d="M21 14.5a2 2 0 0 1-2 2H8.5L3.5 20V5.5a2 2 0 0 1 2-2h13.5a2 2 0 0 1 2 2v9z" />
      <path d="M7.5 8.5h9M7.5 12h6" />
    </>
  ),
  shield: (
    <>
      <path d="M12 21.5s7.5-3 7.5-9.5V5.5L12 2.5 4.5 5.5V12c0 6.5 7.5 9.5 7.5 9.5z" />
      <path d="M9 11.5l2 2 4-4.5" />
    </>
  ),
  list: (
    <>
      <path d="M8.5 6h12M8.5 12h12M8.5 18h12" />
      <path d="M3.5 6h.01M3.5 12h.01M3.5 18h.01" />
    </>
  ),
  key: (
    <>
      <circle cx="7.5" cy="15.5" r="4.5" />
      <path d="M10.8 12.2L20 3M15 8l3 3" />
    </>
  ),
  activity: <path d="M22 12h-4l-3 8-6-16-3 8H2" />,
  chart: (
    <>
      <path d="M3 3v18h18" />
      <path d="M7 14v4M12 9v9M17 12v6" />
    </>
  ),
  check: <path d="M20 6L9 17l-5-5" />,
  x: <path d="M18 6L6 18M6 6l12 12" />,
  bolt: <path d="M13 2L3 14h7l-1 8 10-12h-7l1-8z" />,
  chevron: <path d="M9 18l6-6-6-6" />,
  arrow: <path d="M5 12h14M13 6l6 6-6 6" />,
  refresh: (
    <>
      <path d="M23 4v6h-6" />
      <path d="M1 20v-6h6" />
      <path d="M3.5 9a9 9 0 0 1 14.9-3.4L23 10M1 14l4.6 4.4A9 9 0 0 0 20.5 15" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-4.3-4.3" />
    </>
  ),
  copy: (
    <>
      <rect x="9" y="9" width="12" height="12" rx="2" />
      <path d="M5 15V5a2 2 0 0 1 2-2h10" />
    </>
  ),
  lock: (
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  dot: <circle cx="12" cy="12" r="4" />,
  warning: (
    <>
      <path d="M10.3 3.9L1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
      <path d="M12 9v4M12 17h.01" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
    </>
  ),
  moon: <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />,
  minus: <path d="M5 12h14" />,
  menu: <path d="M3 6h18M3 12h18M3 18h18" />,
  users: (
    <>
      <circle cx="9" cy="8" r="3.2" />
      <path d="M3.5 20a5.5 5.5 0 0 1 11 0" />
      <path d="M16 5.2a3.2 3.2 0 0 1 0 5.6" />
      <path d="M17.5 14.6A5.5 5.5 0 0 1 20.5 20" />
    </>
  )
};

export function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {ICONS[name]}
    </svg>
  );
}
