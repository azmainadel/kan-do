import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

// Links open in a new tab; raw HTML in the source is never rendered.
const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
};

export function Markdown({ children, className = "" }: { children: string; className?: string }) {
  return (
    <div className={`md break-words ${className}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
