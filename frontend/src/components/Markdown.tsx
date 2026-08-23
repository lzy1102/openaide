import { useMemo } from 'react';
import { marked } from 'marked';
import DOMPurify from 'dompurify';

/** Markdown 渲染：marked + DOMPurify（LLM 输出不可信，一律消毒后注入） */
export function Markdown({ text }: { text: string }): React.ReactElement {
  const html = useMemo(() => {
    marked.setOptions({ gfm: true, breaks: true });
    return DOMPurify.sanitize(marked.parse(text, { async: false }) as string);
  }, [text]);
  // eslint-disable-next-line react/no-danger
  return <div className="md" dangerouslySetInnerHTML={{ __html: html }} />;
}
