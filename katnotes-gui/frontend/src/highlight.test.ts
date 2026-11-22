import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Marked } from 'marked';
import { highlightExtension } from './highlight.ts';

const md = (s: string) => new Marked(highlightExtension).parse(s, { async: false }) as string;

test('{{ }} is wrapped in a highlight mark, braces kept', () => {
  assert.equal(md('a {{b **c**}} d'), '<p>a <mark class="hl">{{b <strong>c</strong>}}</mark> d</p>\n');
});

test('code is left alone', () => {
  assert.equal(md('`{{x}}`'), '<p><code>{{x}}</code></p>\n');
  assert.ok(!md('```\n{{x}}\n```').includes('mark'));
});

test('empty or unclosed braces are plain text', () => {
  assert.ok(!md('{{}} and {{ }} and {{open').includes('mark'));
});
