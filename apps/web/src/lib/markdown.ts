import { Marked } from "marked";
import markedKatex from "marked-katex-extension";

export interface Heading {
  id: string;
  text: string;
  depth: number;
}

export interface RenderedMarkdown {
  html: string;
  headings: Heading[];
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (char) => {
    switch (char) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case '"':
        return "&quot;";
      default:
        return "&#39;";
    }
  });
}

function slugify(text: string): string {
  return (
    text
      .toLowerCase()
      .trim()
      .replace(/[^\p{L}\p{N}]+/gu, "-")
      .replace(/^-+|-+$/g, "") || "section"
  );
}

export function renderMarkdown(markdown: string): RenderedMarkdown {
  const headings: Heading[] = [];
  const used = new Map<string, number>();

  const marked = new Marked({ gfm: true, breaks: false });
  marked.use(markedKatex({ throwOnError: false, nonStandard: false }));
  marked.use({
    renderer: {
      heading({ tokens, depth }) {
        const inline = this.parser.parseInline(tokens);
        const plain = inline.replace(/<[^>]+>/g, "").trim();
        const base = slugify(plain);
        const count = used.get(base) ?? 0;
        used.set(base, count + 1);
        const id = count === 0 ? base : `${base}-${count + 1}`;
        if (depth >= 1 && depth <= 3) {
          headings.push({ id, text: plain, depth });
        }
        return `<h${depth} id="${id}">${inline}</h${depth}>\n`;
      },
      code({ text, lang }) {
        const language = (lang ?? "").trim().split(/\s+/)[0];
        if (language === "mermaid") {
          return [
            `<figure class="mermaid-block" data-mermaid>`,
            `<pre class="mermaid-source"><code>${escapeHtml(text)}</code></pre>`,
            `<figcaption class="mermaid-fallback">图示尚未渲染</figcaption>`,
            `</figure>`,
          ].join("");
        }
        const label = language || "text";
        return [
          `<div class="code-block">`,
          `<div class="code-head"><span class="code-lang">${escapeHtml(label)}</span>`,
          `<button type="button" class="code-copy" data-copy>复制</button></div>`,
          `<pre><code${language ? ` class="language-${escapeHtml(language)}"` : ""}>${escapeHtml(text)}</code></pre>`,
          `</div>`,
        ].join("");
      },
    },
  });

  const html = marked.parse(markdown, { async: false });
  return { html, headings };
}
