import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import lucode from 'lucode-starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: 'Torment',
      plugins: [lucode()],
      components: { Head: './src/components/Head.astro' },
      sidebar: [
        { label: 'Getting started', items: ['getting-started/introduction', 'getting-started/quickstart', 'getting-started/concepts'] },
        {
          label: 'Build a brokerage',
          items: [
            'brokerage',
            'brokerage/chart-of-accounts',
            'brokerage/deposits',
            'brokerage/buying-shares',
            'brokerage/partial-fills',
            'brokerage/currency-exchange',
            'brokerage/dividends',
            'brokerage/withdrawals',
            'brokerage/statements',
            'brokerage/mistakes',
          ],
        },
        { label: 'Reference', items: ['reference/api', 'reference/errors', 'reference/limits'] },
        { label: 'Operating', items: ['operating/configuration', 'operating/production'] },
        { label: 'Internals', items: ['internals/decisions', 'internals/performance'] },
      ],
    }),
  ],
});
