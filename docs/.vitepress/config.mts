import { defineConfig } from 'vitepress'

const repo = 'https://github.com/jakesmd/hobnob'
const site = 'https://hobnob.jakesmd.dev'

export default defineConfig({
  title: 'hobnob',
  description: 'Turn the shell commands in your README into tasks anyone can run: in the terminal, in the browser or on CI.',
  base: '/',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: site },
  appearance: 'force-dark',
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }],
    ['meta', { property: 'og:site_name', content: 'hobnob' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:image', content: `${site}/og.png` }],
    ['meta', { property: 'og:image:width', content: '1200' }],
    ['meta', { property: 'og:image:height', content: '630' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],

  // Per page: the canonical URL, and the title and description link previews show.
  transformHead({ pageData, title, description }) {
    const path = pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    const url = `${site}/${path}`
    const head: [string, Record<string, string>, string?][] = [
      ['link', { rel: 'canonical', href: url }],
      ['meta', { property: 'og:url', content: url }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
    ]
    if (path === '') {
      head.push(['script', { type: 'application/ld+json' }, JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'SoftwareApplication',
        name: 'hobnob',
        url: site,
        description,
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'macOS, Linux',
        license: 'https://opensource.org/licenses/BSD-3-Clause',
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
        sameAs: repo,
      })])
    }
    return head
  },
  srcExclude: ['adr/**', 'agents/**', 'parts/**'],

  // The homepage demo loads hobnob's --browser page from internal/browser.
  // ts.net lets `tailscale serve` proxy the dev server across a tailnet.
  vite: { server: { fs: { allow: ['..'] }, allowedHosts: ['.ts.net'] } },

  rewrites: {
    'installation.md': 'docs/installation.md',
    'quick-start.md': 'docs/quick-start.md',
    'guide/:page': 'docs/guide/:page',
    'reference/:page': 'docs/reference/:page',
    'compare/:page': 'docs/compare/:page',
  },

  markdown: {
    anchor: {
      slugify: (s) =>
        s.trim().toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').replace(/\s/g, '-'),
    },
    config: (md) => {
      const inline = md.renderer.rules.code_inline!
      md.renderer.rules.code_inline = (tokens, idx, options, env, self) => {
        tokens[idx].attrSet('v-pre', '')
        return inline(tokens, idx, options, env, self)
      }
    },
  },

  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'Docs', link: '/docs/installation', activeMatch: '^/docs/' },
      { text: 'Releases', link: `${repo}/releases` },
    ],

    sidebar: {
      '/docs/': [
        {
          text: 'Getting started',
          items: [
            { text: 'Installation', link: '/docs/installation' },
            { text: 'Quick start', link: '/docs/quick-start' },
          ],
        },
        {
          text: 'Guide',
          link: '/docs/guide/',
          items: [
            {
              text: 'Basics',
              collapsed: false,
              items: [
                { text: 'Tasks', link: '/docs/guide/tasks' },
                { text: 'Running tasks', link: '/docs/guide/running-tasks' },
                { text: 'Running commands', link: '/docs/guide/commands' },
                { text: 'Browser mode', link: '/docs/guide/browser-mode' },
              ],
            },
            {
              text: 'Values',
              collapsed: false,
              items: [
                { text: 'Variables', link: '/docs/guide/variables' },
                { text: 'Templates', link: '/docs/guide/templates' },
                { text: 'Structured data', link: '/docs/guide/structured-data' },
                { text: 'Secrets', link: '/docs/guide/secrets' },
              ],
            },
            {
              text: 'Getting values',
              collapsed: false,
              items: [
                { text: 'Capturing output', link: '/docs/guide/capturing-output' },
                { text: 'Calling tasks', link: '/docs/guide/calling-tasks' },
                { text: 'Prompts', link: '/docs/guide/prompts' },
              ],
            },
            {
              text: 'Control flow',
              collapsed: false,
              items: [
                { text: 'Conditions', link: '/docs/guide/conditions' },
                { text: 'Loops', link: '/docs/guide/loops' },
                { text: 'Errors', link: '/docs/guide/errors' },
              ],
            },
            {
              text: 'Scaling up',
              collapsed: false,
              items: [
                { text: 'Running once', link: '/docs/guide/running-once' },
                { text: 'Modules', link: '/docs/guide/modules' },
              ],
            },
          ],
        },
        {
          text: 'Reference',
          link: '/docs/reference/',
          items: [
            { text: 'CLI', link: '/docs/reference/cli' },
            { text: 'Taskfile schema', link: '/docs/reference/schema' },
            { text: 'Templating', link: '/docs/reference/templating' },
          ],
        },
        {
          text: 'Compared',
          link: '/docs/compare/',
          items: [
            { text: 'hobnob vs a bash script', link: '/docs/compare/bash' },
            { text: 'hobnob vs a custom CLI', link: '/docs/compare/cli' },
            { text: 'hobnob vs Task', link: '/docs/compare/task' },
            { text: 'hobnob vs just', link: '/docs/compare/just' },
            { text: 'hobnob vs make', link: '/docs/compare/make' },
          ],
        },
        {
          text: 'Project',
          items: [
            { text: 'Releases', link: `${repo}/releases` },
            { text: 'Contributing', link: `${repo}/blob/main/CONTRIBUTING.md` },
          ],
        },
      ],
    },

    outline: { level: [2, 3], label: 'On this page' },
    search: { provider: 'local' },
    socialLinks: [{ icon: 'github', link: repo }],
    editLink: { pattern: `${repo}/edit/main/docs/:path` },
    footer: { message: 'Released under the BSD-3-Clause License.' },
  },
})
