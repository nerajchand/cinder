// Shared by the create-page preview and the reveal page, so both go through the
// same sanitising pipeline.
import { renderMarkdown } from './markdown.js';
import { highlightInto } from './highlight.js';

export function renderSecret(doc, container, lang, text) {
  while (container.firstChild) container.removeChild(container.firstChild);
  if (lang === 'markdown') {
    container.appendChild(renderMarkdown(text, doc));
    return;
  }
  const pre = doc.createElement('pre');
  const code = doc.createElement('code');
  if (lang === 'text') code.appendChild(doc.createTextNode(text));
  else highlightInto(doc, code, text, lang);
  pre.appendChild(code);
  container.appendChild(pre);
}
