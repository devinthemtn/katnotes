// GitHub-style callouts: a blockquote whose first line is [!type] becomes a
// coloured box with an icon and title.
//
//   > [!WARNING]            > [!idea] Custom title
//   > body                  > body
//
// The five GitHub types work out of the box; [callouts.<type>] in
// config.toml restyles them or adds new ones. An unknown type stays a plain
// blockquote, as on GitHub. Colours come from calloutCSS (a --callout-color
// per type), so the markup only carries classes and survives DOMPurify.
import { Lexer, type MarkedExtension, type Tokens } from 'marked';

export interface CalloutStyle {
  color?: string;
  icon?: string;
  title?: string;
}

// Built-in colours per theme (GitHub's palette), and icons.
export const BUILTIN_CALLOUTS: Record<string, { icon: string; light: string; dark: string }> = {
  note: { icon: 'ℹ️', light: '#0969da', dark: '#4493f8' },
  tip: { icon: '💡', light: '#1a7f37', dark: '#3fb950' },
  important: { icon: '❗', light: '#8250df', dark: '#ab7df8' },
  warning: { icon: '⚠️', light: '#9a6700', dark: '#d29922' },
  caution: { icon: '⛔', light: '#cf222e', dark: '#f85149' },
};

const DEFAULT_ICON = '●';
const marker = /^\[!([A-Za-z0-9-]+)\][ \t]*([^\n]*)(?:\n|$)/;
const cssColor = /^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+|(rgb|rgba|hsl|hsla)\([0-9.,%/ ]+\))$/;

// The configured types, set from config.toml by setCallouts.
let custom: Record<string, CalloutStyle> = {};

export function setCallouts(c: Record<string, CalloutStyle> | null | undefined) {
  custom = {};
  for (const [k, v] of Object.entries(c ?? {})) if (v) custom[k.toLowerCase()] = v;
}

function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`);
}

function known(type: string): boolean {
  return type in BUILTIN_CALLOUTS || !!custom[type]?.color;
}

function defaultTitle(type: string): string {
  return type.charAt(0).toUpperCase() + type.slice(1).replace(/-/g, ' ');
}

export const calloutExtension: MarkedExtension = {
  renderer: {
    blockquote(token: Tokens.Blockquote) {
      const m = marker.exec(token.text);
      if (!m) return false;
      const type = m[1].toLowerCase();
      if (!known(type)) return false;
      const style = custom[type] ?? {};
      const icon = style.icon || BUILTIN_CALLOUTS[type]?.icon || DEFAULT_ICON;
      const title = m[2].trim()
        ? this.parser.parseInline(Lexer.lexInline(m[2].trim(), this.parser.options))
        : escapeHTML(style.title || defaultTitle(type));
      const body = token.text.slice(m[0].length);
      const inner = body.trim() ? this.parser.parse(Lexer.lex(body, this.parser.options)) : '';
      return `<div class="callout callout-${type}">\n`
        + `<p class="callout-title"><span class="callout-icon">${escapeHTML(icon)}</span>${title}</p>\n`
        + `${inner}</div>\n`;
    },
  },
};

// calloutCSS sets --callout-color for every type: built-ins follow the light
// or dark theme unless config.toml gives them a colour.
export function calloutCSS(dark: boolean): string {
  const rules: string[] = [];
  const colors: Record<string, string> = {};
  for (const [k, v] of Object.entries(BUILTIN_CALLOUTS)) colors[k] = dark ? v.dark : v.light;
  for (const [k, v] of Object.entries(custom)) {
    if (v.color && cssColor.test(v.color) && /^[a-z0-9-]+$/.test(k)) colors[k] = v.color;
  }
  for (const [k, c] of Object.entries(colors)) rules.push(`.callout-${k} { --callout-color: ${c}; }`);
  return rules.join('\n');
}
