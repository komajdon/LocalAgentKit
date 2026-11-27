// A small stroke icon set, so the interface does not depend on emoji fonts.
const PATHS = {
  plus: 'M12 5v14M5 12h14',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4',
  settings: 'M4 7h10M18 7h2M4 17h2M10 17h10M16 5v4M8 15v4',
  chart: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  layers: 'M12 3l9 5-9 5-9-5zM3 13l9 5 9-5',
  activity: 'M3 12h4l3-7 4 14 3-7h4',
  diff: 'M7 3h7l5 5v13H7zM14 3v5h5M10 13h6M13 10v6M10 18h6',
  download: 'M12 4v11M7 11l5 5 5-5M5 20h14',
  panel: 'M4 5h16v14H4zM9 5v14',
  send: 'M12 19V5M6 11l6-6 6 6',
  stop: 'M7 7h10v10H7z',
  mic: 'M12 3a3 3 0 0 0-3 3v6a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3zM6 11a6 6 0 0 0 12 0M12 17v4',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  edit: 'M4 20h4L19 9l-4-4L4 16zM13 7l4 4',
  branch: 'M7 4v10M7 18a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM17 8a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM17 8c0 5-10 3-10 6',
  check: 'M5 12l5 5 9-10',
  x: 'M6 6l12 12M18 6L6 18',
  chevron: 'M9 6l6 6-6 6',
  alert: 'M12 4l9 16H3zM12 10v4M12 17v.5',
  shield: 'M12 3l7 3v6c0 4-3 7-7 9-4-2-7-5-7-9V6z',
  terminal: 'M4 5h16v14H4zM7 9l3 3-3 3M12 15h5',
  file: 'M7 3h7l5 5v13H7zM14 3v5h5',
  globe: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18',
  tool: 'M14 7a4 4 0 0 1 5-4l-3 3 2 2 3-3a4 4 0 0 1-5 5l-8 8-3-3 8-8z',
  clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2',
  spark: 'M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z',
  pin: 'M9 4h6l-1 6 3 3H7l3-3zM12 13v7',
  tag: 'M3 12V4h8l10 10-8 8zM8 8v.5',
  trash: 'M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13',
  cpu: 'M7 7h10v10H7zM10 3v4M14 3v4M10 17v4M14 17v4M3 10h4M3 14h4M17 10h4M17 14h4',
  database: 'M5 6c0-4 14-4 14 0s-14 4-14 0zM5 6v12c0 4 14 4 14 0V6M5 12c0 4 14 4 14 0',
  plug: 'M9 3v5M15 3v5M6 8h12v3a6 6 0 0 1-12 0zM12 17v4',
  sun: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M19 5l-1.5 1.5M6.5 17.5L5 19',
} as const

export type IconName = keyof typeof PATHS

export function Icon({ name, size = 16 }: { name: IconName; size?: number }) {
  return (
    <svg className="icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} />
    </svg>
  )
}

/** The icon that best describes a tool. */
export function toolIcon(tool: string): IconName {
  if (tool === 'shell') return 'terminal'
  if (tool === 'search_web') return 'globe'
  if (tool === 'search_files') return 'search'
  if (tool === 'get_time') return 'clock'
  if (tool === 'list_dir') return 'folder'
  if (tool === 'delete_file') return 'trash'
  if (tool === 'write_file') return 'edit'
  if (tool.startsWith('read_')) return 'file'
  return tool.includes('__') ? 'plug' : 'tool'
}
