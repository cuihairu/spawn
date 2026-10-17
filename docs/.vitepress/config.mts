import { defineConfig } from 'vitepress'

// 坑：GitHub Pages 项目站部署在 /spawn/ 子路径下，base 必须与仓库名一致，
// 否则线上资源全部 404；head 里的 favicon 等静态引用不自动拼 base，需手写前缀。
export default defineConfig({
  title: 'spawn',
  description:
    '游戏社区平台：攻略、榜单与社区的 monorepo 实现，React Web 端、Expo 移动端与 go-zero 微服务。',
  base: '/spawn/',
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/spawn/favicon.svg' }],
    ['link', { rel: 'icon', type: 'image/png', href: '/spawn/favicon.png' }],
    ['meta', { name: 'theme-color', content: '#d4237a' }]
  ],
  rewrites: {
    'index.zh.md': 'zh/index.md',
    'README.zh.md': 'zh/README.md',
    '审计-文档一致性.zh.md': 'zh/doc-audit.md',
    'guide/getting-started.zh.md': 'zh/guide/getting-started.md',
    'architecture/overview.zh.md': 'zh/architecture/overview.md',
    'architecture/topology.zh.md': 'zh/architecture/topology.md',
    'clients.zh.md': 'zh/clients.md',
    'development-guide.zh.md': 'zh/development-guide.md',
    'realtime-hub.zh.md': 'zh/realtime-hub.md',
    'backlog.zh.md': 'zh/backlog.md',
    'mobile_plan.zh.md': 'zh/mobile_plan.md'
  },
  locales: {
    root: {
      label: 'English',
      lang: 'en',
      themeConfig: {
        logo: '/logo.svg',
        nav: [
          { text: 'Home', link: '/' },
          { text: 'Getting Started', link: '/guide/getting-started' },
          {
            text: 'Architecture',
            items: [
              { text: 'Architecture Overview', link: '/architecture/overview' },
              { text: 'Current Topology', link: '/architecture/topology' }
            ]
          },
          { text: 'Clients & Services', link: '/clients' },
          { text: 'Development Guide', link: '/development-guide' }
        ],
        sidebar: {
          '/guide/': [
            { text: 'Getting Started', link: '/guide/getting-started' }
          ],
          '/architecture/': [
            {
              text: 'Architecture',
              items: [
                { text: 'Architecture Overview', link: '/architecture/overview' },
                { text: 'Current Topology', link: '/architecture/topology' }
              ]
            }
          ],
          '/': [
            {
              text: 'Guides',
              items: [
                { text: 'Getting Started', link: '/guide/getting-started' },
                { text: 'Clients & Services', link: '/clients' }
              ]
            },
            {
              text: 'Architecture',
              items: [
                { text: 'Architecture Overview', link: '/architecture/overview' },
                { text: 'Current Topology', link: '/architecture/topology' }
              ]
            },
            {
              text: 'Documents',
              items: [
                { text: 'Documentation Index', link: '/README' },
                { text: 'Mobile Plan', link: '/mobile_plan' },
                { text: 'Development Guide', link: '/development-guide' },
                { text: 'Realtime Hub', link: '/realtime-hub' },
                { text: 'Backlog', link: '/backlog' },
                { text: 'Documentation Audit', link: '/doc-audit' }
              ]
            }
          ]
        },
        socialLinks: [
          { icon: 'github', link: 'https://github.com/cuihairu/spawn' }
        ],
        search: {
          provider: 'local',
          options: {
            translations: {
              button: { buttonText: 'Search docs', buttonAriaLabel: 'Search docs' },
              modal: {
                noResultsText: 'No results',
                resetButtonTitle: 'Reset keyword',
                footer: { selectText: 'Select', navigateText: 'Navigate', closeText: 'Close' }
              }
            }
          }
        },
        outline: { level: [2, 3], label: 'On this page' },
        docFooter: { prev: 'Previous', next: 'Next' },
        returnToTopLabel: 'Back to top',
        sidebarMenuLabel: 'Table of Contents',
        darkModeSwitchLabel: 'Appearance',
        lightModeSwitchTitle: 'Switch to light theme',
        darkModeSwitchTitle: 'Switch to dark theme',
        footer: {
          message: 'spawn · game community platform',
          copyright: 'Copyright © 2026 spawn contributors'
        }
      }
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      themeConfig: {
        logo: '/logo.svg',
        nav: [
          { text: '首页', link: '/zh/' },
          { text: '快速上手', link: '/zh/guide/getting-started' },
          {
            text: '架构',
            items: [
              { text: '架构总览', link: '/zh/architecture/overview' },
              { text: '现状拓扑', link: '/zh/architecture/topology' }
            ]
          },
          { text: '各端说明', link: '/zh/clients' },
          { text: '开发指南', link: '/zh/development-guide' }
        ],
        sidebar: {
          '/zh/guide/': [
            { text: '快速上手', link: '/zh/guide/getting-started' }
          ],
          '/zh/architecture/': [
            {
              text: '架构',
              items: [
                { text: '架构总览', link: '/zh/architecture/overview' },
                { text: '现状拓扑', link: '/zh/architecture/topology' }
              ]
            }
          ],
          '/zh/': [
            {
              text: '指南',
              items: [
                { text: '快速上手', link: '/zh/guide/getting-started' },
                { text: '各端说明', link: '/zh/clients' }
              ]
            },
            {
              text: '架构',
              items: [
                { text: '架构总览', link: '/zh/architecture/overview' },
                { text: '现状拓扑', link: '/zh/architecture/topology' }
              ]
            },
            {
              text: '文档',
              items: [
                { text: '文档索引', link: '/zh/README' },
                { text: '移动端计划', link: '/zh/mobile_plan' },
                { text: '开发指南', link: '/zh/development-guide' },
                { text: 'Realtime Hub', link: '/zh/realtime-hub' },
                { text: 'Backlog', link: '/zh/backlog' },
                { text: '文档一致性审计', link: '/zh/doc-audit' }
              ]
            }
          ]
        },
        socialLinks: [
          { icon: 'github', link: 'https://github.com/cuihairu/spawn' }
        ],
        search: {
          provider: 'local',
          options: {
            translations: {
              button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
              modal: {
                noResultsText: '没有找到结果',
                resetButtonTitle: '清空关键词',
                footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' }
              }
            }
          }
        },
        outline: { level: [2, 3], label: '本页目录' },
        docFooter: { prev: '上一篇', next: '下一篇' },
        returnToTopLabel: '回到顶部',
        sidebarMenuLabel: '目录',
        darkModeSwitchLabel: '主题',
        lightModeSwitchTitle: '切换到亮色',
        darkModeSwitchTitle: '切换到暗色',
        footer: {
          message: 'spawn · 游戏社区平台',
          copyright: 'Copyright © 2026 spawn contributors'
        }
      }
    }
  }
})
