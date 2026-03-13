import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';

const repository = process.env.GITHUB_REPOSITORY ?? 'eric/wails-release';
const [ownerName = 'eric', repoName = 'wails-release'] = repository.split('/');
const isUserSite = repoName.toLowerCase() === `${ownerName.toLowerCase()}.github.io`;

const config: Config = {
  title: 'wailsrel',
  tagline: 'Release and update tooling for Wails v3 applications',
  favicon: 'img/favicon.svg',
  future: {
    v4: true,
  },
  url: process.env.DOCS_URL ?? `https://${ownerName}.github.io`,
  baseUrl: process.env.DOCS_BASE_URL ?? (isUserSite ? '/' : `/${repoName}/`),
  organizationName: ownerName,
  projectName: repoName,
  trailingSlash: false,
  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },
  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: `https://github.com/${ownerName}/${repoName}/tree/main/website/`,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],
  themeConfig: {
    image: 'img/social-card.svg',
    colorMode: {
      defaultMode: 'light',
      disableSwitch: false,
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'wailsrel',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Docs',
        },
        {
          href: `https://github.com/${ownerName}/${repoName}`,
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Quick Start',
              to: '/getting-started',
            },
            {
              label: 'CLI Commands',
              to: '/commands',
            },
          ],
        },
        {
          title: 'Project',
          items: [
            {
              label: 'Repository',
              href: `https://github.com/${ownerName}/${repoName}`,
            },
            {
              label: 'GitHub Pages Setup',
              to: '/github-pages',
            },
          ],
        },
      ],
      copyright: `Copyright ${new Date().getFullYear()} wailsrel`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  },
};

export default config;
