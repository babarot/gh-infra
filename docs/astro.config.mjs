import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: process.env.SITE_URL || 'http://localhost:4321',
  base: process.env.BASE_PATH || '/',
  integrations: [
    starlight({
      title: 'gh-infra',
      description: 'Declarative GitHub infrastructure management via YAML',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/babarot/gh-infra' },
      ],
      editLink: {
        baseUrl: 'https://github.com/babarot/gh-infra/edit/main/docs/',
      },
      pagination: false,
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        { label: 'Introduction', autogenerate: { directory: 'introduction' } },
        {
          label: 'YAML DSL Reference',
          items: [
            'reference/overview',
            { label: 'Repository', autogenerate: { directory: 'reference/repository' } },
            { label: 'RepositorySet', autogenerate: { directory: 'reference/repository-set' } },
            { label: 'FileSet', autogenerate: { directory: 'reference/fileset' } },
          ],
        },
        { label: 'Commands', autogenerate: { directory: 'commands' } },
        { label: 'Guides', autogenerate: { directory: 'patterns' } },
        { label: 'Advanced', autogenerate: { directory: 'advanced' } },
      ],
    }),
  ],
});
