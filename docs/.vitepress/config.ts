import { defineConfig, type HeadConfig } from 'vitepress'

const defaultTitle = 'monorel'
const defaultDescription =
  'A changesets-style release tool for multi-module Go monorepos.'
const baseUrl = 'https://monorel.disaresta.com'
const gaMeasurementId = 'G-BFQMES7Z4B'

export default defineConfig({
  lang: 'en-US',
  title: 'monorel',
  description: defaultDescription,
  srcDir: 'src',
  appearance: 'force-dark',
  sitemap: { hostname: baseUrl },
  ignoreDeadLinks: [/ci\/github\/README/],
  async transformHead({ pageData }) {
    const head: HeadConfig[] = [
      // Google Analytics (gtag.js)
      [
        'script',
        {
          async: '',
          src: `https://www.googletagmanager.com/gtag/js?id=${gaMeasurementId}`,
        },
      ],
      [
        'script',
        {},
        `window.dataLayer = window.dataLayer || [];
function gtag(){dataLayer.push(arguments);}
gtag('js', new Date());
gtag('config', '${gaMeasurementId}');`,
      ],
      ['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
      // Vanity import path. Go's toolchain fetches
      // monorel.disaresta.com/cmd/monorel?go-get=1, parses these
      // meta tags, then pulls source from the GitHub repo.
      // pkg.go.dev uses go-source to render "View Source" links.
      ['meta', {
        name: 'go-import',
        content: 'monorel.disaresta.com git https://github.com/disaresta-org/monorel',
      }],
      ['meta', {
        name: 'go-source',
        content: 'monorel.disaresta.com https://github.com/disaresta-org/monorel https://github.com/disaresta-org/monorel/tree/main{/dir} https://github.com/disaresta-org/monorel/blob/main{/dir}/{file}#L{line}',
      }],
      ['meta', {
        name: 'keywords',
        content: 'monorel, go, golang, monorepo, release, changesets, semver',
      }],
      ['meta', { property: 'og:type', content: 'website' }],
      ['meta', { property: 'og:site_name', content: 'monorel' }],
      ['meta', { property: 'og:locale', content: 'en_US' }],
      ['meta', { name: 'twitter:card', content: 'summary' }],
    ]

    head.push([
      'meta',
      {
        property: 'og:title',
        content: String(pageData?.frontmatter?.title ?? defaultTitle).replace(/"/g, '&quot;'),
      },
    ])
    head.push([
      'meta',
      {
        property: 'og:description',
        content: String(pageData?.frontmatter?.description ?? defaultDescription).replace(
          /"/g,
          '&quot;'
        ),
      },
    ])
    head.push([
      'meta',
      {
        property: 'og:url',
        content: `${baseUrl}${pageData.relativePath ? '/' + pageData.relativePath.replace(/\.md$/, '') : ''}`,
      },
    ])

    return head
  },
  themeConfig: {
    logo: { src: '/logo-v2.webp', alt: 'monorel' },
    editLink: {
      pattern: 'https://github.com/disaresta-org/monorel/edit/main/docs/src/:path',
      text: 'Edit this page on GitHub',
    },
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    nav: [
      { text: 'Get Started', link: '/getting-started' },
      { text: "What's New", link: '/whats-new' },
      {
        text: '<img alt="Go Reference" src="https://pkg.go.dev/badge/monorel.disaresta.com.svg" />',
        link: 'https://pkg.go.dev/monorel.disaresta.com',
      },
      {
        text: '<img alt="Latest version" src="https://img.shields.io/github/v/tag/disaresta-org/monorel?filter=v*&amp;sort=date&amp;label=version&amp;style=flat-square&color=blue" />',
        link: 'https://github.com/disaresta-org/monorel/releases',
      },
    ],
    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'Why monorel?', link: '/introduction' },
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Workflows', link: '/workflows' },
          { text: 'Design', link: '/design' },
          { text: "What's New", link: '/whats-new' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Cheat Sheet', link: '/cheat-sheet' },
          { text: 'Configuration', link: '/configuration' },
          { text: 'CLI', link: '/cli-reference' },
          { text: 'Library API', link: '/api' },
          { text: 'Changesets', link: '/changesets' },
          { text: 'Glossary', link: '/glossary' },
        ],
      },
      {
        text: 'Integrations',
        items: [
          { text: 'Bitbucket', link: '/integrations/bitbucket' },
          { text: 'GitHub', link: '/integrations/github' },
          { text: 'Gitea / Forgejo', link: '/integrations/gitea' },
          { text: 'GitLab', link: '/integrations/gitlab' },
        ],
      },
      {
        text: 'Help',
        items: [
          { text: 'FAQ', link: '/faq' },
          { text: 'Use with AI / LLMs', link: '/llms' },
        ],
      },
      {
        text: 'Recipes',
        items: [
          { text: 'Running in Docker', link: '/docker' },
          { text: 'Migrating from release-please', link: '/recipes/migration-from-release-please' },
        ],
      },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/disaresta-org/monorel' },
    ],
    lastUpdated: {
      text: 'Updated',
      formatOptions: { dateStyle: 'medium' },
    },
    docFooter: {
      prev: 'Previous',
      next: 'Next',
    },
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'monorel · made with ❤️ by <a href="https://disaresta.com">Disaresta</a>',
    },
  },
})
