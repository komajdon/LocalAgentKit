/** A unified diff with added and removed lines marked. Content is shown as text only. */
export function DiffView({ diff }: { diff: string }) {
  return (
    <pre className="diff-view" aria-label="Changes">
      {diff.split('\n').map((line, i) => {
        const kind = line.startsWith('+++') || line.startsWith('---') ? 'head'
          : line.startsWith('+') ? 'add' : line.startsWith('-') ? 'del' : line.startsWith('@@') ? 'head' : ''
        return <div key={i} className={kind}>{line || ' '}</div>
      })}
    </pre>
  )
}
