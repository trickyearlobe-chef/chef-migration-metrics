/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      keyframes: {
        highlight: {
          '0%': { backgroundColor: 'rgb(220 252 231)' },   // green-100
          '100%': { backgroundColor: 'transparent' },
        },
        indeterminate: {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(100%)' },
        },
      },
      animation: {
        highlight: 'highlight 2s ease-out',
        indeterminate: 'indeterminate 1.5s infinite ease-in-out',
      },
      colors: {
        // Status colours matching the spec confidence indicators
        'status-compatible': '#16a34a',    // green-600
        'status-cookstyle': '#d97706',    // amber-600
        'status-incompatible': '#dc2626',  // red-600
        'status-untested': '#6b7280',    // gray-500
        'status-stale': '#9333ea',    // purple-600
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
}
