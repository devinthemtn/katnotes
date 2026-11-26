import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Marked } from 'marked';
import { calloutExtension, calloutCSS, setCallouts } from './callouts.ts';
import { highlightExtension } from './highlight.ts';

const md = (s: string) => new Marked(calloutExtension, highlightExtension).parse(s, { async: false, gfm: true }) as string;

test('built-in type becomes a callout', () => {
  setCallouts({});
  const out = md('> [!WARNING]\n> Be **careful**.');
  assert.match(out, /^<div class="callout callout-warning">/);
  assert.match(out, /<p class="callout-title"><span class="callout-icon">⚠️<\/span>Warning<\/p>/);
  assert.match(out, /<p>Be <strong>careful<\/strong>.<\/p>/);
  assert.ok(!out.includes('[!WARNING]'));
  assert.ok(!out.includes('blockquote'));
});

test('type is case-insensitive and title can be overridden', () => {
  setCallouts({});
  const out = md('> [!note] Heads *up*\n> body');
  assert.match(out, /callout-note/);
  assert.match(out, /<\/span>Heads <em>up<\/em><\/p>/);
});

test('unknown type stays a blockquote', () => {
  setCallouts({});
  const out = md('> [!idea]\n> body');
  assert.match(out, /<blockquote>/);
  assert.match(out, /\[!idea\]/);
});

test('custom type from config', () => {
  setCallouts({ Idea: { color: '#e0af68', icon: '🧠', title: 'Big <idea>' } });
  const out = md('> [!IDEA]\n> - one\n> - two');
  assert.match(out, /callout callout-idea/);
  assert.match(out, /🧠<\/span>Big &#60;idea&#62;<\/p>/);
  assert.match(out, /<li>one<\/li>/);
  assert.match(calloutCSS(false), /\.callout-idea \{ --callout-color: #e0af68; \}/);
});

test('custom type without colour is not a callout', () => {
  setCallouts({ idea: { icon: 'x' } });
  assert.match(md('> [!idea]\n> body'), /<blockquote>/);
});

test('built-in colours follow the theme and can be overridden', () => {
  setCallouts({});
  assert.match(calloutCSS(true), /\.callout-note \{ --callout-color: #4493f8; \}/);
  assert.match(calloutCSS(false), /\.callout-note \{ --callout-color: #0969da; \}/);
  setCallouts({ note: { color: 'orange' }, bad: { color: 'red;}body{x' } });
  assert.match(calloutCSS(true), /\.callout-note \{ --callout-color: orange; \}/);
  assert.ok(!calloutCSS(true).includes('callout-bad'));
});

test('escaped marker, marker-only callout, plain blockquote, extensions inside', () => {
  setCallouts({});
  assert.match(md('> \\[!NOTE]\n> x'), /<blockquote>/);
  assert.match(md('> [!TIP]'), /callout-tip/);
  assert.match(md('> just a quote'), /<blockquote>/);
  assert.match(md('> [!TIP]\n> {{hi}}'), /<mark class="hl">/);
});
