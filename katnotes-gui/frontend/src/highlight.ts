// {{ … }} in a note is highlighted in the preview, in the theme's
// highlight / highlight_bg colours. The braces stay visible; Markdown inside
// still renders. Code spans and blocks are left alone, since marked
// tokenizes those before inline extensions see the text.
import type { MarkedExtension, Tokens } from 'marked';

const re = /^\{\{([^\n{}]*[^\s{}][^\n{}]*)\}\}/;

export const highlightExtension: MarkedExtension = {
  extensions: [{
    name: 'highlight',
    level: 'inline',
    start(src: string) {
      const i = src.indexOf('{{');
      return i < 0 ? undefined : i;
    },
    tokenizer(src: string) {
      const m = re.exec(src);
      if (!m) return undefined;
      return { type: 'highlight', raw: m[0], tokens: this.lexer.inlineTokens(m[1]) };
    },
    renderer(token: Tokens.Generic) {
      return `<mark class="hl">{{${this.parser.parseInline(token.tokens ?? [])}}}</mark>`;
    },
  }],
};
