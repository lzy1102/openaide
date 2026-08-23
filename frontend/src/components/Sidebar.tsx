import type { SessionDto } from '../types';
import { relTime } from '../ui';

export function Sidebar({
  sessions,
  activeId,
  onSelect,
  onNew,
  onDelete,
}: {
  sessions: SessionDto[];
  activeId: string | null;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
}): React.ReactElement {
  const sorted = [...sessions].sort((a, b) => b.updatedAt - a.updatedAt);
  return (
    <aside className="sidebar">
      <div className="sidebar-head">
        <span>sessions</span>
      </div>
      <button className="btn-new" onClick={onNew}>
        ＋ new chat
      </button>
      <div className="session-list">
        {sorted.map((s) => (
          <div
            key={s.id}
            className={`session-item${s.id === activeId ? ' active' : ''}`}
            onClick={() => onSelect(s.id)}
          >
            <span className="sid">{s.id.slice(0, 8)}</span>
            <span className="meta">
              {s.messages.length}msg · {relTime(s.updatedAt)}
            </span>
            <button
              className="session-del"
              title="delete"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(s.id);
              }}
            >
              ✕
            </button>
          </div>
        ))}
        {sorted.length === 0 && (
          <div style={{ padding: '8px 10px', fontSize: 12, color: 'var(--text-dim)' }}>
            no sessions yet
          </div>
        )}
      </div>
    </aside>
  );
}
