// @ts-check
import prettier from 'eslint-config-prettier';
import pluginPrettier from 'eslint-plugin-prettier';
import withNuxt from './.nuxt/eslint.config.mjs';

export default withNuxt(prettier, {
  plugins: {
    prettier: pluginPrettier,
  },
  rules: {
    'prettier/prettier': 'error',
  },
});
