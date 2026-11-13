import { defineConfig } from 'vitepress'

const repo = 'https://github.com/jakesmd/hobnob'

export default defineConfig({
  title: 'hobnob',
  description: 'A YAML task runner where tasks return values, prompts are steps and JSON is data.',
  base: '/hobnob/',
  cleanUrls: true,
  lastUpdated: true,
  appearance: 'force-dark',
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: '/hobnob/logo.svg' }]],
  srcExclude: ['adr/**', 'agents/**'],

  rewrites: {
    'installation.md': 'docs/installation.md',
    'quick-start.md': 'docs/quick-start.md',
    'guide/:page': 'docs/guide/:page',
    'reference/:page': 'docs/reference/:page',
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
      { text: 'Get started', link: '/docs/installation', activeMatch: '^/docs/(installation|quick-start)' },
      { text: 'Guide', link: '/docs/guide/', activeMatch: '^/docs/guide/' },
      { text: 'Reference', link: '/docs/reference/', activeMatch: '^/docs/reference/' },
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
