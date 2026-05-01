import { defineConfig, type HeadConfig } from 'vitepress'

const defaultTitle = 'monorel'
const defaultDescription =
  'A changesets-style release tool for multi-module Go monorepos.'
const baseUrl = 'https://disaresta-org.github.io/monorel'

export default defineConfig({
  lang: 'en-US',
  title: 'monorel',
  description: defaultDescription,
  srcDir: 'src',
  base: '/monorel/',
  appearance: 'force-dark',
  sitemap: { hostname: baseUrl },
  async transformHead({ pageData }) {
    const head: HeadConfig[] = [
      ['link', { rel: 'icon', href: '/monorel/favicon.ico' }],
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
    editLink: {
      pattern: 'https://github.com/disaresta-org/monorel/edit/main/docs/src/:path',
      text: 'Edit this page on GitHub',
    },
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    nav: [
      { text: 'Get Started', link: '/getting-started' },
      { text: 'Configuration', link: '/configuration' },
      { text: 'CLI', link: '/cli-reference' },
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
          { text: 'Design', link: '/design' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Configuration', link: '/configuration' },
          { text: 'CLI', link: '/cli-reference' },
          { text: 'Changesets', link: '/changesets' },
          { text: 'GitHub Action', link: '/github-action' },
        ],
      },
      {
        text: 'Recipes',
        items: [
          { text: 'Migrating from release-please', link: '/recipes/migration-from-release-please' },
          { text: 'loglayer-go (worked example)', link: '/recipes/loglayer-go' },
          { text: 'Bootstrapping monorel', link: '/bootstrap' },
        ],
      },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/disaresta-org/monorel' },
    ],
  },
})
