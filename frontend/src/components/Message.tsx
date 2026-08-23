import { Markdown } from './Markdown';
import type { UiMsg } from '../ui';

function ToolCard({ m }: { m: Extract<UiMsg, { kind: 'tool' }> }): React.ReactElement {
  const cls = `toolcard ${m.status}`;
  const statusLabel = m.status === 'running' ? 'running…' : m.status === 'failed' ? 'failed' : 'ok';
  return (
    <div className={cls}>
      <span>⚡</span>
      <details>
        <summary>
          {m.name}({m.args ?? ''})
        </summary>
        {(m.result || m.error) && (
          <pre>{m.error ? `error: ${m.error}` : m.result}</pre>
        )}
      </details>
      <span className="t-status">{statusLabel}</span>
    </div>
  );
}

export function MessageView({ m }: { m: UiMsg }): React.ReactElement {
  if (m.kind === 'tool') return <ToolCard m={m} />;
  if (m.kind === 'user') {
    return (
      <div className="msg user">
        <div className="msg-label" />
        <div className="msg-body user">{m.content}</div>
      </div>
    );
  }
  return (
    <div className="msg assistant">
      <div className="msg-label" />
      <div className="msg-body assistant">
        {m.content ? (
          <Markdown text={m.content} />
        ) : (
          <p style={{ color: 'var(--text-dim)' }}>…</p>
        )}
        {m.reasoning ? (
          <details className="thinking">
            <summary>✻ thinking</summary>
            <div className="thinking-body">{m.reasoning}</div>
          </details>
        ) : null}
      </div>
    </div>
  );
}
