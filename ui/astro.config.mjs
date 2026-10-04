// @ts-check
import { defineConfig } from 'astro/config';

import react from '@astrojs/react';

export default defineConfig({
  site: 'https://mausamgiri.in',
  trailingSlash: 'never',
  integrations: [react()],
  output: "static"
});