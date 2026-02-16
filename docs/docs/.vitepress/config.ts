import { defineConfig } from 'vitepress'
import { blogs } from './blogs'
import { en } from './en'
import { zh } from './zh'

// https://vitepress.vuejs.org/config/app-configs
export default defineConfig({
  title: 'Memoh Documentation',
  description: 'Multi-Member, Structured Long-Memory, Containerized AI Agent System.',

  head: [
    ['link', { rel: 'icon', href: '/logo.png' }]
  ],

  base: '/',

  locales: {
    root: {
      label: 'English',
      lang: 'en'
    },
    zh: {
      label: '简体中文',
      lang: 'zh',
    }
  },

  themeConfig: {
    siteTitle: 'Memoh',
    sidebar: {
      '/blogs/': blogs,
      '/': en,
      '/zh/': zh,
    },

    nav: [
      { text: 'Guides', link: '/' },
      { text: 'Blogs', link: '/blogs/' },
    ],

    logo: {
      src: '/logo.png',
      alt: 'Memoh'
    },
    
    socialLinks: [
      { icon: 'github', link: 'https://github.com/memohai/Memoh' }
    ],
    
    footer: {
      message: 'Published under AGPLv3',
      copyright: 'Copyright © 2024 Memoh'
    },
    
    search: {
      provider: 'local'
    },
    
    editLink: {
      pattern: 'https://github.com/memohai/Memoh/edit/main/docs/docs/:path',
      text: 'Edit on GitHub'
    },
    
    lastUpdated: {
      text: 'Last Updated',
      formatOptions: {
        dateStyle: 'short',
        timeStyle: 'medium'
      }
    }
  },

  ignoreDeadLinks: true,
})
