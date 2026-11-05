import type { MarkdownRenderer } from "vitepress";

// ```mermaid のコードブロックを、ブラウザで図を描くMermaidDiagramコンポーネントに置き換える。
// 図の文は属性へ入れるので、引用符やHTMLの記号で崩れないようにエンコードして渡す。
export function renderMermaidFences(markdown: MarkdownRenderer): void {
  const renderFence = markdown.renderer.rules.fence;
  markdown.renderer.rules.fence = (tokens, index, options, env, renderer) => {
    const token = tokens[index];
    if (token.info.trim() === "mermaid") {
      return `<MermaidDiagram source="${encodeURIComponent(token.content)}" />\n`;
    }
    return renderFence ? renderFence(tokens, index, options, env, renderer) : renderer.renderToken(tokens, index, options);
  };
}
