/** Tailwind build config for NGINX++ (compiled at build time, no CDN). */
module.exports = {
  darkMode: 'class',
  content: ['./static/*.html', './static/*.js'],
  // Classes assembled at runtime in JS (e.g. `bg-${stg.color}-500/15`)
  safelist: [
    { pattern: /^(bg|text|border)-(blue|emerald|amber|purple|sky|rose)-(400|500)(\/(15|20|40))?$/ },
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'Menlo', 'monospace'],
      },
      colors: {
        np: {
          green: '#34D399', greenDark: '#059669', greenAlt: '#10B981',
          black: '#080F11', charcoal: '#1D252C', surfaceDark: '#0f171a',
          cream: '#F1F0E4', beige: '#E5E4D8', gray: '#898E91',
        },
      },
    },
  },
};
