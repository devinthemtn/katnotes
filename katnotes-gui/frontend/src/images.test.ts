import { test } from 'node:test';
import assert from 'node:assert/strict';
import { assetURL, rewriteImageSrc, assetName, imageFiles, fileToBase64, imageInsert } from './images.ts';

test('assetURL', () => {
  assert.equal(assetURL('../assets/a.png'), '/kn-assets/a.png');
  assert.equal(assetURL('assets/a%20b.png'), '/kn-assets/a%20b.png');
  assert.equal(assetURL('./assets/a.png'), '/kn-assets/a.png');
  assert.equal(assetURL('https://x.com/assets/a.png'), null);
  assert.equal(assetURL('../assets/sub/a.png'), null);
  assert.equal(assetURL('/etc/assets/a.png'), null);
});

test('rewriteImageSrc only touches asset images', () => {
  const html = '<p><img src="../assets/a.png" alt="A"> <img alt="" src="https://e.com/b.png"> <a href="../assets/a.png">x</a></p>';
  assert.equal(
    rewriteImageSrc(html),
    '<p><img src="/kn-assets/a.png" alt="A"> <img alt="" src="https://e.com/b.png"> <a href="../assets/a.png">x</a></p>',
  );
});

test('assetName drops generic clipboard names', () => {
  assert.equal(assetName({ name: 'image.png' }, true), '');
  assert.equal(assetName({ name: 'image.png' }, false), 'image.png');
  assert.equal(assetName({ name: 'diagram.png' }, true), 'diagram.png');
});

test('imageFiles filters', () => {
  const f = (name: string, type: string) => ({ name, type }) as File;
  assert.deepEqual(imageFiles([f('a.png', 'image/png'), f('b.txt', 'text/plain'), f('c.JPG', '')]).map((x) => x.name), ['a.png', 'c.JPG']);
  assert.deepEqual(imageFiles(null), []);
});

test('fileToBase64', async () => {
  assert.equal(await fileToBase64(new Blob(['hi!'])), 'aGkh');
});

test('imageInsert puts images on their own lines', () => {
  assert.equal(imageInsert('', 0, ['![](a)']), '![](a)');
  assert.equal(imageInsert('ab', 1, ['![](a)', '![](b)']), '\n![](a)\n![](b)\n');
  assert.equal(imageInsert('a\n', 2, ['![](a)']), '![](a)');
});
