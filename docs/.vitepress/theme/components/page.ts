import PAGE_HTML from '../../../../internal/browser/page.html?raw'

// The page as hobnob serves it, but talking to the component that frames it:
// snapshots come in as messages and its actions go back out as messages. The
// site is always dark, so the page's light theme goes. Focus never scrolls the
// homepage to the frame.
const SHIM = `<script>
(() => {
  const focus = HTMLElement.prototype.focus;
  HTMLElement.prototype.focus = function (options) { focus.call(this, { ...options, preventScroll: true }); };
  window.EventSource = class {
    constructor() {
      addEventListener('message', (e) => {
        if (e.source === parent && e.data && e.data.hobnobSnapshot) this.onmessage && this.onmessage({ data: e.data.hobnobSnapshot });
      });
      parent.postMessage({ hobnobReady: true }, '*');
    }
    close() {}
  };
  window.fetch = (url, init) => {
    parent.postMessage({ hobnobAction: String(url).split('?')[0].replace('/actions/', ''), body: init && init.body }, '*');
    return Promise.resolve(new Response(null, { status: 204 }));
  };
})();
<` + `/script>`
export const PAGE = PAGE_HTML.replace('@media (prefers-color-scheme: light)', '@media not all').replace('<script>', SHIM + '<script>')
