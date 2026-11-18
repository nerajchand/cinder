# web

The frontend source of truth lives in `internal/web/dist/` (plain ES modules, no build step, no CDN, no inline scripts or styles) so that `go:embed` needs no copy step. This directory holds only tests and this note.

- `internal/web/dist/index.html`: create page (and link page, same document).
- `internal/web/dist/view.html`: view shell served for every `/s/{id}` (all states are in the markup, toggled by `view.js`).
- `internal/web/dist/static/`: `crypto.js`, `envelope.js`, `markdown.js`, `highlight.js`, `render.js`, `api.js`, `create.js`, `view.js`, `theme.js`, `style.css`, and `vendor/qrcode.js` (+ license).
- Envelope limit: the whole `{lang,text}` JSON is at most 65,520 bytes so that the ciphertext (plus the 16 byte GCM tag) fits the server's 64 KiB cap.

## Tests

```
node --test        # needs Node >= 22 (ES modules in .js files, WebCrypto); no npm deps
go test ./internal/web
```

Not covered by automated tests: real-browser behaviour (clipboard, canvas QR, theme toggle, tab keyboard handling, CSP enforcement).
