import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import mermaid from 'astro-mermaid';

export default defineConfig({
  site: process.env.SITE_URL || 'http://localhost:4321',
  base: process.env.BASE_PATH || '/',
  integrations: [
    mermaid(),
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
        { label: 'Introduction', items: [{ autogenerate: { directory: 'introduction' } }] },
        {
          label: 'Resources',
          items: [
            'resources/overview',
            { label: 'Repository', items: [{ autogenerate: { directory: 'resources/repository' } }] },
            { label: 'RepositorySet', items: [{ autogenerate: { directory: 'resources/repository-set' } }] },
            { label: 'File', items: [{ autogenerate: { directory: 'resources/file' } }] },
            { label: 'FileSet', items: [{ autogenerate: { directory: 'resources/fileset' } }] },
          ],
        },
        { label: 'Commands', items: [{ autogenerate: { directory: 'commands' } }] },
        { label: 'Guides', items: [{ autogenerate: { directory: 'patterns' } }] },
        { label: 'Internals', items: [{ autogenerate: { directory: 'internals' } }] },
      ],
    }),
  ],
});
