import { defineConfig } from 'vitepress'

// 坑：GitHub Pages 项目站部署在 /spawn/ 子路径下，base 必须与仓库名一致，
// 否则线上资源全部 404；head 里的 favicon 等静态引用不自动拼 base，需手写前缀。
export default defineConfig({
  lang: 'zh-CN',
  title: 'spawn',
  description:
    '游戏社区平台：攻略、榜单与社区的 monorepo 实现，React Web 端、Expo 移动端与 go-zero 微服务。',
  base: '/spawn/',
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/spawn/favicon.svg' }],
    ['link', { rel: 'icon', type: 'image/png', href: '/spawn/favicon.png' }],
    ['meta', { name: 'theme-color', content: '#d4237a' }]
  ],
  themeConfig: {
    // 注意：themeConfig.logo 由 VitePress 自动拼 base，此处不写 /spawn/ 前缀，
    // 手写会变成 /spawn/spawn/logo.svg。head 里的 favicon 不自动拼，需手写前缀。
    logo: '/logo.svg',
    nav: [
      { text: '首页', link: '/' },
      { text: '快速上手', link: '/guide/getting-started' },
      {
        text: '架构',
        items: [
          { text: '架构总览', link: '/architecture/overview' },
          { text: '现状拓扑', link: '/architecture/topology' }
        ]
      },
      { text: '各端说明', link: '/clients' },
      { text: '开发指南', link: '/development-guide' }
    ],
    sidebar: {
      '/guide/': [
        {
          text: '快速上手',
          link: '/guide/getting-started'
        }
      ],
      '/architecture/': [
        {
          text: '架构',
          items: [
            { text: '架构总览', link: '/architecture/overview' },
            { text: '现状拓扑', link: '/architecture/topology' }
          ]
        }
      ],
      '/': [
        {
          text: '指南',
          items: [
            { text: '快速上手', link: '/guide/getting-started' },
            { text: '各端说明', link: '/clients' }
          ]
        },
        {
          text: '架构',
          items: [
            { text: '架构总览', link: '/architecture/overview' },
            { text: '现状拓扑', link: '/architecture/topology' }
          ]
        },
        {
          text: '文档',
          items: [
            { text: '文档索引', link: '/README' },
            { text: '移动端计划', link: '/mobile_plan' },
            { text: '开发指南', link: '/development-guide' }
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
})
