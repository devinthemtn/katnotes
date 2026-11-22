// Images in notes live in <notes>/assets/ and are written as
// "![alt](../assets/file.png)", so other Markdown tools show them too. The
// webview can't read local files, so the preview points them at the Go
// side's /kn-assets/ route instead.

export const ASSET_ROUTE = '/kn-assets/';

// "../assets/x.png", "./assets/x.png" or "assets/x.png" (any number of ../).
const assetSrc = /^(?:\.{1,2}\/)*assets\/([^/?#]+)$/;

// The webview URL for an image src, or null if it isn't in assets/.
export function assetURL(src: string): string | null {
  const m = src.match(assetSrc);
  return m ? ASSET_ROUTE + m[1] : null;
}

// Rewrite <img src="…assets/…"> in rendered HTML to load from the app.
export function rewriteImageSrc(html: string): string {
  return html.replace(/<img\b[^>]*>/g, (tag) =>
    tag.replace(/\ssrc="([^"]*)"/, (m, src: string) => {
      const u = assetURL(src);
      return u ? ` src="${u}"` : m;
    }),
  );
}

// Clipboard images come named "image.png" (or similar); those get a
// "pasted-<time>" name on the Go side instead.
export function assetName(file: { name: string }, pasted: boolean): string {
  if (pasted && /^image(\.\w+)?$/i.test(file.name)) return '';
  return file.name;
}

export function isImage(file: { type: string; name: string }): boolean {
  return file.type.startsWith('image/') || /\.(png|jpe?g|gif|webp|svg|avif|bmp)$/i.test(file.name);
}

// The image files in a paste or drop.
export function imageFiles(files: FileList | File[] | null | undefined): File[] {
  return Array.from(files ?? []).filter(isImage);
}

export async function fileToBase64(file: Blob): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(bin);
}

// Text to insert for images at pos in body: each on its own line.
export function imageInsert(body: string, pos: number, md: string[]): string {
  let text = md.join('\n');
  if (pos > 0 && body[pos - 1] !== '\n') text = '\n' + text;
  if (pos < body.length && body[pos] !== '\n') text += '\n';
  return text;
}
